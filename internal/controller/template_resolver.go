/*
MIT License

Copyright (c) 2025 jupyter-ai-contrib

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
*/

// Package controller provides template resolution and validation for WorkspaceTemplate resources.
// This file implements the core logic for:
// - Resolving WorkspaceTemplate references to compute final workspace configuration
// - Merging template defaults with workspace-specific overrides
// - Validating overrides against template-defined resource bounds
// - Enforcing image allowlists and storage constraints
package controller

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// TemplateResolver handles resolving WorkspaceTemplate references and applying overrides
type TemplateResolver struct {
	client client.Client
}

// NewTemplateResolver creates a new TemplateResolver
func NewTemplateResolver(k8sClient client.Client) *TemplateResolver {
	return &TemplateResolver{
		client: k8sClient,
	}
}

// ResolvedTemplate contains the resolved template configuration
type ResolvedTemplate struct {
	Image                  string
	Resources              corev1.ResourceRequirements
	EnvironmentVariables   []corev1.EnvVar
	StorageConfiguration   *workspacesv1alpha1.StorageConfig
	AllowSecondaryStorages bool
	NodeSelector           map[string]string
	Affinity               *corev1.Affinity
	Tolerations            []corev1.Toleration
}

// ValidateAndResolveTemplate resolves a WorkspaceTemplate reference, validates overrides, and returns validation result
func (tr *TemplateResolver) ValidateAndResolveTemplate(ctx context.Context, workspace *workspacesv1alpha1.Workspace) (*TemplateValidationResult, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name, "namespace", workspace.Namespace)

	// If no template reference, workspace must specify image directly
	if workspace.Spec.TemplateRef == nil {
		logger.Info("No template reference, workspace must specify image directly")

		// Require workspace.Spec.Image to be set
		if workspace.Spec.Image == "" {
			return nil, fmt.Errorf("workspace does not reference a template and does not specify an image - either set spec.templateRef or spec.image")
		}

		// Return nil template for non-template workspaces
		return &TemplateValidationResult{
			Valid:      true,
			Violations: []TemplateViolation{},
			Template:   nil, // No template = use workspace spec directly
		}, nil
	}

	// Fetch the WorkspaceTemplate (cluster-scoped resource)
	templateName := *workspace.Spec.TemplateRef
	template := &workspacesv1alpha1.WorkspaceTemplate{}
	templateKey := types.NamespacedName{
		Name: templateName,
	}

	logger.Info("Fetching template for validation", "template", templateName)

	if err := tr.client.Get(ctx, templateKey, template); err != nil {
		// Template not found is a system error (not a user validation error)
		// Return error to trigger controller error handling and Degraded condition
		return nil, fmt.Errorf("failed to get WorkspaceTemplate %s: %w", *workspace.Spec.TemplateRef, err)
	}

	logger.Info("Resolving template", "template", template.Name, "displayName", template.Spec.DisplayName)

	// Start with template defaults
	// Require DefaultImage to be set in template
	defaultImage := template.Spec.DefaultImage
	if defaultImage == "" {
		return nil, fmt.Errorf("template %s does not define a DefaultImage - templates must specify a default container image", template.Name)
	}

	allowSecondaryStorages := true
	if template.Spec.AllowSecondaryStorages != nil {
		allowSecondaryStorages = *template.Spec.AllowSecondaryStorages
	}

	resolved := &ResolvedTemplate{
		Image:                  defaultImage,
		Resources:              corev1.ResourceRequirements{}, // Default empty if not specified
		EnvironmentVariables:   template.Spec.EnvironmentVariables,
		AllowSecondaryStorages: allowSecondaryStorages,
		NodeSelector:           template.Spec.DefaultNodeSelector,
		Affinity:               template.Spec.DefaultAffinity,
		Tolerations:            template.Spec.DefaultTolerations,
	}

	if template.Spec.DefaultResources != nil {
		resolved.Resources = *template.Spec.DefaultResources
	}

	if template.Spec.PrimaryStorage != nil {
		resolved.StorageConfiguration = template.Spec.PrimaryStorage
	}

	// Validate and apply workspace overrides
	violations := tr.validateAndApplyOverrides(ctx, resolved, workspace, template)

	if len(violations) > 0 {
		logger.Info("Template validation failed", "violations", len(violations))
		return &TemplateValidationResult{
			Valid:      false,
			Violations: violations,
			Template:   resolved, // Keep for debugging
		}, nil
	}

	logger.Info("Template resolved and validated successfully", "finalImage", resolved.Image)
	return &TemplateValidationResult{
		Valid:      true,
		Violations: []TemplateViolation{},
		Template:   resolved,
	}, nil
}

