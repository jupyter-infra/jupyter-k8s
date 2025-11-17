/*
MIT License

Copyright (c) 2025 jupyter-ai-contrib

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
*/

package workspace

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Label constants for tracking workspace references
// These are defined here to avoid import cycles with the controller package
const (
	// Template labels
	LabelWorkspaceTemplate          = "workspace.jupyter.org/template"
	LabelWorkspaceTemplateNamespace = "workspace.jupyter.org/template-namespace"

	// AccessStrategy labels
	LabelAccessStrategyName      = "workspace.jupyter.org/access-strategy-name"
	LabelAccessStrategyNamespace = "workspace.jupyter.org/access-strategy-namespace"
)

// GetTemplateRefNamespace returns the namespace for a workspace's template reference,
// defaulting to the workspace's namespace if not specified
func GetTemplateRefNamespace(ws *workspacev1alpha1.Workspace) string {
	if ws.Spec.TemplateRef == nil || ws.Spec.TemplateRef.Namespace == "" {
		return ws.Namespace
	}
	return ws.Spec.TemplateRef.Namespace
}

// GetAccessStrategyRefNamespace returns the namespace for a workspace's access strategy reference,
// defaulting to the workspace's namespace if not specified
func GetAccessStrategyRefNamespace(ws *workspacev1alpha1.Workspace) string {
	if ws.Spec.AccessStrategy == nil || ws.Spec.AccessStrategy.Namespace == "" {
		return ws.Namespace
	}
	return ws.Spec.AccessStrategy.Namespace
}

// ListActiveWorkspacesByTemplate returns all active (non-deleted) workspaces using the specified template.
// Reads from controller-runtime's informer cache (not direct API calls), providing efficient lookup
// with eventual consistency guarantees. Filters out workspaces being deleted (DeletionTimestamp set).
// Validates templateRef matches label to guard against drift.
// If templateNamespace is empty, it acts as a wildcard (backwards compatible).
// Supports pagination for large-scale deployments via continueToken and limit parameters.
func ListActiveWorkspacesByTemplate(
	ctx context.Context,
	k8sClient client.Client,
	templateName string,
	templateNamespace string,
	continueToken string,
	limit int64) ([]workspacev1alpha1.Workspace, string, error) {
	logger := logf.FromContext(ctx)

	workspaceList := &workspacev1alpha1.WorkspaceList{}

	// Build label selector - namespace is optional for backwards compatibility
	labels := map[string]string{
		LabelWorkspaceTemplate: templateName,
	}
	if templateNamespace != "" {
		labels[LabelWorkspaceTemplateNamespace] = templateNamespace
	}

	listOptions := []client.ListOption{
		client.MatchingLabels(labels),
	}

	// Add pagination options if specified
	if limit > 0 {
		listOptions = append(listOptions, client.Limit(limit))
	}
	if continueToken != "" {
		listOptions = append(listOptions, client.Continue(continueToken))
	}

	if err := k8sClient.List(ctx, workspaceList, listOptions...); err != nil {
		return nil, "", fmt.Errorf("failed to list workspaces by template label: %w", err)
	}

	// Filter out workspaces being deleted and verify template reference
	// This follows Kubernetes controller best practice: resources with deletionTimestamp
	// are considered "gone" for dependency checking purposes
	activeWorkspaces := []workspacev1alpha1.Workspace{}
	for _, ws := range workspaceList.Items {
		if !ws.DeletionTimestamp.IsZero() {
			continue // Skip workspaces being deleted
		}

		// Verify templateRef matches label to guard against label/spec mismatch.
		// This is somewhat redundant given CEL immutability validation but adds zero cost and adds a layer of verification.
		if ws.Spec.TemplateRef == nil {
			logger.Info("Workspace has template label but nil templateRef - data integrity issue",
				"workspace", ws.Name,
				"workspaceNamespace", ws.Namespace,
				"label", templateName)
			continue
		}

		if ws.Spec.TemplateRef.Name != templateName {
			// This should never happen - log if it occurs
			logger.Info("Workspace has template label but different templateRef name",
				"workspace", ws.Name,
				"workspaceNamespace", ws.Namespace,
				"label", templateName,
				"spec", ws.Spec.TemplateRef.Name)
			continue
		}

		// Verify namespace if filtering by namespace
		if templateNamespace != "" {
			actualNamespace := GetTemplateRefNamespace(&ws)
			if actualNamespace != templateNamespace {
				logger.V(1).Info("Workspace has template label but different namespace",
					"workspace", ws.Name,
					"workspaceNamespace", ws.Namespace,
					"labelNamespace", templateNamespace,
					"specNamespace", actualNamespace)
				continue
			}
		}

		activeWorkspaces = append(activeWorkspaces, ws)
	}

	// Extract continuation token for next page
	nextToken := workspaceList.Continue

	return activeWorkspaces, nextToken, nil
}

