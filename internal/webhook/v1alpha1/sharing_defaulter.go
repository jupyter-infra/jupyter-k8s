package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	webhookconst "github.com/jupyter-infra/jupyter-k8s/internal/webhook"
)

// setWorkspaceDefaults sets default values for OwnershipType and AccessType
// If OwnershipType is not set, we default to Public.
// If AccessType is not set, we default to OwnershipType value
func setWorkspaceSharingDefaults(workspace *workspacev1alpha1.Workspace) {
	if workspace.Spec.OwnershipType == "" {
		workspace.Spec.OwnershipType = webhookconst.OwnershipTypePublic
	}

	if workspace.Spec.AccessType == "" {
		workspace.Spec.AccessType = workspace.Spec.OwnershipType
	}
}
