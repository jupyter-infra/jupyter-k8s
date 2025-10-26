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
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

// TemplateValidator handles template validation for webhooks
type TemplateValidator struct {
	client client.Client
}

// NewTemplateValidator creates a new TemplateValidator
func NewTemplateValidator(k8sClient client.Client) *TemplateValidator {
	return &TemplateValidator{
		client: k8sClient,
	}
}

// ValidateCreateWorkspace validates workspace against template constraints
func (tv *TemplateValidator) ValidateCreateWorkspace(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	// If no template reference, workspace must specify image
	if workspace.Spec.TemplateRef == nil {
		if workspace.Spec.Image == "" {
			return fmt.Errorf("workspace must specify either templateRef or image")
		}
		return nil
	}

	// Fetch template
	template := &workspacev1alpha1.WorkspaceTemplate{}
	templateKey := types.NamespacedName{Name: *workspace.Spec.TemplateRef}
	if err := tv.client.Get(ctx, templateKey, template); err != nil {
		return fmt.Errorf("failed to get template %s: %w", *workspace.Spec.TemplateRef, err)
	}

	var violations []controller.TemplateViolation

	// Validate image
	if workspace.Spec.Image != "" {
		if violation := tv.validateImageAllowed(workspace.Spec.Image, template.Spec.AllowedImages, template.Spec.DefaultImage); violation != nil {
			violations = append(violations, *violation)
		}
	}

	// Validate resources
	if workspace.Spec.Resources != nil {
		if resourceViolations := tv.validateResourceBounds(*workspace.Spec.Resources, template.Spec.ResourceBounds); len(resourceViolations) > 0 {
			violations = append(violations, resourceViolations...)
		}
	}

	// Only validate storage if it changed
	if workspace.Spec.Storage != nil && !workspace.Spec.Storage.Size.IsZero() {
		if violation := tv.validateStorageSize(workspace.Spec.Storage.Size, template.Spec.PrimaryStorage); violation != nil {
			violations = append(violations, *violation)
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("workspace violates template constraints: %s", formatViolations(violations))
	}

	return nil
}

// ValidateUpdateWorkspace validates only changed fields in workspace against template constraints
func (tv *TemplateValidator) ValidateUpdateWorkspace(ctx context.Context, oldWorkspace, newWorkspace *workspacev1alpha1.Workspace) error {
	// If no template reference, workspace must specify image
	if newWorkspace.Spec.TemplateRef == nil {
		if newWorkspace.Spec.Image == "" {
			return fmt.Errorf("workspace must specify either templateRef or image")
		}
		return nil
	}

	// Fetch template
	template := &workspacev1alpha1.WorkspaceTemplate{}
	templateKey := types.NamespacedName{Name: *newWorkspace.Spec.TemplateRef}
	if err := tv.client.Get(ctx, templateKey, template); err != nil {
		return fmt.Errorf("failed to get template %s: %w", *newWorkspace.Spec.TemplateRef, err)
	}

	var violations []controller.TemplateViolation

	// Only validate image if it changed
	if oldWorkspace.Spec.Image != newWorkspace.Spec.Image && newWorkspace.Spec.Image != "" {
		if violation := tv.validateImageAllowed(newWorkspace.Spec.Image, template.Spec.AllowedImages, template.Spec.DefaultImage); violation != nil {
			violations = append(violations, *violation)
		}
	}

	// Only validate resources if they changed
	if !resourcesEqual(oldWorkspace.Spec.Resources, newWorkspace.Spec.Resources) && newWorkspace.Spec.Resources != nil {
		if resourceViolations := tv.validateResourceBounds(*newWorkspace.Spec.Resources, template.Spec.ResourceBounds); len(resourceViolations) > 0 {
			violations = append(violations, resourceViolations...)
		}
	}

	// Only validate storage if it changed
	if !storageEqual(oldWorkspace.Spec.Storage, newWorkspace.Spec.Storage) &&
		newWorkspace.Spec.Storage != nil && !newWorkspace.Spec.Storage.Size.IsZero() {
		if violation := tv.validateStorageSize(newWorkspace.Spec.Storage.Size, template.Spec.PrimaryStorage); violation != nil {
			violations = append(violations, *violation)
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("workspace violates template constraints: %s", formatViolations(violations))
	}

	return nil
}

// validateImageAllowed checks if image is in allowed list
func (tv *TemplateValidator) validateImageAllowed(image string, allowedImages []string, defaultImage string) *controller.TemplateViolation {
	effectiveAllowedImages := allowedImages
	if len(allowedImages) == 0 {
		effectiveAllowedImages = []string{defaultImage}
	}

	for _, allowed := range effectiveAllowedImages {
		if image == allowed {
			return nil
		}
	}

	return &controller.TemplateViolation{
		Type:    controller.ViolationTypeImageNotAllowed,
		Field:   "spec.image",
		Message: "Image is not in the template's allowed list",
		Allowed: fmt.Sprintf("%v", effectiveAllowedImages),
		Actual:  image,
	}
}

// validateResourceBounds checks if resources are within bounds
func (tv *TemplateValidator) validateResourceBounds(resources corev1.ResourceRequirements, bounds *workspacev1alpha1.ResourceBounds) []controller.TemplateViolation {
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
					Message: "CPU request is below template minimum",
					Allowed: fmt.Sprintf("min: %s", bounds.CPU.Min.String()),
					Actual:  cpuRequest.String(),
				})
			}
			if cpuRequest.Cmp(bounds.CPU.Max) > 0 {
				violations = append(violations, controller.TemplateViolation{
					Type:    controller.ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.cpu",
					Message: "CPU request exceeds template maximum",
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
					Message: "Memory request is below template minimum",
					Allowed: fmt.Sprintf("min: %s", bounds.Memory.Min.String()),
					Actual:  memRequest.String(),
				})
			}
			if memRequest.Cmp(bounds.Memory.Max) > 0 {
				violations = append(violations, controller.TemplateViolation{
					Type:    controller.ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.memory",
					Message: "Memory request exceeds template maximum",
					Allowed: fmt.Sprintf("max: %s", bounds.Memory.Max.String()),
					Actual:  memRequest.String(),
				})
			}
		}
	}

	return violations
}