// validateAndApplyOverrides validates and applies workspace-specific overrides to the resolved template
// When template is set, base spec fields (Image, Resources, Storage.Size) act as overrides
// Returns a list of violations if validation fails
func (tr *TemplateResolver) validateAndApplyOverrides(ctx context.Context, resolved *ResolvedTemplate, workspace *workspacesv1alpha1.Workspace, template *workspacesv1alpha1.WorkspaceTemplate) []TemplateViolation {
	logger := logf.FromContext(ctx)
	var violations []TemplateViolation

	// Override image if workspace specifies one
	if workspace.Spec.Image != "" {
		// Note: defaultImage is already validated to be non-empty earlier in resolveTemplate
		defaultImage := template.Spec.DefaultImage
		if violation := tr.validateImageAllowed(workspace.Spec.Image, template.Spec.AllowedImages, defaultImage); violation != nil {
			violations = append(violations, *violation)
		} else {
			resolved.Image = workspace.Spec.Image
			logger.Info("Applied image override", "image", workspace.Spec.Image)
		}
	}

	// Override resources if workspace specifies them
	if workspace.Spec.Resources != nil {
		if resourceViolations := tr.validateResourceBounds(*workspace.Spec.Resources, template.Spec.ResourceBounds); len(resourceViolations) > 0 {
			violations = append(violations, resourceViolations...)
		} else {
			resolved.Resources = *workspace.Spec.Resources
			logger.Info("Applied resource overrides")
		}
	}

	// Override storage size if workspace specifies one
	if workspace.Spec.Storage != nil && !workspace.Spec.Storage.Size.IsZero() && resolved.StorageConfiguration != nil {
		storageQuantity := workspace.Spec.Storage.Size
		if violation := tr.validateStorageSize(storageQuantity, template.Spec.PrimaryStorage); violation != nil {
			violations = append(violations, *violation)
		} else {
			resolved.StorageConfiguration.DefaultSize = storageQuantity
			logger.Info("Applied storage size override", "storageSize", storageQuantity.String())
		}
	}

	return violations
}

// validateImageAllowed checks if the image is in the template's allowed list
// When allowedImages is empty, only the defaultImage is allowed (secure by default)
// Returns a violation if the image is not allowed, nil otherwise
func (tr *TemplateResolver) validateImageAllowed(image string, allowedImages []string, defaultImage string) *TemplateViolation {
	effectiveAllowedImages := allowedImages
	if len(allowedImages) == 0 {
		effectiveAllowedImages = []string{defaultImage}
	}

	for _, allowedImage := range effectiveAllowedImages {
		if image == allowedImage {
			return nil
		}
	}

	// Image not in allowed list
	allowedStr := fmt.Sprintf("%v", effectiveAllowedImages)

	return &TemplateViolation{
		Type:    ViolationTypeImageNotAllowed,
		Field:   "spec.image",
		Message: "Image is not in the template's allowed list",
		Allowed: allowedStr,
		Actual:  image,
	}
}

// validateResourceBounds checks if resources are within template bounds
// Returns a list of violations for any resources that exceed bounds
func (tr *TemplateResolver) validateResourceBounds(resources corev1.ResourceRequirements, bounds *workspacesv1alpha1.ResourceBounds) []TemplateViolation {
	var violations []TemplateViolation

	// First validate that limits >= requests for all resources
	if resources.Requests != nil && resources.Limits != nil {
		if cpuRequest, hasRequest := resources.Requests[corev1.ResourceCPU]; hasRequest {
			if cpuLimit, hasLimit := resources.Limits[corev1.ResourceCPU]; hasLimit {
				if cpuLimit.Cmp(cpuRequest) < 0 {
					violations = append(violations, TemplateViolation{
						Type:    ViolationTypeResourceExceeded,
						Field:   "spec.resources.limits.cpu",
						Message: "CPU limit must be greater than or equal to CPU request",
						Allowed: fmt.Sprintf("limit >= %s", cpuRequest.String()),
						Actual:  cpuLimit.String(),
					})
				}
			}
		}

		if memoryRequest, hasRequest := resources.Requests[corev1.ResourceMemory]; hasRequest {
			if memoryLimit, hasLimit := resources.Limits[corev1.ResourceMemory]; hasLimit {
				if memoryLimit.Cmp(memoryRequest) < 0 {
					violations = append(violations, TemplateViolation{
						Type:    ViolationTypeResourceExceeded,
						Field:   "spec.resources.limits.memory",
						Message: "Memory limit must be greater than or equal to memory request",
						Allowed: fmt.Sprintf("limit >= %s", memoryRequest.String()),
						Actual:  memoryLimit.String(),
					})
				}
			}
		}
	}

	if bounds == nil {
		// No bounds specified, any resources are allowed
		return violations
	}

	// Validate CPU bounds
	if bounds.CPU != nil && resources.Requests != nil {
		if cpuRequest, exists := resources.Requests[corev1.ResourceCPU]; exists {
			if cpuRequest.Cmp(bounds.CPU.Min) < 0 {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.cpu",
					Message: "CPU request is below template minimum",
					Allowed: fmt.Sprintf("min: %s", bounds.CPU.Min.String()),
					Actual:  cpuRequest.String(),
				})
			}
			if cpuRequest.Cmp(bounds.CPU.Max) > 0 {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.cpu",
					Message: "CPU request exceeds template maximum",
					Allowed: fmt.Sprintf("max: %s", bounds.CPU.Max.String()),
					Actual:  cpuRequest.String(),
				})
			}
		}
	}

	// Validate Memory bounds
	if bounds.Memory != nil && resources.Requests != nil {
		if memoryRequest, exists := resources.Requests[corev1.ResourceMemory]; exists {
			if memoryRequest.Cmp(bounds.Memory.Min) < 0 {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.memory",
					Message: "Memory request is below template minimum",
					Allowed: fmt.Sprintf("min: %s", bounds.Memory.Min.String()),
					Actual:  memoryRequest.String(),
				})
			}
			if memoryRequest.Cmp(bounds.Memory.Max) > 0 {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests.memory",
					Message: "Memory request exceeds template maximum",
					Allowed: fmt.Sprintf("max: %s", bounds.Memory.Max.String()),
					Actual:  memoryRequest.String(),
				})
			}
		}
	}

	// Validate GPU bounds if specified
	if bounds.GPU != nil && resources.Requests != nil {
		gpuResourceName := corev1.ResourceName("nvidia.com/gpu")
		if gpuRequest, exists := resources.Requests[gpuResourceName]; exists {
			if gpuRequest.Cmp(bounds.GPU.Min) < 0 {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests['nvidia.com/gpu']",
					Message: "GPU request is below template minimum",
					Allowed: fmt.Sprintf("min: %s", bounds.GPU.Min.String()),
					Actual:  gpuRequest.String(),
				})
			}
			if gpuRequest.Cmp(bounds.GPU.Max) > 0 {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeResourceExceeded,
					Field:   "spec.resources.requests['nvidia.com/gpu']",
					Message: "GPU request exceeds template maximum",
					Allowed: fmt.Sprintf("max: %s", bounds.GPU.Max.String()),
					Actual:  gpuRequest.String(),
				})
			}
		}
	}

	return violations
}

