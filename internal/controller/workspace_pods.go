/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// errNoRunningPod is returned when a workspace has no running pod that is not being deleted.
var errNoRunningPod = errors.New("no running pod found for workspace")

// workspacePods lists the pods of a workspace by the labels the operator puts on them.
func workspacePods(ctx context.Context, c client.Reader, workspace *workspacev1alpha1.Workspace) ([]corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := c.List(ctx, podList,
		client.InNamespace(workspace.Namespace),
		client.MatchingLabels(GenerateLabels(workspace.Name)),
	); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	return podList.Items, nil
}

// runningWorkspacePod returns the workspace's running pod that is not being deleted, or errNoRunningPod.
// Under the Recreate strategy the previous pod can still report Running while it is torn down, and a probe
// or an idle check against it would answer for the pod on its way out.
func runningWorkspacePod(ctx context.Context, c client.Reader, workspace *workspacev1alpha1.Workspace) (*corev1.Pod, error) {
	pods, err := workspacePods(ctx, c, workspace)
	if err != nil {
		return nil, err
	}
	for i := range pods {
		if pods[i].Status.Phase == corev1.PodRunning && pods[i].DeletionTimestamp == nil {
			return &pods[i], nil
		}
	}
	return nil, errNoRunningPod
}
