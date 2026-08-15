/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
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

// integrationRefsChanged detects whether spec.integrationTemplateRefs differs between old and new.
// Used on update to run integration-ref validation only when the user actually changed the refs --
// mirroring how template validation gates on specChanged. This is what keeps an admin's out-of-band
// template edit (e.g. adding a required parameter) or deletion from wedging reconciliation of an
// already-referencing workspace: the controller's own finalizer/label updates leave the refs untouched,
// so they skip the check. Uses semantic equality (nil slice == empty slice) like the sibling comparators.
func integrationRefsChanged(oldSpec, newSpec *workspacev1alpha1.WorkspaceSpec) bool {
	return !equality.Semantic.DeepEqual(oldSpec.IntegrationTemplateRefs, newSpec.IntegrationTemplateRefs)
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