// validateStorageSize checks if storage size is within template bounds
// Returns a violation if the size is outside bounds, nil otherwise
func (tr *TemplateResolver) validateStorageSize(size resource.Quantity, storageConfig *workspacesv1alpha1.StorageConfig) *TemplateViolation {
	if storageConfig == nil {
		return nil
	}

	if storageConfig.MinSize != nil && size.Cmp(*storageConfig.MinSize) < 0 {
		return &TemplateViolation{
			Type:    ViolationTypeStorageExceeded,
			Field:   "spec.storage.size",
			Message: "Storage size is below template minimum",
			Allowed: fmt.Sprintf("min: %s", storageConfig.MinSize.String()),
			Actual:  size.String(),
		}
	}

	if storageConfig.MaxSize != nil && size.Cmp(*storageConfig.MaxSize) > 0 {
		return &TemplateViolation{
			Type:    ViolationTypeStorageExceeded,
			Field:   "spec.storage.size",
			Message: "Storage size exceeds template maximum",
			Allowed: fmt.Sprintf("max: %s", storageConfig.MaxSize.String()),
			Actual:  size.String(),
		}
	}

	return nil
}

// ListWorkspacesUsingTemplate returns all workspaces that reference the specified template
// Excludes workspaces that are being deleted (have deletionTimestamp set) to follow
// Kubernetes best practices for finalizer management and garbage collection
func (tr *TemplateResolver) ListWorkspacesUsingTemplate(ctx context.Context, templateName string) ([]workspacesv1alpha1.Workspace, error) {
	logger := logf.FromContext(ctx)

	// Use label selector for fast, efficient lookup
	workspaceList := &workspacesv1alpha1.WorkspaceList{}
	if err := tr.client.List(ctx, workspaceList, client.MatchingLabels{
		"workspaces.jupyter.org/template": templateName,
	}); err != nil {
		return nil, fmt.Errorf("failed to list workspaces by template label: %w", err)
	}

	// Filter out workspaces being deleted and verify template reference
	// This follows Kubernetes controller best practice: resources with deletionTimestamp
	// are considered "gone" for dependency checking purposes
	activeWorkspaces := []workspacesv1alpha1.Workspace{}
	for _, ws := range workspaceList.Items {
		if !ws.DeletionTimestamp.IsZero() {
			continue // Skip workspaces being deleted
		}

		// Verify templateRef matches label to guard against label/spec mismatch.
		// This is somewhat redundant given CEL immutability validation but adds zero cost and adds a layer of verification.
		if ws.Spec.TemplateRef == nil {
			logger.V(1).Info("Workspace has template label but nil templateRef",
				"workspace", fmt.Sprintf("%s/%s", ws.Namespace, ws.Name),
				"label", templateName)
			continue
		}

		if *ws.Spec.TemplateRef != templateName {
			// This should never happen due to CEL immutability - log if it occurs
			logger.Info("Workspace has template label but different templateRef",
				"workspace", fmt.Sprintf("%s/%s", ws.Namespace, ws.Name),
				"label", templateName,
				"spec", *ws.Spec.TemplateRef)
			continue
		}

		activeWorkspaces = append(activeWorkspaces, ws)
	}

	return activeWorkspaces, nil
}