// HasActiveWorkspacesWithTemplate checks if any active (non-deleted) workspace uses the specified template.
// Reads from controller-runtime's informer cache with eventual consistency guarantees.
// Returns true if at least one active workspace uses the template.
func HasActiveWorkspacesWithTemplate(ctx context.Context, k8sClient client.Client, templateName string, templateNamespace string) (bool, error) {
	workspaceList := &workspacev1alpha1.WorkspaceList{}

	// Build label selector - namespace is optional for backwards compatibility
	labels := map[string]string{
		LabelWorkspaceTemplate: templateName,
	}
	if templateNamespace != "" {
		labels[LabelWorkspaceTemplateNamespace] = templateNamespace
	}

	if err := k8sClient.List(ctx, workspaceList, client.MatchingLabels(labels)); err != nil {
		return false, fmt.Errorf("failed to check workspaces by template label: %w", err)
	}

	// Check if any non-deleted workspace exists
	for _, ws := range workspaceList.Items {
		if ws.DeletionTimestamp.IsZero() && ws.Spec.TemplateRef != nil && ws.Spec.TemplateRef.Name == templateName {
			// Verify namespace if filtering by namespace
			if templateNamespace != "" {
				actualNamespace := GetTemplateRefNamespace(&ws)
				if actualNamespace != templateNamespace {
					continue
				}
			}
			return true, nil
		}
	}

	return false, nil
}

// HasActiveWorkspacesWithAccessStrategy checks if any active (non-deleted) workspace uses the specified access strategy.
// Reads from controller-runtime's informer cache with eventual consistency guarantees.
// Returns true if at least one active workspace uses the template.
func HasActiveWorkspacesWithAccessStrategy(
	ctx context.Context,
	k8sClient client.Client,
	accessStrategyName string,
	accessStrategyNamespace string) (bool, error) {
	workspaceList := &workspacev1alpha1.WorkspaceList{}

	labels := map[string]string{
		LabelAccessStrategyName:      accessStrategyName,
		LabelAccessStrategyNamespace: accessStrategyNamespace,
	}

	if err := k8sClient.List(ctx, workspaceList, client.MatchingLabels(labels)); err != nil {
		return false, fmt.Errorf("failed to check workspaces by access strategy label: %w", err)
	}

	// Check if any non-deleted workspace exists
	for _, ws := range workspaceList.Items {
		if ws.DeletionTimestamp.IsZero() && ws.Spec.AccessStrategy != nil && ws.Spec.AccessStrategy.Name == accessStrategyName {
			// Verify namespace if filtering by namespace
			actualNamespace := GetAccessStrategyRefNamespace(&ws)
			if actualNamespace != accessStrategyNamespace {
				continue
			}
			return true, nil
		}
	}

	return false, nil
}

