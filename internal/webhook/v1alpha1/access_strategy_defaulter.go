/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// applyAccessStrategyDefaults applies access strategy defaults from template to workspace
func applyAccessStrategyDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	if workspace.Spec.AccessStrategy == nil && template.Spec.DefaultAccessStrategy != nil {
		workspace.Spec.AccessStrategy = template.Spec.DefaultAccessStrategy.DeepCopy()
	}
}
