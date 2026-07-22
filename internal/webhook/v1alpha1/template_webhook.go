/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// log is for logging in this package.
var templatelog = logf.Log.WithName("workspacetemplate-resource")

// SetupWorkspaceTemplateWebhookWithManager registers the webhook for WorkspaceTemplate in the manager.
func SetupWorkspaceTemplateWebhookWithManager(mgr ctrl.Manager, defaultTemplateNamespace string) error {
	accessStrategyValidator := NewAccessStrategyValidator(defaultTemplateNamespace)
	return ctrl.NewWebhookManagedBy(mgr, &workspacev1alpha1.WorkspaceTemplate{}).
		WithValidator(&WorkspaceTemplateCustomValidator{
			accessStrategyValidator: accessStrategyValidator,
		}).
		WithDefaulter(&WorkspaceTemplateCustomDefaulter{
			client:                  mgr.GetClient(),
			accessStrategyValidator: accessStrategyValidator,
		}).
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
	client                  client.Client
	accessStrategyValidator *AccessStrategyValidator
}

var _ admission.Defaulter[*workspacev1alpha1.WorkspaceTemplate] = &WorkspaceTemplateCustomDefaulter{}

// Default implements admission.Defaulter so a webhook will be registered for the Kind WorkspaceTemplate.
func (d *WorkspaceTemplateCustomDefaulter) Default(ctx context.Context, template *workspacev1alpha1.WorkspaceTemplate) error {

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
	// Check namespace scope first so we never modify an AccessStrategy in a disallowed namespace.
	if template.Spec.DefaultAccessStrategy != nil && template.Spec.DefaultAccessStrategy.Name != "" {
		if err := d.accessStrategyValidator.ValidateCreateTemplate(template); err != nil {
			return err
		}
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

// +kubebuilder:webhook:path=/validate-workspace-jupyter-org-v1alpha1-workspacetemplate,mutating=false,failurePolicy=ignore,sideEffects=None,groups=workspace.jupyter.org,resources=workspacetemplates,verbs=create;update,versions=v1alpha1,name=vworkspacetemplate-v1alpha1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

// WorkspaceTemplateCustomValidator struct is responsible for validating the WorkspaceTemplate resource
// when it is created or updated. On create/update it enforces that any referenced access strategy lives
// in an allowed namespace (the template's own or the shared namespace), so the template cannot make
// referencing workspaces un-admittable. On update it also checks if constraint fields changed and
// returns warnings; the WorkspaceTemplate controller is responsible for marking affected workspaces
// for compliance checking.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type WorkspaceTemplateCustomValidator struct {
	accessStrategyValidator *AccessStrategyValidator
}

var _ admission.Validator[*workspacev1alpha1.WorkspaceTemplate] = &WorkspaceTemplateCustomValidator{}

// ValidateCreate implements admission.Validator so a webhook will be registered for the type WorkspaceTemplate.
func (v *WorkspaceTemplateCustomValidator) ValidateCreate(ctx context.Context, template *workspacev1alpha1.WorkspaceTemplate) (admission.Warnings, error) {
	templatelog.Info("Validation for WorkspaceTemplate upon creation", "name", template.GetName())

	// Enforce that the referenced access strategy is in an allowed namespace, so the template
	// cannot make referencing workspaces fail their own admission webhook.
	if err := v.accessStrategyValidator.ValidateCreateTemplate(template); err != nil {
		return nil, err
	}

	// Enforce that the template's own constraints are internally consistent, so it cannot make
	// its own defaults or any workspace value un-admittable.
	if err := validateTemplateConsistency(template); err != nil {
		return nil, err
	}

	return nil, nil
}

// ValidateUpdate implements admission.Validator so a webhook will be registered for the type WorkspaceTemplate.
func (v *WorkspaceTemplateCustomValidator) ValidateUpdate(ctx context.Context, oldTemplate, newTemplate *workspacev1alpha1.WorkspaceTemplate) (admission.Warnings, error) {
	templatelog.Info("Validation for WorkspaceTemplate upon update", "name", newTemplate.GetName())

	// Enforce that the referenced access strategy is in an allowed namespace, so the template
	// cannot make referencing workspaces fail their own admission webhook.
	if err := v.accessStrategyValidator.ValidateUpdateTemplate(oldTemplate, newTemplate); err != nil {
		return nil, err
	}

	// Enforce that the template's own constraints are internally consistent, so it cannot make
	// its own defaults or any workspace value un-admittable.
	if err := validateTemplateConsistency(newTemplate); err != nil {
		return nil, err
	}

	// Check if constraint fields changed
	if constraintsChanged(oldTemplate, newTemplate) {
		templatelog.Info("Template constraints changed, controller will mark workspaces for compliance check", "template", newTemplate.GetName())
		// Return a warning to inform the user that workspaces will be validated
		return admission.Warnings{"Template constraints changed. Affected workspaces will be marked for compliance validation by the controller."}, nil
	}

	return nil, nil
}

// ValidateDelete implements admission.Validator so a webhook will be registered for the type WorkspaceTemplate.
func (v *WorkspaceTemplateCustomValidator) ValidateDelete(ctx context.Context, template *workspacev1alpha1.WorkspaceTemplate) (admission.Warnings, error) {
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

// validateTemplateConsistency rejects a template whose own constraints are internally
// inconsistent - contradictions that would make the template's defaults or any workspace value
// un-admittable, silently self-defeating the template. These checks run on create and update.
func validateTemplateConsistency(template *workspacev1alpha1.WorkspaceTemplate) error {
	// defaultImage must be creatable under the template's own image policy (#440).
	if err := validateTemplateImageConsistency(template); err != nil {
		return err
	}

	// primaryStorage minSize must not exceed maxSize.
	if err := validateTemplateStorageConsistency(template); err != nil {
		return err
	}

	// resourceBounds min must not exceed max for any resource.
	if err := validateTemplateResourceBoundsConsistency(template); err != nil {
		return err
	}

	// idleShutdownOverrides bounds must be consistent, and a locked policy needs a default.
	return validateIdleShutdownPolicyConsistency(template)
}

// validateIdleShutdownPolicyConsistency rejects a template whose idle shutdown policy is
// self-defeating: bounds that no timeout can satisfy, a locked policy with no default to enforce
// against, or a default timeout that its own bounds would reject. All are surfaced at template
// admission instead of silently making the template's default un-creatable.
func validateIdleShutdownPolicyConsistency(template *workspacev1alpha1.WorkspaceTemplate) error {
	policy := template.Spec.IdleShutdownOverrides
	if policy == nil {
		return nil
	}

	// The timeout bounds must be internally consistent regardless of the Allow setting: a
	// min greater than max can never admit any workspace timeout.
	if policy.MinIdleTimeoutInMinutes != nil && policy.MaxIdleTimeoutInMinutes != nil &&
		*policy.MinIdleTimeoutInMinutes > *policy.MaxIdleTimeoutInMinutes {
		return fmt.Errorf(
			"idleShutdownOverrides.minIdleTimeoutInMinutes %d is greater than maxIdleTimeoutInMinutes %d: "+
				"no idle timeout can satisfy these bounds (template %q)",
			*policy.MinIdleTimeoutInMinutes, *policy.MaxIdleTimeoutInMinutes, template.GetName(),
		)
	}

	// A locked policy (allow=false) needs a default for workspaces to match against.
	if policy.Allow != nil && !*policy.Allow && template.Spec.DefaultIdleShutdown == nil {
		return fmt.Errorf(
			"idleShutdownOverrides.allow is false but defaultIdleShutdown is not set: "+
				"a locked idle shutdown policy requires a defaultIdleShutdown for workspaces to match (template %q)",
			template.GetName(),
		)
	}

	// The default timeout must satisfy the declared bounds, regardless of the Allow setting: the
	// defaulter applies defaultIdleShutdown to any workspace that omits idleShutdown, and the
	// workspace webhook then enforces the bounds on that enabled default. A default outside the
	// bounds would make the template's own default un-creatable.
	def := template.Spec.DefaultIdleShutdown
	if def != nil && def.Enabled {
		if policy.MinIdleTimeoutInMinutes != nil && def.IdleTimeoutInMinutes < *policy.MinIdleTimeoutInMinutes {
			return fmt.Errorf(
				"defaultIdleShutdown.idleTimeoutInMinutes %d is below idleShutdownOverrides.minIdleTimeoutInMinutes %d: "+
					"the template default would be rejected by its own bounds (template %q)",
				def.IdleTimeoutInMinutes, *policy.MinIdleTimeoutInMinutes, template.GetName(),
			)
		}
		if policy.MaxIdleTimeoutInMinutes != nil && def.IdleTimeoutInMinutes > *policy.MaxIdleTimeoutInMinutes {
			return fmt.Errorf(
				"defaultIdleShutdown.idleTimeoutInMinutes %d is above idleShutdownOverrides.maxIdleTimeoutInMinutes %d: "+
					"the template default would be rejected by its own bounds (template %q)",
				def.IdleTimeoutInMinutes, *policy.MaxIdleTimeoutInMinutes, template.GetName(),
			)
		}
	}

	return nil
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
