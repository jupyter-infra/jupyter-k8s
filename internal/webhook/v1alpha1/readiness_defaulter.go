/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// applyReadinessProbeDefaults applies the readiness probe default from template
// to workspace when the workspace does not specify its own.
func applyReadinessProbeDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	if workspace.Spec.ReadinessProbe == nil && template.Spec.DefaultReadinessProbe != nil {
		workspace.Spec.ReadinessProbe = template.Spec.DefaultReadinessProbe.DeepCopy()
	}
}
