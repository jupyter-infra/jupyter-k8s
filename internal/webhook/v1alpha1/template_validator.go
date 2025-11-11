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

	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
)

// TemplateValidator handles template validation for webhooks
type TemplateValidator struct {
	client                   client.Client
	defaultTemplateNamespace string
}

// NewTemplateValidator creates a new TemplateValidator
func NewTemplateValidator(k8sClient client.Client, defaultTemplateNamespace string) *TemplateValidator {
	return &TemplateValidator{
		client:                   k8sClient,
		defaultTemplateNamespace: defaultTemplateNamespace,
	}
}

// fetchTemplate retrieves a template by name using namespace resolution
func (tv *TemplateValidator) fetchTemplate(ctx context.Context, templateRef *workspacev1alpha1.TemplateRef, workspaceNamespace string) (*workspacev1alpha1.WorkspaceTemplate, error) {
	// Determine template namespace using fallback logic
	templateNamespace := templateRef.Namespace
	if templateNamespace == "" {
		templateNamespace = workspaceNamespace
	}

	// Try to get template from determined namespace
	template := &workspacev1alpha1.WorkspaceTemplate{}
	templateKey := client.ObjectKey{Name: templateRef.Name, Namespace: templateNamespace}
	err := tv.client.Get(ctx, templateKey, template)

	// If not found and we have a default namespace, try there
	if err != nil && tv.defaultTemplateNamespace != "" && templateNamespace != tv.defaultTemplateNamespace {
		templateKey = client.ObjectKey{Name: templateRef.Name, Namespace: tv.defaultTemplateNamespace}
		if fallbackErr := tv.client.Get(ctx, templateKey, template); fallbackErr == nil {
			return template, nil
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get template %s: %w", templateRef.Name, err)
	}
	return template, nil
}

// ValidateCreateWorkspace validates workspace against template constraints
func (tv *TemplateValidator) ValidateCreateWorkspace(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	if workspace.Spec.TemplateRef == nil {
		return nil
	}

	template, err := tv.fetchTemplate(ctx, workspace.Spec.TemplateRef, workspace.Namespace)
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
// Handles templateRef lifecycle: added, deleted, changed, unchanged
func (tv *TemplateValidator) ValidateUpdateWorkspace(ctx context.Context, oldWorkspace, newWorkspace *workspacev1alpha1.Workspace) error {
	oldTemplateRef := oldWorkspace.Spec.TemplateRef
	newTemplateRef := newWorkspace.Spec.TemplateRef

	// Detect templateRef transitions
	templateRefDeleted := oldTemplateRef != nil && newTemplateRef == nil
	templateRefAdded := oldTemplateRef == nil && newTemplateRef != nil
	templateRefChanged := oldTemplateRef != nil && newTemplateRef != nil && oldTemplateRef.Name != newTemplateRef.Name

	// Case 1: TemplateRef deleted (template → standalone)
	// Removing constraints is always safe - no validation needed
	if templateRefDeleted {
		workspacelog.Info("TemplateRef deleted, allowing transition to standalone workspace", "workspace", newWorkspace.Name)
		return nil
	}

	// Case 2: TemplateRef changed (template A → template B)
	// Must validate entire spec against NEW template
	if templateRefChanged {
		workspacelog.Info("TemplateRef changed, validating against new template",
			"workspace", newWorkspace.Name,
			"oldTemplate", oldTemplateRef.Name,
			"newTemplate", newTemplateRef.Name)
		return tv.ValidateCreateWorkspace(ctx, newWorkspace)
	}

	// Case 3: TemplateRef added (standalone → template)
	// Validate entire spec against template
	if templateRefAdded {
		workspacelog.Info("TemplateRef added, validating against template", "workspace", newWorkspace.Name)
		return tv.ValidateCreateWorkspace(ctx, newWorkspace)
	}

	// Case 4: No templateRef in both old and new
	// No template constraints to validate
	if newTemplateRef == nil {
		return nil
	}

	// Case 5: TemplateRef unchanged - check other conditions

	// Check if any spec field changed
	if !specChanged(&oldWorkspace.Spec, &newWorkspace.Spec) {
		// No spec changes - skip validation (metadata-only update)
		return nil
	}

	// Special case: If ONLY DesiredStatus changed to Stopped, allow without validation
	// This enables users to stop workspaces without validation (emergency shutdown, cost savings)
	// However, if other spec fields also changed, those changes must be validated
	if newWorkspace.Spec.DesiredStatus == controller.PhaseStopped &&
		oldWorkspace.Spec.DesiredStatus != controller.PhaseStopped &&
		onlyDesiredStatusChanged(&oldWorkspace.Spec, &newWorkspace.Spec) {
		workspacelog.Info("Allowing workspace stop without template validation (status-only change)", "workspace", newWorkspace.Name)
		return nil
	}

	// Spec changed with same template - validate ENTIRE spec against template
	// This follows Kubernetes best practices: admission webhooks validate desired state, not deltas
	// This includes cases where stopping + other changes occur simultaneously
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
