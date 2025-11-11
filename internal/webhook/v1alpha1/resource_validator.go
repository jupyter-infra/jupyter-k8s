/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

// validateResourceBounds checks if resources are within template bounds
func validateResourceBounds(resources corev1.ResourceRequirements, template *workspacev1alpha1.WorkspaceTemplate) []controller.TemplateViolation {
	var violations []controller.TemplateViolation

	// Validate limits >= requests
	if resources.Requests != nil && resources.Limits != nil {
		if cpuRequest, hasRequest := resources.Requests[corev1.ResourceCPU]; hasRequest {
			if cpuLimit, hasLimit := resources.Limits[corev1.ResourceCPU]; hasLimit {
				if cpuLimit.Cmp(cpuRequest) < 0 {
					violations = append(violations, controller.TemplateViolation{
						Type:    controller.ViolationTypeResourceExceeded,
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
					violations = append(violations, controller.TemplateViolation{
						Type:    controller.ViolationTypeResourceExceeded,
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

	// Validate CPU bounds
	if bounds.CPU != nil && resources.Requests != nil {
		if cpuRequest, exists := resources.Requests[corev1.ResourceCPU]; exists {
			if cpuRequest.Cmp(bounds.CPU.Min) < 0 {
				violations = append(violations, controller.TemplateViolation{
					Type:    controller.ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.cpu",
					Message: fmt.Sprintf("CPU request %s is below minimum %s required by template '%s'", cpuRequest.String(), bounds.CPU.Min.String(), template.Name),
					Allowed: fmt.Sprintf("min: %s", bounds.CPU.Min.String()),
					Actual:  cpuRequest.String(),
				})
			}
			if cpuRequest.Cmp(bounds.CPU.Max) > 0 {
				violations = append(violations, controller.TemplateViolation{
					Type:    controller.ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.cpu",
					Message: fmt.Sprintf("CPU request %s exceeds maximum %s allowed by template '%s'", cpuRequest.String(), bounds.CPU.Max.String(), template.Name),
					Allowed: fmt.Sprintf("max: %s", bounds.CPU.Max.String()),
					Actual:  cpuRequest.String(),
				})
			}
		}
	}

	// Validate memory bounds
	if bounds.Memory != nil && resources.Requests != nil {
		if memRequest, exists := resources.Requests[corev1.ResourceMemory]; exists {
			if memRequest.Cmp(bounds.Memory.Min) < 0 {
				violations = append(violations, controller.TemplateViolation{
					Type:    controller.ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.memory",
					Message: fmt.Sprintf("Memory request %s is below minimum %s required by template '%s'", memRequest.String(), bounds.Memory.Min.String(), template.Name),
					Allowed: fmt.Sprintf("min: %s", bounds.Memory.Min.String()),
					Actual:  memRequest.String(),
				})
			}
			if memRequest.Cmp(bounds.Memory.Max) > 0 {
				violations = append(violations, controller.TemplateViolation{
					Type:    controller.ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.memory",
					Message: fmt.Sprintf("Memory request %s exceeds maximum %s allowed by template '%s'", memRequest.String(), bounds.Memory.Max.String(), template.Name),
					Allowed: fmt.Sprintf("max: %s", bounds.Memory.Max.String()),
					Actual:  memRequest.String(),
				})
			}
		}
	}

	// Validate GPU bounds
	if bounds.GPU != nil && resources.Requests != nil {
		gpuResourceName := corev1.ResourceName("nvidia.com/gpu")
		if gpuRequest, exists := resources.Requests[gpuResourceName]; exists {
			if gpuRequest.Cmp(bounds.GPU.Min) < 0 {
				violations = append(violations, controller.TemplateViolation{
					Type:    controller.ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.nvidia.com/gpu",
					Message: fmt.Sprintf("GPU request %s is below minimum %s required by template '%s'", gpuRequest.String(), bounds.GPU.Min.String(), template.Name),
					Allowed: fmt.Sprintf("min: %s", bounds.GPU.Min.String()),
					Actual:  gpuRequest.String(),
				})
			}
			if gpuRequest.Cmp(bounds.GPU.Max) > 0 {
				violations = append(violations, controller.TemplateViolation{
					Type:    controller.ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.nvidia.com/gpu",
					Message: fmt.Sprintf("GPU request %s exceeds maximum %s allowed by template '%s'", gpuRequest.String(), bounds.GPU.Max.String(), template.Name),
					Allowed: fmt.Sprintf("max: %s", bounds.GPU.Max.String()),
					Actual:  gpuRequest.String(),
				})
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
