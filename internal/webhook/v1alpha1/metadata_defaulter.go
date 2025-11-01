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
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

// applyMetadataDefaults applies metadata defaults (labels and annotations) from template to workspace
func applyMetadataDefaults(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) {
	// Add template tracking label
	if workspace.Labels == nil {
		workspace.Labels = make(map[string]string)
	}
	workspace.Labels[controller.LabelWorkspaceTemplate] = template.Name
}
