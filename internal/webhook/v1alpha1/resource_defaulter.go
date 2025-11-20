/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// applyResourceDefaults applies resource defaults from template to workspace
func applyResourceDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	if workspace.Spec.Resources == nil && template.Spec.DefaultResources != nil {
		workspace.Spec.Resources = template.Spec.DefaultResources.DeepCopy()
	}
}
