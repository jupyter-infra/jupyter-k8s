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

// validateResourceBounds checks if resources are within template bounds
func validateResourceBounds(resources corev1.ResourceRequirements, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	var violations []TemplateViolation

	// Validate limits >= requests
	if resources.Requests != nil && resources.Limits != nil {
		if cpuRequest, hasRequest := resources.Requests[corev1.ResourceCPU]; hasRequest {
			if cpuLimit, hasLimit := resources.Limits[corev1.ResourceCPU]; hasLimit {
				if cpuLimit.Cmp(cpuRequest) < 0 {
					violations = append(violations, TemplateViolation{
						Type:    ViolationTypeResourceExceeded,
						Field:   "spec.resources.limits.cpu",
						Message: "CPU limit must be greater than or equal to CPU request",
						Allowed: cpuRequest.String(),
						Actual:  cpuLimit.String(),
					})
				}
			}
		}
		if memRequest, hasRequest := resources.Requests[corev1.ResourceMemory]; hasRequest {
			if memLimit, hasLimit := resources.Limits[corev1.ResourceMemory]; hasLimit {
				if memLimit.Cmp(memRequest) < 0 {
					violations = append(violations, TemplateViolation{
						Type:    ViolationTypeResourceExceeded,
						Field:   "spec.resources.limits.memory",
						Message: "Memory limit must be greater than or equal to memory request",
						Allowed: memRequest.String(),
						Actual:  memLimit.String(),
					})
				}
			}
		}
	}

	bounds := template.Spec.ResourceBounds
	if bounds == nil {
		return violations
	}

	// Validate resource bounds - iterate over all bounded resources
	if bounds.Resources != nil && resources.Requests != nil {
		for resourceName, resourceRange := range bounds.Resources {
			if request, exists := resources.Requests[resourceName]; exists {
				// Validate minimum bound
				if request.Cmp(resourceRange.Min) < 0 {
					violations = append(violations, TemplateViolation{
						Type:    ViolationTypeResourceExceeded,
						Field:   fmt.Sprintf("spec.resources.requests.%s", resourceName),
						Message: fmt.Sprintf("%s request %s is below minimum %s required by template '%s'", resourceName, request.String(), resourceRange.Min.String(), template.Name),
						Allowed: fmt.Sprintf("min: %s", resourceRange.Min.String()),
						Actual:  request.String(),
					})
				}
				// Validate maximum bound
				if request.Cmp(resourceRange.Max) > 0 {
					violations = append(violations, TemplateViolation{
						Type:    ViolationTypeResourceExceeded,
						Field:   fmt.Sprintf("spec.resources.requests.%s", resourceName),
						Message: fmt.Sprintf("%s request %s exceeds maximum %s allowed by template '%s'", resourceName, request.String(), resourceRange.Max.String(), template.Name),
						Allowed: fmt.Sprintf("max: %s", resourceRange.Max.String()),
						Actual:  request.String(),
					})
				}
			}
		}
	}

	return violations
}

// resourcesEqual compares two ResourceRequirements for equality
func resourcesEqual(old, new *corev1.ResourceRequirements) bool {
	if old == nil && new == nil {
		return true
	}
	if old == nil || new == nil {
		return false
	}

	return resourceListEqual(old.Requests, new.Requests) && resourceListEqual(old.Limits, new.Limits)
}

// resourceListEqual compares two ResourceList for equality
func resourceListEqual(old, new corev1.ResourceList) bool {
	if len(old) != len(new) {
		return false
	}

	for key, oldVal := range old {
		newVal, exists := new[key]
		if !exists || !oldVal.Equal(newVal) {
			return false
		}
	}

	return true
}
