/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// log is for logging in this package.
var templatelog = logf.Log.WithName("workspacetemplate-resource")

// SetupWorkspaceTemplateWebhookWithManager registers the webhook for WorkspaceTemplate in the manager.
func SetupWorkspaceTemplateWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&workspacev1alpha1.WorkspaceTemplate{}).
		WithValidator(&WorkspaceTemplateCustomValidator{}).
		WithDefaulter(&WorkspaceTemplateCustomDefaulter{client: mgr.GetClient()}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-workspace-jupyter-org-v1alpha1-workspacetemplate,mutating=true,failurePolicy=ignore,sideEffects=None,groups=workspace.jupyter.org,resources=workspacetemplates,verbs=create;update,versions=v1alpha1,name=mworkspacetemplate-v1alpha1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

// WorkspaceTemplateCustomDefaulter stamps the access strategy lookup labels on a WorkspaceTemplate so
// the access strategy deletion webhook can find referencing templates with an efficient label query,
// and eagerly adds the protection finalizer to the referenced AccessStrategy so it cannot be deleted
// while the template references it. It mirrors how the workspace defaulter stamps template lookup
// labels and finalizes the AccessStrategy.
//
// Uses failurePolicy: Ignore so template writes are not blocked if the webhook is unavailable; the
// controller backfills both the labels and the finalizer as a safety net.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type WorkspaceTemplateCustomDefaulter struct {
	client client.Client
}

var _ webhook.CustomDefaulter = &WorkspaceTemplateCustomDefaulter{}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind WorkspaceTemplate.
func (d *WorkspaceTemplateCustomDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	template, ok := obj.(*workspacev1alpha1.WorkspaceTemplate)
	if !ok {
		return fmt.Errorf("expected a WorkspaceTemplate object but got %T", obj)
	}

	// Skip on delete (defaulters don't run on delete, but guard defensively) and when being deleted.
	if !template.DeletionTimestamp.IsZero() {
		return nil
	}

	if workspaceutil.ApplyAccessStrategyLabels(template) {
		templatelog.Info("Stamped access strategy labels on WorkspaceTemplate",
			"name", template.GetName(),
			"accessStrategyName", template.Labels[workspaceutil.LabelAccessStrategyName],
			"accessStrategyNamespace", template.Labels[workspaceutil.LabelAccessStrategyNamespace])
	}

	// Eagerly add the protection finalizer to the referenced AccessStrategy (lazy finalizer pattern):
	// add when referenced, never remove here - the AccessStrategy controller removes it when the last
	// referrer (across workspaces and templates) is gone. mustExist=true rejects the template write when
	// the referenced AccessStrategy does not exist, matching the workspace webhook.
	if template.Spec.DefaultAccessStrategy != nil && template.Spec.DefaultAccessStrategy.Name != "" {
		asName := template.Labels[workspaceutil.LabelAccessStrategyName]
		asNamespace := template.Labels[workspaceutil.LabelAccessStrategyNamespace]
		if err := workspaceutil.EnsureAccessStrategyFinalizerByRef(ctx, templatelog, d.client, asName, asNamespace,
			workspaceutil.AccessStrategyTemplateFinalizerName, true); err != nil {
			templatelog.Error(err, "Failed to add finalizer to AccessStrategy referenced by template",
				"template", template.GetName(), "accessStrategy", asName, "namespace", asNamespace)
			return fmt.Errorf("failed to add finalizer to AccessStrategy referenced by template: %w", err)
		}
	}

	return nil
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

	// Check EnvRequirements changes
	if !equality.Semantic.DeepEqual(oldSpec.EnvRequirements, newSpec.EnvRequirements) {
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
