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

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

// TemplateValidator handles template validation for webhooks
type TemplateValidator struct {
	client   client.Client
	resolver *controller.TemplateResolver
}

// NewTemplateValidator creates a new TemplateValidator
func NewTemplateValidator(k8sClient client.Client) *TemplateValidator {
	return &TemplateValidator{
		client:   k8sClient,
		resolver: controller.NewTemplateResolver(k8sClient),
	}
}

// ValidateWorkspace validates workspace against template constraints
func (tv *TemplateValidator) ValidateWorkspace(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	// If no template reference, workspace must specify image
	if workspace.Spec.TemplateRef == nil {
		if workspace.Spec.Image == "" {
			return fmt.Errorf("workspace must specify either templateRef or image")
		}
		return nil
	}

	// Validate template exists and workspace conforms to it
	result, err := tv.resolver.ValidateAndResolveTemplate(ctx, workspace)
	if err != nil {
		return fmt.Errorf("template validation failed: %w", err)
	}

	if !result.Valid {
		return fmt.Errorf("workspace violates template constraints: %s",
			formatViolations(result.Violations))
	}

	return nil
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
