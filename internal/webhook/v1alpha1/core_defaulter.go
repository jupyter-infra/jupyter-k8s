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
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// applyCoreDefaults applies core workspace defaults from template to workspace
func applyCoreDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	// Apply image defaults
	if workspace.Spec.Image == "" && template.Spec.DefaultImage != "" {
		workspace.Spec.Image = template.Spec.DefaultImage
	}

	// Apply ownership type defaults
	if workspace.Spec.OwnershipType == "" && template.Spec.DefaultOwnershipType != "" {
		workspace.Spec.OwnershipType = template.Spec.DefaultOwnershipType
	}

	// Apply container config defaults
	if workspace.Spec.ContainerConfig == nil && template.Spec.DefaultContainerConfig != nil {
		workspace.Spec.ContainerConfig = template.Spec.DefaultContainerConfig.DeepCopy()
	}

	// Apply access type defaults
	if workspace.Spec.AccessType == "" && template.Spec.DefaultAccessType != "" {
		workspace.Spec.AccessType = template.Spec.DefaultAccessType
	}

	// Apply app type defaults
	if workspace.Spec.AppType == "" && template.Spec.AppType != "" {
		workspace.Spec.AppType = template.Spec.AppType
	}
}
