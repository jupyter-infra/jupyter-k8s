/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1alpha1

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// TemplateValidator handles template validation for webhooks
type TemplateValidator struct {
	resolver *workspaceutil.TemplateResolver
}

// NewTemplateValidator creates a new TemplateValidator
func NewTemplateValidator(k8sClient client.Client, defaultTemplateNamespace string) *TemplateValidator {
	return &TemplateValidator{
		resolver: workspaceutil.NewTemplateResolver(k8sClient, defaultTemplateNamespace),
	}
}

// fetchTemplate retrieves a template using centralized resolver
func (tv *TemplateValidator) fetchTemplate(ctx context.Context, templateRef *workspacev1alpha1.TemplateRef, workspaceNamespace string) (*workspacev1alpha1.WorkspaceTemplate, error) {
	return tv.resolver.ResolveTemplate(ctx, templateRef, workspaceNamespace)
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

	var violations []TemplateViolation

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

	// Validate secondary storage volumes
	if violation := validateSecondaryStorages(workspace.Spec.Volumes, template); violation != nil {
		violations = append(violations, *violation)
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
	if newWorkspace.Spec.DesiredStatus == controller.DesiredStateStopped &&
		oldWorkspace.Spec.DesiredStatus != controller.DesiredStateStopped &&
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
func formatViolations(violations []TemplateViolation) string {
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
