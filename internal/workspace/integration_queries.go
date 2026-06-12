/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package workspace

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// GetIntegrationStrategyRefNamespace returns the namespace for a workspace's integration
// strategy reference, defaulting to the workspace's namespace if not specified.
func GetIntegrationStrategyRefNamespace(ws *workspacev1alpha1.Workspace) string {
	if ws.Spec.IntegrationStrategy == nil || ws.Spec.IntegrationStrategy.Namespace == "" {
		return ws.Namespace
	}
	return ws.Spec.IntegrationStrategy.Namespace
}

// ListActiveWorkspacesByIntegrationStrategy returns all active (non-deleted) workspaces using
// the specified WorkspaceIntegrationStrategy. Reads from the controller-runtime informer cache
// and validates the IntegrationStrategy reference matches the label to guard against drift.
func ListActiveWorkspacesByIntegrationStrategy(
	ctx context.Context,
	k8sClient client.Client,
	integrationStrategyName string,
	integrationStrategyNamespace string) ([]workspacev1alpha1.Workspace, error) {
	logger := logf.FromContext(ctx)

	workspaceList := &workspacev1alpha1.WorkspaceList{}

	labels := map[string]string{
		LabelIntegrationStrategyName:      integrationStrategyName,
		LabelIntegrationStrategyNamespace: integrationStrategyNamespace,
	}

	if err := k8sClient.List(ctx, workspaceList, client.MatchingLabels(labels)); err != nil {
		return nil, fmt.Errorf("failed to list workspaces by IntegrationStrategy label: %w", err)
	}

	activeWorkspaces := []workspacev1alpha1.Workspace{}
	for _, ws := range workspaceList.Items {
		if !ws.DeletionTimestamp.IsZero() {
			continue // Skip workspaces being deleted
		}

		// Verify the IntegrationStrategy reference matches the label to guard against drift
		if ws.Spec.IntegrationStrategy == nil {
			logger.Info("Workspace has IntegrationStrategy label but nil reference - data integrity issue",
				"workspace", ws.Name,
				"workspaceNamespace", ws.Namespace,
				"label", integrationStrategyName)
			continue
		}

		if ws.Spec.IntegrationStrategy.Name != integrationStrategyName {
			logger.Info("Workspace has IntegrationStrategy label but different name",
				"workspace", ws.Name,
				"workspaceNamespace", ws.Namespace,
				"label", integrationStrategyName,
				"spec", ws.Spec.IntegrationStrategy.Name)
			continue
		}

		if integrationStrategyNamespace != "" {
			actualNamespace := GetIntegrationStrategyRefNamespace(&ws)
			if actualNamespace != integrationStrategyNamespace {
				continue
			}
		}

		activeWorkspaces = append(activeWorkspaces, ws)
	}

	return activeWorkspaces, nil
}

// GetWorkspaceReconciliationRequestsForIntegrationStrategy retrieves all active workspaces
// using the specified IntegrationStrategy and returns them as reconcile requests.
func GetWorkspaceReconciliationRequestsForIntegrationStrategy(
	ctx context.Context,
	k8sClient client.Client,
	integrationStrategyName string,
	integrationStrategyNamespace string) ([]reconcile.Request, error) {

	workspaces, err := ListActiveWorkspacesByIntegrationStrategy(
		ctx, k8sClient, integrationStrategyName, integrationStrategyNamespace)
	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces by integration strategy: %w", err)
	}

	allRequests := []reconcile.Request{}
	for _, ws := range workspaces {
		allRequests = append(allRequests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      ws.Name,
				Namespace: ws.Namespace,
			},
		})
	}

	return allRequests, nil
}
