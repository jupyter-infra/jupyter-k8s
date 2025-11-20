/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
	workspacequery "github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
)

// applyMetadataDefaults applies metadata defaults (labels and annotations) from template to workspace
func applyMetadataDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	// Add template tracking label
	if workspace.Labels == nil {
		workspace.Labels = make(map[string]string)
	}
	workspace.Labels[controller.LabelWorkspaceTemplate] = template.Name

	// Set template namespace label - use workspace namespace if templateRef.namespace is empty
	// This follows Kubernetes convention (e.g., ConfigMapRef, SecretRef default to same namespace)
	templateNamespace := workspacequery.GetTemplateRefNamespace(workspace)
	workspace.Labels[controller.LabelWorkspaceTemplateNamespace] = templateNamespace
}
