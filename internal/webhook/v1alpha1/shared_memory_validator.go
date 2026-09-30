/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// validateSharedMemory keeps a workspace's sharedMemory within the template's: the workspace may not
// enable the /dev/shm volume when the template disables it, nor exceed the template's sizeLimit cap.
// An unset workspace sizeLimit means the container memory limit, so under a cap it counts as exceeding.
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
				Message: fmt.Sprintf("Template '%s' caps the /dev/shm volume at %s, but the workspace sets no sizeLimit, which means the container memory limit", template.Name, tpl.SizeLimit.String()),
				Allowed: "<= " + tpl.SizeLimit.String(),
				Actual:  "unset",
			})
		case ws.SizeLimit.Cmp(*tpl.SizeLimit) > 0:
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeSharedMemoryExceeded,
				Field:   "spec.sharedMemory.sizeLimit",
				Message: fmt.Sprintf("Workspace /dev/shm sizeLimit %s exceeds template '%s' cap %s", ws.SizeLimit.String(), template.Name, tpl.SizeLimit.String()),
				Allowed: "<= " + tpl.SizeLimit.String(),
				Actual:  ws.SizeLimit.String(),
			})
		}
	}

	return violations
}

// validateSharedMemorySpec rejects a sharedMemory.sizeLimit of zero or less, which the kubelet
// ignores, leaving the volume capped only by the container memory limit.
func validateSharedMemorySpec(workspace *workspacev1alpha1.Workspace) error {
	sm := workspace.Spec.SharedMemory
	if sm == nil || sm.SizeLimit == nil || sm.SizeLimit.Sign() > 0 {
		return nil
	}
	return fmt.Errorf("spec.sharedMemory.sizeLimit %s must be greater than zero", sm.SizeLimit.String())
}
