/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
)

// sharedMemoryOverridesAllowed reports whether the template lets workspaces deviate from its default.
func sharedMemoryOverridesAllowed(policy *workspacev1alpha1.SharedMemoryOverridePolicy) bool {
	return policy.Allow == nil || *policy.Allow
}

// validateSharedMemory keeps a workspace's sharedMemory within the template's sharedMemoryOverrides:
// with allow false it must equal the template's defaultSharedMemory, and under maxSizeLimit an enabled
// volume must carry a sizeLimit at or below the maximum, since an unset sizeLimit means the container
// memory limit.
func validateSharedMemory(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	policy := template.Spec.SharedMemoryOverrides
	ws := workspace.Spec.SharedMemory
	if policy == nil || ws == nil {
		return nil
	}

	var violations []TemplateViolation
	if !sharedMemoryOverridesAllowed(policy) && !equality.Semantic.DeepEqual(ws, template.Spec.DefaultSharedMemory) {
		violations = append(violations, TemplateViolation{
			Type:    ViolationTypeSharedMemoryOverrideNotAllowed,
			Field:   "spec.sharedMemory",
			Message: fmt.Sprintf("Template '%s' does not allow overriding shared memory, but the workspace sets its own", template.Name),
			Allowed: "the template's defaultSharedMemory",
			Actual:  "a different setting",
		})
	}

	if ws.Enabled != nil && !*ws.Enabled {
		return violations
	}

	if policy.MaxSizeLimit != nil {
		switch {
		case ws.SizeLimit == nil:
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeSharedMemoryExceeded,
				Field:   "spec.sharedMemory.sizeLimit",
				Message: fmt.Sprintf("Template '%s' limits /dev/shm to %s, but the workspace sets no sizeLimit, which means the container memory limit", template.Name, policy.MaxSizeLimit.String()),
				Allowed: "<= " + policy.MaxSizeLimit.String(),
				Actual:  "unset",
			})
		case ws.SizeLimit.Cmp(*policy.MaxSizeLimit) > 0:
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeSharedMemoryExceeded,
				Field:   "spec.sharedMemory.sizeLimit",
				Message: fmt.Sprintf("Workspace /dev/shm sizeLimit %s exceeds template '%s' maxSizeLimit %s", ws.SizeLimit.String(), template.Name, policy.MaxSizeLimit.String()),
				Allowed: "<= " + policy.MaxSizeLimit.String(),
				Actual:  ws.SizeLimit.String(),
			})
		}
	}

	return violations
}

// validateSharedMemoryVolumes applies the template's sharedMemoryOverrides to volumes the workspace itself
// mounts at /dev/shm, since such a volume replaces the operator's: none when overrides are not allowed,
// and under maxSizeLimit only a memory-backed emptyDir whose sizeLimit does not exceed it, so a workspace
// cannot get an unbounded /dev/shm by declaring its own volume.
func validateSharedMemoryVolumes(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	policy := template.Spec.SharedMemoryOverrides
	if policy == nil {
		return nil
	}

	var violations []TemplateViolation
	for _, vol := range workspace.Spec.Volumes {
		if !controller.MountsSharedMemoryPath(vol.MountPath) {
			continue
		}
		if !sharedMemoryOverridesAllowed(policy) {
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeSharedMemoryOverrideNotAllowed,
				Field:   fmt.Sprintf("spec.volumes[%s].mountPath", vol.Name),
				Message: fmt.Sprintf("Template '%s' does not allow overriding shared memory, but the workspace mounts volume '%s' at /dev/shm", template.Name, vol.Name),
				Allowed: "no volume at /dev/shm",
				Actual:  vol.MountPath,
			})
			continue
		}
		if policy.MaxSizeLimit == nil {
			continue
		}
		field := fmt.Sprintf("spec.volumes[%s].emptyDir.sizeLimit", vol.Name)
		switch {
		case vol.EmptyDir == nil || vol.EmptyDir.Medium != corev1.StorageMediumMemory || vol.EmptyDir.SizeLimit == nil:
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeSharedMemoryExceeded,
				Field:   field,
				Message: fmt.Sprintf("Template '%s' limits /dev/shm to %s, but volume '%s' at /dev/shm is not a memory-backed emptyDir with a sizeLimit", template.Name, policy.MaxSizeLimit.String(), vol.Name),
				Allowed: "emptyDir with medium Memory and sizeLimit <= " + policy.MaxSizeLimit.String(),
				Actual:  actualUnbounded,
			})
		case vol.EmptyDir.SizeLimit.Cmp(*policy.MaxSizeLimit) > 0:
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeSharedMemoryExceeded,
				Field:   field,
				Message: fmt.Sprintf("Volume '%s' at /dev/shm has sizeLimit %s, which exceeds template '%s' maxSizeLimit %s", vol.Name, vol.EmptyDir.SizeLimit.String(), template.Name, policy.MaxSizeLimit.String()),
				Allowed: "<= " + policy.MaxSizeLimit.String(),
				Actual:  vol.EmptyDir.SizeLimit.String(),
			})
		}
	}
	return violations
}

// validateSharedMemorySpec rejects a sharedMemory.sizeLimit of zero or less, which the kubelet
// ignores, leaving the volume bounded only by the pod's memory limit.
func validateSharedMemorySpec(workspace *workspacev1alpha1.Workspace) error {
	sm := workspace.Spec.SharedMemory
	if sm == nil || sm.SizeLimit == nil || sm.SizeLimit.Sign() > 0 {
		return nil
	}
	return fmt.Errorf("spec.sharedMemory.sizeLimit %s must be greater than zero", sm.SizeLimit.String())
}
