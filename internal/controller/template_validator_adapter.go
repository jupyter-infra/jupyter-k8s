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

package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// TemplateValidatorAdapter implements TemplateValidatorInterface in the controller package
// This provides template validation logic for compliance checking without circular dependencies
type TemplateValidatorAdapter struct {
	client client.Client
}

// NewTemplateValidatorAdapter creates a new template validator adapter
func NewTemplateValidatorAdapter(k8sClient client.Client) *TemplateValidatorAdapter {
	return &TemplateValidatorAdapter{
		client: k8sClient,
	}
}

// ValidateCreateWorkspace validates workspace against template constraints
// This implements simplified validation logic for compliance checking
// Full validation is performed by webhooks; this is for detecting violations after template changes
func (tv *TemplateValidatorAdapter) ValidateCreateWorkspace(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	if workspace.Spec.TemplateRef == nil {
		return nil
	}

	template, err := tv.fetchTemplate(ctx, workspace.Spec.TemplateRef.Name)
	if err != nil {
		return err
	}

	// Simplified validation focusing on key constraints that can change in templates
	var violations []TemplateViolation

	// Validate image if workspace specifies one
	if workspace.Spec.Image != "" {
		if violation := tv.validateImage(workspace.Spec.Image, template); violation != nil {
			violations = append(violations, *violation)
		}
	}

	// Validate resource bounds if workspace specifies resources
	if workspace.Spec.Resources != nil {
		if resourceViolations := tv.validateResources(workspace.Spec.Resources, template); len(resourceViolations) > 0 {
			violations = append(violations, resourceViolations...)
		}
	}

	// Validate storage size if workspace specifies storage
	if workspace.Spec.Storage != nil && !workspace.Spec.Storage.Size.IsZero() {
		if violation := tv.validateStorage(workspace.Spec.Storage.Size, template); violation != nil {
			violations = append(violations, *violation)
		}
	}

	if len(violations) > 0 {
		// Return first violation as error message
		return fmt.Errorf("workspace violates template '%s' constraints: %s", template.Name, violations[0].Message)
	}

	return nil
}

// fetchTemplate retrieves a template by name
func (tv *TemplateValidatorAdapter) fetchTemplate(ctx context.Context, templateName string) (*workspacev1alpha1.WorkspaceTemplate, error) {
	template := &workspacev1alpha1.WorkspaceTemplate{}
	if err := tv.client.Get(ctx, types.NamespacedName{Name: templateName}, template); err != nil {
		return nil, fmt.Errorf("failed to get template %s: %w", templateName, err)
	}
	return template, nil
}

// validateImage checks if image is in template's allowed list
func (tv *TemplateValidatorAdapter) validateImage(image string, template *workspacev1alpha1.WorkspaceTemplate) *TemplateViolation {
	// Skip validation if custom images are allowed
	if template.Spec.AllowCustomImages != nil && *template.Spec.AllowCustomImages {
		return nil
	}

	effectiveAllowedImages := template.Spec.AllowedImages
	if len(template.Spec.AllowedImages) == 0 {
		effectiveAllowedImages = []string{template.Spec.DefaultImage}
	}

	for _, allowed := range effectiveAllowedImages {
		if image == allowed {
			return nil
		}
	}

	return &TemplateViolation{
		Type:    ViolationTypeImageNotAllowed,
		Field:   "spec.image",
		Message: fmt.Sprintf("Image '%s' is not allowed by template '%s'", image, template.Name),
		Allowed: fmt.Sprintf("%v", effectiveAllowedImages),
		Actual:  image,
	}
}

// validateResources checks if resources are within template bounds
func (tv *TemplateValidatorAdapter) validateResources(resources *corev1.ResourceRequirements, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	var violations []TemplateViolation

	bounds := template.Spec.ResourceBounds
	if bounds == nil || resources.Requests == nil {
		return violations
	}

	// Validate CPU bounds
	if bounds.CPU != nil {
		if cpuRequest, exists := resources.Requests[corev1.ResourceCPU]; exists {
			if cpuRequest.Cmp(bounds.CPU.Min) < 0 {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.cpu",
					Message: fmt.Sprintf("CPU request %s is below minimum %s required by template '%s'", cpuRequest.String(), bounds.CPU.Min.String(), template.Name),
					Allowed: fmt.Sprintf("min: %s", bounds.CPU.Min.String()),
					Actual:  cpuRequest.String(),
				})
			}
			if cpuRequest.Cmp(bounds.CPU.Max) > 0 {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.cpu",
					Message: fmt.Sprintf("CPU request %s exceeds maximum %s allowed by template '%s'", cpuRequest.String(), bounds.CPU.Max.String(), template.Name),
					Allowed: fmt.Sprintf("max: %s", bounds.CPU.Max.String()),
					Actual:  cpuRequest.String(),
				})
			}
		}
	}

	// Validate memory bounds
	if bounds.Memory != nil {
		if memRequest, exists := resources.Requests[corev1.ResourceMemory]; exists {
			if memRequest.Cmp(bounds.Memory.Min) < 0 {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.memory",
					Message: fmt.Sprintf("Memory request %s is below minimum %s required by template '%s'", memRequest.String(), bounds.Memory.Min.String(), template.Name),
					Allowed: fmt.Sprintf("min: %s", bounds.Memory.Min.String()),
					Actual:  memRequest.String(),
				})
			}
			if memRequest.Cmp(bounds.Memory.Max) > 0 {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.memory",
					Message: fmt.Sprintf("Memory request %s exceeds maximum %s allowed by template '%s'", memRequest.String(), bounds.Memory.Max.String(), template.Name),
					Allowed: fmt.Sprintf("max: %s", bounds.Memory.Max.String()),
					Actual:  memRequest.String(),
				})
			}
		}
	}

	return violations
}

// validateStorage checks if storage size is within template bounds
func (tv *TemplateValidatorAdapter) validateStorage(size resource.Quantity, template *workspacev1alpha1.WorkspaceTemplate) *TemplateViolation {
	config := template.Spec.PrimaryStorage
	if config == nil {
		return nil
	}

	if config.MinSize != nil && size.Cmp(*config.MinSize) < 0 {
		return &TemplateViolation{
			Type:    ViolationTypeStorageExceeded,
			Field:   "spec.storage.size",
			Message: fmt.Sprintf("Storage size %s is below minimum %s required by template '%s'", size.String(), config.MinSize.String(), template.Name),
			Allowed: fmt.Sprintf("min: %s", config.MinSize.String()),
			Actual:  size.String(),
		}
	}

	if config.MaxSize != nil && size.Cmp(*config.MaxSize) > 0 {
		return &TemplateViolation{
			Type:    ViolationTypeStorageExceeded,
			Field:   "spec.storage.size",
			Message: fmt.Sprintf("Storage size %s exceeds maximum %s allowed by template '%s'", size.String(), config.MaxSize.String(), template.Name),
			Allowed: fmt.Sprintf("max: %s", config.MaxSize.String()),
			Actual:  size.String(),
		}
	}

	return nil
}
