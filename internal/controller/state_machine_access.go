/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package controller

import (
	"context"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ReconcileAccessForDesiredRunningStatus reconciles the access strategy for a Workspace whose desired state is Running
func (sm *StateMachine) ReconcileAccessForDesiredRunningStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	service *corev1.Service,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) error {
	logger := logf.FromContext(ctx)
	accessStrategyRef := workspace.Spec.AccessStrategy

	// CASE 1: there is an AccessStrategy
	// ensure the AccessResources exist
	if accessStrategyRef != nil {

		ensureAccessResourceErr := sm.resourceManager.EnsureAccessResourcesExist(ctx, workspace, accessStrategy, service)
		if ensureAccessResourceErr != nil {
			logger.Error(ensureAccessResourceErr, "Failed to apply access strategy")
			return ensureAccessResourceErr
		}

		accessUrl, accessUrlErr := sm.resourceManager.accessResourcesBuilder.ResolveAccessURL(workspace, accessStrategy, service)
		if accessUrlErr != nil {
			logger.Error(accessUrlErr, "Failed to retrieve Access URL from access strategy")
			return accessUrlErr
		}
		workspace.Status.AccessURL = accessUrl
		workspace.Status.AccessResourceSelector = sm.resourceManager.accessResourcesBuilder.ResolveAccessResourceSelector(
			workspace, accessStrategy)
		return nil
	}
	// END OF CASE 1

	// CASE 2: there is no AccessStrategy (it may have been removed by an update)
	workspace.Status.AccessURL = ""
	workspace.Status.AccessResourceSelector = ""

	err := sm.resourceManager.EnsureAccessResourcesDeleted(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to delete access resources")
		return err
	}
	return nil
}

// ReconcileAccessForDesiredStoppedStatus reconciles the access strategy for a Workspace whose desired state is Stopped
func (sm *StateMachine) ReconcileAccessForDesiredStoppedStatus(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	logger := logf.FromContext(ctx)

	workspace.Status.AccessURL = ""
	workspace.Status.AccessResourceSelector = ""

	err := sm.resourceManager.EnsureAccessResourcesDeleted(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to delete access resources")
		return err
	}
	return nil
}
