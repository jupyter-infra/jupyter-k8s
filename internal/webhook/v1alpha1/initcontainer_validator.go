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
)

// validateInitContainers checks if user-specified init containers are allowed by template.
// Rejects when allowCustomInitContainers is false or nil (secure by default).
// Init containers that match the template's defaultInitContainers (by name, deep equality)
// are not considered user-specified.
func validateInitContainers(initContainers []corev1.Container, template *workspacev1alpha1.WorkspaceTemplate) *TemplateViolation {
	if len(initContainers) == 0 {
		return nil
	}

	// If custom init containers are explicitly allowed, skip validation
	if template.Spec.AllowCustomInitContainers != nil && *template.Spec.AllowCustomInitContainers {
		return nil
	}

	// Check if all init containers match template defaults by name with deep equality
	if matchesTemplateDefaults(initContainers, template.Spec.DefaultInitContainers) {
		return nil
	}

	return &TemplateViolation{
		Type:    ViolationTypeInitContainersNotAllowed,
		Field:   "spec.initContainers",
		Message: fmt.Sprintf("Template '%s' does not allow custom init containers (set allowCustomInitContainers: true to enable)", template.Name),
		Allowed: "no custom init containers",
		Actual:  fmt.Sprintf("%d init container(s)", len(initContainers)),
	}
}

// matchesTemplateDefaults checks if the workspace init containers exactly match
// the template defaults. Matches by container name using deep equality on the
// full container spec. Both sets must have the same length.
func matchesTemplateDefaults(initContainers []corev1.Container, defaults []corev1.Container) bool {
	if len(initContainers) != len(defaults) {
		return false
	}

	// Build lookup of template defaults by name
	defaultsByName := make(map[string]corev1.Container, len(defaults))
	for _, d := range defaults {
		defaultsByName[d.Name] = d
	}

	for _, ic := range initContainers {
		defaultContainer, exists := defaultsByName[ic.Name]
		if !exists {
			return false
		}
		if !equality.Semantic.DeepEqual(ic, defaultContainer) {
			return false
		}
	}

	return true
}
