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
	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
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
