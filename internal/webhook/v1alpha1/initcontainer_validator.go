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

// validateInitContainers checks if init containers are allowed by template
func validateInitContainers(initContainers []corev1.Container, template *workspacev1alpha1.WorkspaceTemplate) *TemplateViolation {
	if len(initContainers) == 0 {
		return nil
	}

	if template.Spec.AllowInitContainers != nil && !*template.Spec.AllowInitContainers {
		return &TemplateViolation{
			Type:    ViolationTypeInitContainersNotAllowed,
			Field:   "spec.initContainers",
			Message: fmt.Sprintf("Template '%s' does not allow init containers, but workspace specifies %d init container(s)", template.Name, len(initContainers)),
			Allowed: "no init containers",
			Actual:  fmt.Sprintf("%d init container(s)", len(initContainers)),
		}
	}

	return nil
}
