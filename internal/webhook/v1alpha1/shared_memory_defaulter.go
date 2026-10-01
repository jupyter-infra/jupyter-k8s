/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// applySharedMemoryDefaults copies the template's defaultSharedMemory onto a workspace that sets none.
func applySharedMemoryDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	if workspace.Spec.SharedMemory == nil && template.Spec.DefaultSharedMemory != nil {
		workspace.Spec.SharedMemory = template.Spec.DefaultSharedMemory.DeepCopy()
	}
}
