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

// applyStorageDefaults applies storage defaults from template to workspace
func applyStorageDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	if template.Spec.PrimaryStorage == nil {
		return
	}

	// Create storage if it doesn't exist and we have a default size
	if workspace.Spec.Storage == nil && !template.Spec.PrimaryStorage.DefaultSize.IsZero() {
		workspace.Spec.Storage = &workspacev1alpha1.StorageSpec{}
	}

	// Apply individual storage defaults if storage exists
	if workspace.Spec.Storage != nil {
		// Apply default size if not specified
		if workspace.Spec.Storage.Size.IsZero() && !template.Spec.PrimaryStorage.DefaultSize.IsZero() {
			workspace.Spec.Storage.Size = template.Spec.PrimaryStorage.DefaultSize
		}

		// Apply default storage class name if not specified
		if workspace.Spec.Storage.StorageClassName == nil && template.Spec.PrimaryStorage.DefaultStorageClassName != nil {
			workspace.Spec.Storage.StorageClassName = template.Spec.PrimaryStorage.DefaultStorageClassName
		}

		// Apply default mount path if not specified
		if workspace.Spec.Storage.MountPath == "" && template.Spec.PrimaryStorage.DefaultMountPath != "" {
			workspace.Spec.Storage.MountPath = template.Spec.PrimaryStorage.DefaultMountPath
		}
	}
}
