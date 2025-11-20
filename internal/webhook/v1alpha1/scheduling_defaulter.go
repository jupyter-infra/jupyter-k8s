/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// applySchedulingDefaults applies scheduling-related defaults from template to workspace
func applySchedulingDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	// Apply node selector defaults
	if workspace.Spec.NodeSelector == nil && template.Spec.DefaultNodeSelector != nil {
		workspace.Spec.NodeSelector = make(map[string]string)
		for k, v := range template.Spec.DefaultNodeSelector {
			workspace.Spec.NodeSelector[k] = v
		}
	}

	// Apply affinity defaults
	if workspace.Spec.Affinity == nil && template.Spec.DefaultAffinity != nil {
		workspace.Spec.Affinity = template.Spec.DefaultAffinity.DeepCopy()
	}

	// Apply tolerations defaults
	if workspace.Spec.Tolerations == nil && template.Spec.DefaultTolerations != nil {
		workspace.Spec.Tolerations = make([]corev1.Toleration, len(template.Spec.DefaultTolerations))
		copy(workspace.Spec.Tolerations, template.Spec.DefaultTolerations)
	}
}
