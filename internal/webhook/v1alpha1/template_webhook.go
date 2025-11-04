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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
)

// nolint:unused
// log is for logging in this package.
var templatelog = logf.Log.WithName("workspacetemplate-resource")

// SetupWorkspaceTemplateWebhookWithManager registers the webhook for WorkspaceTemplate in the manager.
// RBAC Note: This webhook requires Workspace access (list, update) to mark workspaces for compliance checks.
// Workspace RBAC is provided by the workspace controller RBAC markers.
func SetupWorkspaceTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&workspacev1alpha1.WorkspaceTemplate{}).
		WithValidator(&WorkspaceTemplateCustomValidator{client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-workspace-jupyter-org-v1alpha1-workspacetemplate,mutating=false,failurePolicy=fail,sideEffects=None,groups=workspace.jupyter.org,resources=workspacetemplates,verbs=update,versions=v1alpha1,name=vworkspacetemplate-v1alpha1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

// WorkspaceTemplateCustomValidator struct is responsible for validating the WorkspaceTemplate resource
// when it is updated. It checks if constraint fields changed and marks affected workspaces for compliance checking.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type WorkspaceTemplateCustomValidator struct {
	client client.Client
}

var _ webhook.CustomValidator = &WorkspaceTemplateCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type WorkspaceTemplate.
func (v *WorkspaceTemplateCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	template, ok := obj.(*workspacev1alpha1.WorkspaceTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a WorkspaceTemplate object but got %T", obj)
	}
	templatelog.Info("Validation for WorkspaceTemplate upon creation", "name", template.GetName())

	// No special validation needed on create
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type WorkspaceTemplate.
func (v *WorkspaceTemplateCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldTemplate, ok := oldObj.(*workspacev1alpha1.WorkspaceTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a WorkspaceTemplate object for the oldObj but got %T", oldObj)
	}
	newTemplate, ok := newObj.(*workspacev1alpha1.WorkspaceTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a WorkspaceTemplate object for the newObj but got %T", newObj)
	}
	templatelog.Info("Validation for WorkspaceTemplate upon update", "name", newTemplate.GetName())

	// Check if constraint fields changed
	if constraintsChanged(oldTemplate, newTemplate) {
		templatelog.Info("Template constraints changed, marking workspaces for compliance check", "template", newTemplate.GetName())

		// Mark all workspaces using this template for compliance checking
		if err := v.markWorkspacesForComplianceCheck(ctx, newTemplate.GetName()); err != nil {
			templatelog.Error(err, "Failed to mark workspaces for compliance check", "template", newTemplate.GetName())
			// Don't fail the webhook - allow the template update to proceed
			// The controller will eventually reconcile and catch compliance issues
			return admission.Warnings{"Failed to mark some workspaces for compliance check - they will be checked during next reconciliation"}, nil
		}
	}

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type WorkspaceTemplate.
func (v *WorkspaceTemplateCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	template, ok := obj.(*workspacev1alpha1.WorkspaceTemplate)
	if !ok {
		return nil, fmt.Errorf("expected a WorkspaceTemplate object but got %T", obj)
	}
	templatelog.Info("Validation for WorkspaceTemplate upon deletion", "name", template.GetName())

	// Deletion is handled by finalizers in the controller
	return nil, nil
}

// constraintsChanged checks if any constraint fields changed between old and new templates
// Constraint fields are those that affect workspace validation (resource bounds, allowed images, etc.)
func constraintsChanged(oldTemplate, newTemplate *workspacev1alpha1.WorkspaceTemplate) bool {
	oldSpec := &oldTemplate.Spec
	newSpec := &newTemplate.Spec

	// Check AllowedImages changes
	if !stringSlicesEqual(oldSpec.AllowedImages, newSpec.AllowedImages) {
		return true
	}

	// Check ResourceBounds changes
	if resourceBoundsChanged(oldSpec.ResourceBounds, newSpec.ResourceBounds) {
		return true
	}

	// Check PrimaryStorage.MaxSize changes
	if maxStorageSizeChanged(oldSpec.PrimaryStorage, newSpec.PrimaryStorage) {
		return true
	}

	// Check IdleShutdownOverrides.Allow changes
	if idleShutdownAllowOverrideChanged(oldSpec.IdleShutdownOverrides, newSpec.IdleShutdownOverrides) {
		return true
	}

	// Check IdleShutdownOverrides timeout bounds changes
	if idleShutdownTimeoutBoundsChanged(oldSpec.IdleShutdownOverrides, newSpec.IdleShutdownOverrides) {
		return true
	}

	return false
}

// stringSlicesEqual checks if two string slices have the same elements (order matters)
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// resourceBoundsChanged checks if resource bounds changed
func resourceBoundsChanged(oldBounds, newBounds *workspacev1alpha1.ResourceBounds) bool {
	// If one is nil and the other isn't, they're different
	if (oldBounds == nil) != (newBounds == nil) {
		return true
	}

	// Both nil means no change
	if oldBounds == nil {
		return false
	}

	// Check CPU bounds
	if resourceRangeChanged(oldBounds.CPU, newBounds.CPU) {
		return true
	}

	// Check Memory bounds
	if resourceRangeChanged(oldBounds.Memory, newBounds.Memory) {
		return true
	}

	// Check GPU bounds
	if resourceRangeChanged(oldBounds.GPU, newBounds.GPU) {
		return true
	}

	return false
}

// resourceRangeChanged checks if a resource range changed
func resourceRangeChanged(oldRange, newRange *workspacev1alpha1.ResourceRange) bool {
	// If one is nil and the other isn't, they're different
	if (oldRange == nil) != (newRange == nil) {
		return true
	}

	// Both nil means no change
	if oldRange == nil {
		return false
	}

	// Compare Min and Max
	return !oldRange.Min.Equal(newRange.Min) || !oldRange.Max.Equal(newRange.Max)
}

// maxStorageSizeChanged checks if max storage size changed
func maxStorageSizeChanged(oldStorage, newStorage *workspacev1alpha1.StorageConfig) bool {
	// If one is nil and the other isn't, they're different
	if (oldStorage == nil) != (newStorage == nil) {
		return true
	}

	// Both nil means no change
	if oldStorage == nil {
		return false
	}

	// Check MaxSize changes
	if (oldStorage.MaxSize == nil) != (newStorage.MaxSize == nil) {
		return true
	}

	if oldStorage.MaxSize != nil && !oldStorage.MaxSize.Equal(*newStorage.MaxSize) {
		return true
	}

	// Check MinSize changes (also affects validation)
	if (oldStorage.MinSize == nil) != (newStorage.MinSize == nil) {
		return true
	}

	if oldStorage.MinSize != nil && !oldStorage.MinSize.Equal(*newStorage.MinSize) {
		return true
	}

	return false
}

// idleShutdownAllowOverrideChanged checks if Allow setting changed
func idleShutdownAllowOverrideChanged(oldOverrides, newOverrides *workspacev1alpha1.IdleShutdownOverridePolicy) bool {
	// If one is nil and the other isn't, they're different
	if (oldOverrides == nil) != (newOverrides == nil) {
		return true
	}

	// Both nil means no change
	if oldOverrides == nil {
		return false
	}

	// Check Allow changes
	if (oldOverrides.Allow == nil) != (newOverrides.Allow == nil) {
		return true
	}

	if oldOverrides.Allow != nil && *oldOverrides.Allow != *newOverrides.Allow {
		return true
	}

	return false
}

// idleShutdownTimeoutBoundsChanged checks if timeout bounds changed
func idleShutdownTimeoutBoundsChanged(oldOverrides, newOverrides *workspacev1alpha1.IdleShutdownOverridePolicy) bool {
	// If one is nil and the other isn't, they're different
	if (oldOverrides == nil) != (newOverrides == nil) {
		return true
	}

	// Both nil means no change
	if oldOverrides == nil {
		return false
	}

	// Check MinTimeoutMinutes
	if (oldOverrides.MinTimeoutMinutes == nil) != (newOverrides.MinTimeoutMinutes == nil) {
		return true
	}
	if oldOverrides.MinTimeoutMinutes != nil && *oldOverrides.MinTimeoutMinutes != *newOverrides.MinTimeoutMinutes {
		return true
	}

	// Check MaxTimeoutMinutes
	if (oldOverrides.MaxTimeoutMinutes == nil) != (newOverrides.MaxTimeoutMinutes == nil) {
		return true
	}
	if oldOverrides.MaxTimeoutMinutes != nil && *oldOverrides.MaxTimeoutMinutes != *newOverrides.MaxTimeoutMinutes {
		return true
	}

	return false
}

// markWorkspacesForComplianceCheck marks all workspaces using the template for compliance checking
// It adds a label to trigger reconciliation and compliance validation
func (v *WorkspaceTemplateCustomValidator) markWorkspacesForComplianceCheck(ctx context.Context, templateName string) error {
	const pageSize = 100 // Process workspaces in batches to avoid overwhelming the API server

	continueToken := ""
	totalMarked := 0

	for {
		workspaces, nextToken, err := workspace.ListByTemplate(ctx, v.client, templateName, continueToken, pageSize)
		if err != nil {
			return fmt.Errorf("failed to list workspaces for template %s: %w", templateName, err)
		}

		// Mark each workspace in this batch
		for i := range workspaces {
			ws := &workspaces[i]

			// Add compliance check label
			if ws.Labels == nil {
				ws.Labels = make(map[string]string)
			}
			ws.Labels[controller.LabelComplianceCheckNeeded] = "true"

			// Update the workspace
			if err := v.client.Update(ctx, ws); err != nil {
				templatelog.Error(err, "Failed to mark workspace for compliance check",
					"workspace", ws.Name,
					"namespace", ws.Namespace,
					"template", templateName)
				// Continue marking other workspaces even if one fails
				continue
			}

			totalMarked++
			templatelog.V(1).Info("Marked workspace for compliance check",
				"workspace", ws.Name,
				"namespace", ws.Namespace,
				"template", templateName)
		}

		// Check if there are more workspaces to process
		if nextToken == "" {
			break
		}
		continueToken = nextToken
	}

	templatelog.Info("Marked workspaces for compliance check",
		"template", templateName,
		"count", totalMarked)

	return nil
}
