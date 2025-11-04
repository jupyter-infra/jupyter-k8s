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
	client client.Client
}

// NewTemplateValidator creates a new TemplateValidator
func NewTemplateValidator(k8sClient client.Client) *TemplateValidator {
	return &TemplateValidator{
		client: k8sClient,
	}
}

// fetchTemplate retrieves a template by name
func (tv *TemplateValidator) fetchTemplate(ctx context.Context, templateName string) (*workspacev1alpha1.WorkspaceTemplate, error) {
	template := &workspacev1alpha1.WorkspaceTemplate{}
	if err := tv.client.Get(ctx, types.NamespacedName{Name: templateName}, template); err != nil {
		return nil, fmt.Errorf("failed to get template %s: %w", templateName, err)
	}
	return template, nil
}

// ValidateCreateWorkspace validates workspace against template constraints
func (tv *TemplateValidator) ValidateCreateWorkspace(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	if workspace.Spec.TemplateRef == nil {
		return nil
	}

	template, err := tv.fetchTemplate(ctx, workspace.Spec.TemplateRef.Name)
	if err != nil {
		return err
	}

	var violations []controller.TemplateViolation

	// Validate image
	if workspace.Spec.Image != "" {
		if violation := validateImageAllowed(workspace.Spec.Image, template); violation != nil {
			violations = append(violations, *violation)
		}
	}

	// Validate resources
	if workspace.Spec.Resources != nil {
		if resourceViolations := validateResourceBounds(*workspace.Spec.Resources, template); len(resourceViolations) > 0 {
			violations = append(violations, resourceViolations...)
		}
	}

	// Only validate storage if it changed
	if workspace.Spec.Storage != nil && !workspace.Spec.Storage.Size.IsZero() {
		if violation := validateStorageSize(workspace.Spec.Storage.Size, template); violation != nil {
			violations = append(violations, *violation)
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("workspace violates template '%s' constraints: %s", workspace.Spec.TemplateRef.Name, formatViolations(violations))
	}

	return nil
}

// ValidateUpdateWorkspace validates entire spec when any spec field changes (Kubernetes best practice)
// Special case: Stopping a workspace (DesiredStatus=Stopped) always bypasses validation
func (tv *TemplateValidator) ValidateUpdateWorkspace(ctx context.Context, oldWorkspace, newWorkspace *workspacev1alpha1.Workspace) error {
	if newWorkspace.Spec.TemplateRef == nil {
		return nil
	}

	// Special case: Always allow stopping workspace without validation
	// This ensures users can always stop non-compliant workspaces
	if newWorkspace.Spec.DesiredStatus == "Stopped" && oldWorkspace.Spec.DesiredStatus != "Stopped" {
		workspacelog.Info("Allowing workspace stop without template validation", "workspace", newWorkspace.Name)
		return nil
	}

	// Check if any spec field changed
	if !specChanged(&oldWorkspace.Spec, &newWorkspace.Spec) {
		// No spec changes - skip validation (metadata-only update)
		return nil
	}

	// Spec changed - validate ENTIRE spec against template (not just changed fields)
	// This follows Kubernetes best practices: admission webhooks validate desired state, not deltas
	workspacelog.Info("Spec changed, validating entire workspace against template", "workspace", newWorkspace.Name)
	return tv.ValidateCreateWorkspace(ctx, newWorkspace)
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
