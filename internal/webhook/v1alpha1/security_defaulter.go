package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// applySecurityDefaults applies security-related defaults from template to workspace
func applySecurityDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	// Apply pod security context defaults
	if workspace.Spec.PodSecurityContext == nil && template.Spec.DefaultPodSecurityContext != nil {
		workspace.Spec.PodSecurityContext = template.Spec.DefaultPodSecurityContext.DeepCopy()
	}
}
