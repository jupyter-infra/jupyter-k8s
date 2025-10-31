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

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// TemplateResolver handles resolving WorkspaceTemplate references and applying overrides
type TemplateResolver struct {
	client client.Client
}

// NewTemplateResolver creates a new TemplateResolver
func NewTemplateResolver(k8sClient client.Client) *TemplateResolver {
	return &TemplateResolver{
		client: k8sClient,
	}
}

// ResolvedTemplate contains the resolved template configuration
type ResolvedTemplate struct {
	Image                     string
	Resources                 corev1.ResourceRequirements
	EnvironmentVariables      []corev1.EnvVar
	StorageConfiguration      *workspacev1alpha1.StorageConfig
	AllowSecondaryStorages    bool
	NodeSelector              map[string]string
	Affinity                  *corev1.Affinity
	Tolerations               []corev1.Toleration
	ContainerConfig           *workspacev1alpha1.ContainerConfig
	IdleShutdown              *workspacev1alpha1.IdleShutdownSpec
	AllowIdleShutdownOverride bool
}

// ListWorkspacesUsingTemplate returns all workspaces that reference the specified template
// Excludes workspaces that are being deleted (have deletionTimestamp set) to follow
// Kubernetes best practices for finalizer management and garbage collection
func (tr *TemplateResolver) ListWorkspacesUsingTemplate(ctx context.Context, templateName string) ([]workspacev1alpha1.Workspace, error) {
	logger := logf.FromContext(ctx)

	// Use label selector for fast, efficient lookup
	workspaceList := &workspacev1alpha1.WorkspaceList{}
	if err := tr.client.List(ctx, workspaceList, client.MatchingLabels{
		"workspace.jupyter.org/template": templateName,
	}); err != nil {
		return nil, fmt.Errorf("failed to list workspaces by template label: %w", err)
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
			logger.V(1).Info("Workspace has template label but nil templateRef",
				"workspace", fmt.Sprintf("%s/%s", ws.Namespace, ws.Name),
				"label", templateName)
			continue
		}

		if ws.Spec.TemplateRef.Name != templateName {
			// This should never happen - log if it occurs
			logger.Info("Workspace has template label but different templateRef",
				"workspace", fmt.Sprintf("%s/%s", ws.Namespace, ws.Name),
				"label", templateName,
				"spec", ws.Spec.TemplateRef.Name)
			continue
		}

		activeWorkspaces = append(activeWorkspaces, ws)
	}

	return activeWorkspaces, nil
}

// ResolveContainerCommand returns the container command from workspace or template
func ResolveContainerCommand(workspace *workspacev1alpha1.Workspace, template *ResolvedTemplate) []string {
	if workspace.Spec.ContainerConfig != nil && len(workspace.Spec.ContainerConfig.Command) > 0 {
		return workspace.Spec.ContainerConfig.Command
	}
	if template != nil && template.ContainerConfig != nil && len(template.ContainerConfig.Command) > 0 {
		return template.ContainerConfig.Command
	}
	return nil
}

// ResolveContainerArgs returns the container args from workspace or template
func ResolveContainerArgs(workspace *workspacev1alpha1.Workspace, template *ResolvedTemplate) []string {
	if workspace.Spec.ContainerConfig != nil && len(workspace.Spec.ContainerConfig.Args) > 0 {
		return workspace.Spec.ContainerConfig.Args
	}
	if template != nil && template.ContainerConfig != nil && len(template.ContainerConfig.Args) > 0 {
		return template.ContainerConfig.Args
	}
	return nil
}
