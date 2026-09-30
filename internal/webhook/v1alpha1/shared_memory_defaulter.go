/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// applySharedMemoryDefaults fills the workspace's sharedMemory from the template: the whole object
// when the workspace sets none, otherwise each field the workspace left unset.
func applySharedMemoryDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	tpl := template.Spec.SharedMemory
	if tpl == nil {
		return
	}
	if workspace.Spec.SharedMemory == nil {
		workspace.Spec.SharedMemory = tpl.DeepCopy()
		return
	}
	ws := workspace.Spec.SharedMemory
	if ws.Enabled == nil && tpl.Enabled != nil {
		enabled := *tpl.Enabled
		ws.Enabled = &enabled
	}
	if ws.SizeLimit == nil && tpl.SizeLimit != nil {
		sizeLimit := tpl.SizeLimit.DeepCopy()
		ws.SizeLimit = &sizeLimit
	}
}