// validateStorageSize checks if storage size is within bounds
func (tv *TemplateValidator) validateStorageSize(size resource.Quantity, config *workspacev1alpha1.StorageConfig) *controller.TemplateViolation {
	if config == nil {
		return nil
	}

	if config.MinSize != nil && size.Cmp(*config.MinSize) < 0 {
		return &controller.TemplateViolation{
			Type:    controller.ViolationTypeStorageExceeded,
			Field:   "spec.storage.size",
			Message: "Storage size is below template minimum",
			Allowed: fmt.Sprintf("min: %s", config.MinSize.String()),
			Actual:  size.String(),
		}
	}

	if config.MaxSize != nil && size.Cmp(*config.MaxSize) > 0 {
		return &controller.TemplateViolation{
			Type:    controller.ViolationTypeStorageExceeded,
			Field:   "spec.storage.size",
			Message: "Storage size exceeds template maximum",
			Allowed: fmt.Sprintf("max: %s", config.MaxSize.String()),
			Actual:  size.String(),
		}
	}

	return nil
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

// storageEqual compares two StorageSpec for equality
func storageEqual(old, new *workspacev1alpha1.StorageSpec) bool {
	if old == nil && new == nil {
		return true
	}
	if old == nil || new == nil {
		return false
	}

	return old.Size.Equal(new.Size) && old.MountPath == new.MountPath
}

// formatViolations formats template violations into a readable error message
func formatViolations(violations []controller.TemplateViolation) string {
	if len(violations) == 0 {
		return ""
	}
	if len(violations) == 1 {
		return violations[0].Message
	}

	msg := fmt.Sprintf("%d violations: ", len(violations))
	for i, v := range violations {
		if i > 0 {
			msg += "; "
		}
		msg += v.Message
	}
	return msg
}

// ApplyTemplateDefaults applies template defaults to workspace
func (tv *TemplateValidator) ApplyTemplateDefaults(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	if workspace.Spec.TemplateRef == nil {
		return nil
	}

	// Fetch template
	template := &workspacev1alpha1.WorkspaceTemplate{}
	templateKey := types.NamespacedName{Name: *workspace.Spec.TemplateRef}
	if err := tv.client.Get(ctx, templateKey, template); err != nil {
		return fmt.Errorf("failed to get template %s: %w", *workspace.Spec.TemplateRef, err)
	}

	// Apply defaults
	if workspace.Spec.Image == "" && template.Spec.DefaultImage != "" {
		workspace.Spec.Image = template.Spec.DefaultImage
	}

	if workspace.Spec.Resources == nil && template.Spec.DefaultResources != nil {
		workspace.Spec.Resources = template.Spec.DefaultResources.DeepCopy()
	}

	if workspace.Spec.Storage == nil && template.Spec.PrimaryStorage != nil &&
		!template.Spec.PrimaryStorage.DefaultSize.IsZero() {
		workspace.Spec.Storage = &workspacev1alpha1.StorageSpec{
			Size: template.Spec.PrimaryStorage.DefaultSize,
		}
	}

	// Add template tracking label
	if workspace.Labels == nil {
		workspace.Labels = make(map[string]string)
	}
	workspace.Labels["workspace.jupyter.org/template"] = *workspace.Spec.TemplateRef

	return nil
}
