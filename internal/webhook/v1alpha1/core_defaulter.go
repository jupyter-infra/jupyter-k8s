package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// applyCoreDefaults applies core workspace defaults from template to workspace
func applyCoreDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	// Apply image defaults
	if workspace.Spec.Image == "" && template.Spec.DefaultImage != "" {
		workspace.Spec.Image = template.Spec.DefaultImage
	}

	// Apply ownership type defaults
	if workspace.Spec.OwnershipType == "" && template.Spec.DefaultOwnershipType != "" {
		workspace.Spec.OwnershipType = template.Spec.DefaultOwnershipType
	}

	// Apply container config defaults
	if workspace.Spec.ContainerConfig == nil && template.Spec.DefaultContainerConfig != nil {
		workspace.Spec.ContainerConfig = template.Spec.DefaultContainerConfig.DeepCopy()
	}

	// Apply access type defaults
	if workspace.Spec.AccessType == "" && template.Spec.DefaultAccessType != "" {
		workspace.Spec.AccessType = template.Spec.DefaultAccessType
	}

	// Apply app type defaults
	if workspace.Spec.AppType == "" && template.Spec.AppType != "" {
		workspace.Spec.AppType = template.Spec.AppType
	}
}
