/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// validateInitContainers checks if user-specified init containers are allowed by template.
// Rejects when allowCustomInitContainers is false or nil (secure by default).
// Init containers that match the template's defaultInitContainers are not considered user-specified.
func validateInitContainers(initContainers []corev1.Container, template *workspacev1alpha1.WorkspaceTemplate) *TemplateViolation {
	if len(initContainers) == 0 {
		return nil
	}

	// If custom init containers are explicitly allowed, skip validation
	if template.Spec.AllowCustomInitContainers != nil && *template.Spec.AllowCustomInitContainers {
		return nil
	}

	// Check if the init containers are exactly the template defaults (set by defaulter)
	if len(initContainers) == len(template.Spec.DefaultInitContainers) {
		allMatch := true
		for i := range initContainers {
			if initContainers[i].Name != template.Spec.DefaultInitContainers[i].Name ||
				initContainers[i].Image != template.Spec.DefaultInitContainers[i].Image {
				allMatch = false
				break
			}
		}
		if allMatch {
			return nil
		}
	}

	return &TemplateViolation{
		Type:    ViolationTypeInitContainersNotAllowed,
		Field:   "spec.initContainers",
		Message: fmt.Sprintf("Template '%s' does not allow custom init containers (set allowCustomInitContainers: true to enable)", template.Name),
		Allowed: "no custom init containers",
		Actual:  fmt.Sprintf("%d init container(s)", len(initContainers)),
	}
}
