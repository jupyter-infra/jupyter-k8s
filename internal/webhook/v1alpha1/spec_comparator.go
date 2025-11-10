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
	"k8s.io/apimachinery/pkg/api/equality"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// specChanged detects if any spec field changed between old and new workspace
// This is used to determine if full template validation is needed on update
// Uses semantic equality which properly handles resource.Quantity and other K8s types
func specChanged(oldSpec, newSpec *workspacev1alpha1.WorkspaceSpec) bool {
	return !equality.Semantic.DeepEqual(oldSpec, newSpec)
}

// onlyDesiredStatusChanged checks if DesiredStatus is the ONLY spec field that changed
// Used to determine if stopping a workspace should bypass validation
// Returns true only when DesiredStatus changed and all other fields remain unchanged
func onlyDesiredStatusChanged(oldSpec, newSpec *workspacev1alpha1.WorkspaceSpec) bool {
	// DesiredStatus must have changed
	if oldSpec.DesiredStatus == newSpec.DesiredStatus {
		return false
	}

	// Check if everything else stayed the same by temporarily setting DesiredStatus to match
	oldCopy := oldSpec.DeepCopy()
	oldCopy.DesiredStatus = newSpec.DesiredStatus
	return equality.Semantic.DeepEqual(oldCopy, newSpec)
}
