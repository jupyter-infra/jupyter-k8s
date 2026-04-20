/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// applyInitContainerDefaults applies default init containers from template to workspace
func applyInitContainerDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	if len(workspace.Spec.InitContainers) > 0 || len(template.Spec.DefaultInitContainers) == 0 {
		return
	}

	workspace.Spec.InitContainers = make([]corev1.Container, len(template.Spec.DefaultInitContainers))
	copy(workspace.Spec.InitContainers, template.Spec.DefaultInitContainers)
}
