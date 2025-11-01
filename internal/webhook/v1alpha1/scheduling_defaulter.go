/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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
