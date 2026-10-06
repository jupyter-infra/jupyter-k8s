/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"
	"reflect"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
)

// normalizedSharedMemory returns a copy with enabled written out, so a lock comparison treats {} and
// {enabled: true} as equal while every other field, present or added later, must match exactly.
func normalizedSharedMemory(spec *workspacev1alpha1.SharedMemorySpec) *workspacev1alpha1.SharedMemorySpec {
	if spec == nil {
		return nil
	}
	normalized := spec.DeepCopy()
	enabled := spec.IsEnabled()
	normalized.Enabled = &enabled
	return normalized
}

// validateSharedMemory holds a workspace's sharedMemory to the template's defaultSharedMemory when the
// template's sharedMemoryOverrides lock it. A workspace that sets nothing passes, since the defaulter
// gives it the default.
func validateSharedMemory(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	workspaceSharedMemory := workspace.Spec.SharedMemory
	if workspaceSharedMemory == nil || !template.Spec.SharedMemoryOverrides.Locked() ||
		reflect.DeepEqual(normalizedSharedMemory(workspaceSharedMemory), normalizedSharedMemory(template.Spec.DefaultSharedMemory)) {
		return nil
	}
	return []TemplateViolation{{
		Type:    ViolationTypeSharedMemoryOverrideNotAllowed,
		Field:   "spec.sharedMemory",
		Message: fmt.Sprintf("Template '%s' does not allow overriding shared memory, but the workspace sets its own", template.Name),
		Allowed: fmt.Sprintf("enabled: %t, the template's defaultSharedMemory", template.Spec.DefaultSharedMemory.IsEnabled()),
		Actual:  fmt.Sprintf("enabled: %t", workspaceSharedMemory.IsEnabled()),
	}}
}

// validateSharedMemoryVolumes rejects a volume the workspace mounts at /dev/shm itself when the template
// locks shared memory, since such a volume replaces the operator's and would bypass the lock.
func validateSharedMemoryVolumes(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	if !template.Spec.SharedMemoryOverrides.Locked() {
		return nil
	}
	var violations []TemplateViolation
	for _, vol := range workspace.Spec.Volumes {
		if !controller.MountsSharedMemoryPath(vol.MountPath) {
			continue
		}
		violations = append(violations, TemplateViolation{
			Type:    ViolationTypeSharedMemoryOverrideNotAllowed,
			Field:   fmt.Sprintf("spec.volumes[%s].mountPath", vol.Name),
			Message: fmt.Sprintf("Template '%s' does not allow overriding shared memory, but the workspace mounts volume '%s' at /dev/shm", template.Name, vol.Name),
			Allowed: "no volume at /dev/shm",
			Actual:  vol.MountPath,
		})
	}
	return violations
}
