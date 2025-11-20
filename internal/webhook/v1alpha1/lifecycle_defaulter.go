/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// applyLifecycleDefaults applies lifecycle-related defaults from template to workspace
func applyLifecycleDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	// Apply lifecycle defaults
	if workspace.Spec.Lifecycle == nil && template.Spec.DefaultLifecycle != nil {
		workspace.Spec.Lifecycle = template.Spec.DefaultLifecycle.DeepCopy()
	}

	// Apply idle shutdown defaults
	if workspace.Spec.IdleShutdown == nil && template.Spec.DefaultIdleShutdown != nil {
		workspace.Spec.IdleShutdown = template.Spec.DefaultIdleShutdown.DeepCopy()
	}
}
