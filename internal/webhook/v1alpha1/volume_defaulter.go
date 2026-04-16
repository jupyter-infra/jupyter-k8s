/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// applyVolumeDefaults applies default volumes from template to workspace
func applyVolumeDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	if len(workspace.Spec.Volumes) > 0 || len(template.Spec.DefaultVolumes) == 0 {
		return
	}

	workspace.Spec.Volumes = make([]workspacev1alpha1.VolumeSpec, len(template.Spec.DefaultVolumes))
	copy(workspace.Spec.Volumes, template.Spec.DefaultVolumes)
}
