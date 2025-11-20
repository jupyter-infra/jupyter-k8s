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
	"k8s.io/apimachinery/pkg/api/equality"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
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
