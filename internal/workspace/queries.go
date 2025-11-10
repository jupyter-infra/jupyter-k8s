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
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// LabelWorkspaceTemplate is the label key for tracking which template a workspace uses
// LabelWorkspaceTemplateNamespace is the label key for tracking which namespace the template reference uses
// These are defined here to avoid import cycles with the controller package
const (
	LabelWorkspaceTemplate          = "workspace.jupyter.org/template"
	LabelWorkspaceTemplateNamespace = "workspace.jupyter.org/template-namespace"
)

// GetTemplateRefNamespace returns the namespace for a workspace's template reference,
// defaulting to the workspace's namespace if not specified
func GetTemplateRefNamespace(ws *workspacev1alpha1.Workspace) string {
	if ws.Spec.TemplateRef == nil || ws.Spec.TemplateRef.Namespace == "" {
		return ws.Namespace
	}
	return ws.Spec.TemplateRef.Namespace
}

// GetWorkspaceKey returns the namespace/name key for a workspace
// This follows Kubernetes convention for logging object references (similar to klog.KObj)
func GetWorkspaceKey(ws *workspacev1alpha1.Workspace) string {
	return fmt.Sprintf("%s/%s", ws.Namespace, ws.Name)
}

// ListByTemplate returns all active workspaces using the specified template
// Supports pagination for large-scale deployments
// Filters out workspaces being deleted (DeletionTimestamp set)
// Validates templateRef matches label to guard against drift
// If templateNamespace is empty, it acts as a wildcard (backwards compatible)
func ListByTemplate(ctx context.Context, k8sClient client.Client, templateName string, templateNamespace string, continueToken string, limit int64) ([]workspacev1alpha1.Workspace, string, error) {
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
				"workspace", GetWorkspaceKey(&ws),
				"label", templateName)
			continue
		}

		if ws.Spec.TemplateRef.Name != templateName {
			// This should never happen - log if it occurs
			logger.Info("Workspace has template label but different templateRef name",
				"workspace", GetWorkspaceKey(&ws),
				"label", templateName,
				"spec", ws.Spec.TemplateRef.Name)
			continue
		}

		// Verify namespace if filtering by namespace
		if templateNamespace != "" {
			actualNamespace := GetTemplateRefNamespace(&ws)
			if actualNamespace != templateNamespace {
				logger.V(1).Info("Workspace has template label but different namespace",
					"workspace", GetWorkspaceKey(&ws),
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

// HasWorkspacesWithTemplate checks if any active workspace uses the specified template
// More efficient than ListByTemplate when only existence check is needed
// Returns true if at least one workspace uses the template
func HasWorkspacesWithTemplate(ctx context.Context, k8sClient client.Client, templateName string) (bool, error) {
	workspaceList := &workspacev1alpha1.WorkspaceList{}
	if err := k8sClient.List(ctx, workspaceList,
		client.MatchingLabels{
			LabelWorkspaceTemplate: templateName,
		},
		client.Limit(1), // Only need to know if ANY exist
	); err != nil {
		return false, fmt.Errorf("failed to check workspaces by template label: %w", err)
	}

	// Check if any non-deleted workspace exists
	for _, ws := range workspaceList.Items {
		if ws.DeletionTimestamp.IsZero() && ws.Spec.TemplateRef != nil && ws.Spec.TemplateRef.Name == templateName {
			return true, nil
		}
	}

	return false, nil
}
