/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
)

// validateSharedMemory keeps a workspace's sharedMemory within the template's: the workspace may not
// enable the /dev/shm volume when the template disables it, nor set a sizeLimit above the template's.
// An unset workspace sizeLimit means the container memory limit, so it counts as exceeding a template
// sizeLimit.
func validateSharedMemory(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	tpl := template.Spec.SharedMemory
	ws := workspace.Spec.SharedMemory
	if tpl == nil || ws == nil {
		return nil
	}

	var violations []TemplateViolation
	if tpl.Enabled != nil && !*tpl.Enabled && (ws.Enabled == nil || *ws.Enabled) {
		violations = append(violations, TemplateViolation{
			Type:    ViolationTypeSharedMemoryNotAllowed,
			Field:   "spec.sharedMemory.enabled",
			Message: fmt.Sprintf("Template '%s' disables the /dev/shm volume, but the workspace enables it", template.Name),
			Allowed: "false",
			Actual:  "true",
		})
	}

	if ws.Enabled != nil && !*ws.Enabled {
		return violations
	}

	if tpl.SizeLimit != nil {
		switch {
		case ws.SizeLimit == nil:
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeSharedMemoryExceeded,
				Field:   "spec.sharedMemory.sizeLimit",
				Message: fmt.Sprintf("Template '%s' limits the /dev/shm volume to %s, but the workspace sets no sizeLimit, which means the container memory limit", template.Name, tpl.SizeLimit.String()),
				Allowed: "<= " + tpl.SizeLimit.String(),
				Actual:  "unset",
			})
		case ws.SizeLimit.Cmp(*tpl.SizeLimit) > 0:
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeSharedMemoryExceeded,
				Field:   "spec.sharedMemory.sizeLimit",
				Message: fmt.Sprintf("Workspace /dev/shm sizeLimit %s exceeds template '%s' sizeLimit %s", ws.SizeLimit.String(), template.Name, tpl.SizeLimit.String()),
				Allowed: "<= " + tpl.SizeLimit.String(),
				Actual:  ws.SizeLimit.String(),
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

// validateSharedMemoryVolumes applies the template's sharedMemory to volumes the workspace itself mounts
// at /dev/shm, since such a volume replaces the operator's: none is allowed when the template disables
// the volume, and under a template sizeLimit it must be a memory-backed emptyDir whose sizeLimit does not
// exceed the template's, so a workspace cannot get an unbounded /dev/shm by declaring its own volume.
func validateSharedMemoryVolumes(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	tpl := template.Spec.SharedMemory
	if tpl == nil {
		return nil
	}

	var violations []TemplateViolation
	for _, vol := range workspace.Spec.Volumes {
		if !controller.MountsSharedMemoryPath(vol.MountPath) {
			continue
		}
		if tpl.Enabled != nil && !*tpl.Enabled {
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeSharedMemoryNotAllowed,
				Field:   fmt.Sprintf("spec.volumes[%s].mountPath", vol.Name),
				Message: fmt.Sprintf("Template '%s' disables the /dev/shm volume, but the workspace mounts volume '%s' at /dev/shm", template.Name, vol.Name),
				Allowed: "no volume at /dev/shm",
				Actual:  vol.MountPath,
			})
			continue
		}
		if tpl.SizeLimit == nil {
			continue
		}
		field := fmt.Sprintf("spec.volumes[%s].emptyDir.sizeLimit", vol.Name)
		switch {
		case vol.EmptyDir == nil || vol.EmptyDir.Medium != corev1.StorageMediumMemory || vol.EmptyDir.SizeLimit == nil:
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeSharedMemoryExceeded,
				Field:   field,
				Message: fmt.Sprintf("Template '%s' limits /dev/shm to %s, but volume '%s' at /dev/shm is not a memory-backed emptyDir with a sizeLimit", template.Name, tpl.SizeLimit.String(), vol.Name),
				Allowed: "emptyDir with medium Memory and sizeLimit <= " + tpl.SizeLimit.String(),
				Actual:  actualUnbounded,
			})
		case vol.EmptyDir.SizeLimit.Cmp(*tpl.SizeLimit) > 0:
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeSharedMemoryExceeded,
				Field:   field,
				Message: fmt.Sprintf("Volume '%s' at /dev/shm has sizeLimit %s, which exceeds template '%s' sizeLimit %s", vol.Name, vol.EmptyDir.SizeLimit.String(), template.Name, tpl.SizeLimit.String()),
				Allowed: "<= " + tpl.SizeLimit.String(),
				Actual:  vol.EmptyDir.SizeLimit.String(),
			})
		}
	}
	return violations
}
