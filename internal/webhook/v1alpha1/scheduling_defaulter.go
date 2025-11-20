/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
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