// ListActiveWorkspacesByAccessStrategy returns all active (non-deleted) workspaces using the specified AccessStrategy.
// Reads from controller-runtime's informer cache (not direct API calls), providing efficient lookup
// with eventual consistency guarantees. Filters out workspaces being deleted (DeletionTimestamp set).
// Validates AccessStrategy reference matches label to guard against drift.
// Supports pagination for large-scale deployments via continueToken and limit parameters.
// Returns list of workspaces, continuationToken and error.
func ListActiveWorkspacesByAccessStrategy(
	ctx context.Context,
	k8sClient client.Client,
	accessStrategyName string,
	accessStrategyNamespace string,
	continueToken string,
	limit int64) ([]workspacev1alpha1.Workspace, string, error) {
	logger := logf.FromContext(ctx)

	workspaceList := &workspacev1alpha1.WorkspaceList{}

	labels := map[string]string{
		LabelAccessStrategyName:      accessStrategyName,
		LabelAccessStrategyNamespace: accessStrategyNamespace,
	}

	listOptions := []client.ListOption{
		client.MatchingLabels(labels),
	}

	// Add pagination options if specified
	if limit > 0 {
		listOptions = append(listOptions, client.Limit(limit))
	}
	if continueToken != "" {
		listOptions = append(listOptions, client.Continue(continueToken))
	}

	if err := k8sClient.List(ctx, workspaceList, listOptions...); err != nil {
		return nil, "", fmt.Errorf("failed to list workspaces by AccessStrategy label: %w", err)
	}

	// Filter out workspaces being deleted and verify AccessStrategy reference
	activeWorkspaces := []workspacev1alpha1.Workspace{}
	for _, ws := range workspaceList.Items {
		if !ws.DeletionTimestamp.IsZero() {
			continue // Skip workspaces being deleted
		}

		// Verify AccessStrategy reference matches label to guard against label/spec mismatch
		if ws.Spec.AccessStrategy == nil {
			logger.Info("Workspace has AccessStrategy label but nil AccessStrategy reference - data integrity issue",
				"workspace", ws.Name,
				"workspaceNamespace", ws.Namespace,
				"label", accessStrategyName)
			continue
		}

		if ws.Spec.AccessStrategy.Name != accessStrategyName {
			// This should never happen - log if it occurs
			logger.Info("Workspace has AccessStrategy label but different AccessStrategy name",
				"workspace", ws.Name,
				"workspaceNamespace", ws.Namespace,
				"label", accessStrategyName,
				"spec", ws.Spec.AccessStrategy.Name)
			continue
		}

		// Verify namespace if filtering by namespace
		if accessStrategyNamespace != "" {
			actualNamespace := GetAccessStrategyRefNamespace(&ws)
			if actualNamespace != accessStrategyNamespace {
				logger.V(1).Info("Workspace has AccessStrategy label but different namespace",
					"workspace", ws.Name,
					"workspaceNamespace", ws.Namespace,
					"labelNamespace", accessStrategyNamespace,
					"specNamespace", actualNamespace)
				continue
			}
		}

		activeWorkspaces = append(activeWorkspaces, ws)
	}

	// Extract continuation token for next page
	nextToken := workspaceList.Continue

	return activeWorkspaces, nextToken, nil
}

// GetWorkspaceReconciliationRequestsForAccessStrategy retrieves all active workspaces using the specified AccessStrategy.
// This function retrieves all matching workspaces in a single call without using continuation tokens,
// which is compatible with the controller-runtime cache client.
// Returns a list of Workspace reconciliation requests.
func GetWorkspaceReconciliationRequestsForAccessStrategy(
	ctx context.Context,
	k8sClient client.Client,
	accessStrategyName string,
	accessStrategyNamespace string) ([]reconcile.Request, error) {

	// Get all workspaces in one go without pagination (no token, no limit)
	// This is compatible with the controller-runtime cache client which doesn't support continuation tokens
	workspaces, _, err := ListActiveWorkspacesByAccessStrategy(
		ctx, k8sClient, accessStrategyName, accessStrategyNamespace, "", 0)

	if err != nil {
		return nil, fmt.Errorf("failed to list workspaces by access strategy: %w", err)
	}

	// Convert workspaces to reconcile requests
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
