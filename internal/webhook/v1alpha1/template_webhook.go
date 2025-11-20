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

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// log is for logging in this package.
var templatelog = logf.Log.WithName("workspacetemplate-resource")

// SetupWorkspaceTemplateWebhookWithManager registers the webhook for WorkspaceTemplate in the manager.
func SetupWorkspaceTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&workspacev1alpha1.WorkspaceTemplate{}).
		WithValidator(&WorkspaceTemplateCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-workspace-jupyter-org-v1alpha1-workspacetemplate,mutating=false,failurePolicy=ignore,sideEffects=None,groups=workspace.jupyter.org,resources=workspacetemplates,verbs=update,versions=v1alpha1,name=vworkspacetemplate-v1alpha1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

// WorkspaceTemplateCustomValidator struct is responsible for validating the WorkspaceTemplate resource
// when it is updated. It checks if constraint fields changed and returns warnings.
// The WorkspaceTemplate controller is responsible for marking affected workspaces for compliance checking.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type WorkspaceTemplateCustomValidator struct {
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
		templatelog.Info("Template constraints changed, controller will mark workspaces for compliance check", "template", newTemplate.GetName())
		// Return a warning to inform the user that workspaces will be validated
		return admission.Warnings{"Template constraints changed. Affected workspaces will be marked for compliance validation by the controller."}, nil
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
	if !equality.Semantic.DeepEqual(oldSpec.AllowedImages, newSpec.AllowedImages) {
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

	// Use semantic equality which properly handles resource.Quantity comparison.
	// Semantic.DeepEqual uses Quantity.Cmp() internally to compare numeric values
	// rather than internal representations, ensuring "1000m" equals "1".
	// See: https://github.com/kubernetes/kubernetes/issues/82242
	return !equality.Semantic.DeepEqual(oldBounds, newBounds)
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

	// Check MinIdleTimeoutInMinutes
	if (oldOverrides.MinIdleTimeoutInMinutes == nil) != (newOverrides.MinIdleTimeoutInMinutes == nil) {
		return true
	}
	if oldOverrides.MinIdleTimeoutInMinutes != nil && *oldOverrides.MinIdleTimeoutInMinutes != *newOverrides.MinIdleTimeoutInMinutes {
		return true
	}

	// Check MaxIdleTimeoutInMinutes
	if (oldOverrides.MaxIdleTimeoutInMinutes == nil) != (newOverrides.MaxIdleTimeoutInMinutes == nil) {
		return true
	}
	if oldOverrides.MaxIdleTimeoutInMinutes != nil && *oldOverrides.MaxIdleTimeoutInMinutes != *newOverrides.MaxIdleTimeoutInMinutes {
		return true
	}

	return false
}
