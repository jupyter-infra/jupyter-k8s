/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/internal/stringutil"
	webhookconst "github.com/jupyter-infra/jupyter-k8s/internal/webhook"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// log is for logging in this package.
var workspacelog = logf.Log.WithName("workspace-resource")

// ensureTemplateFinalizer ensures the template has a finalizer to prevent deletion while in use.
// Uses lazy finalizer pattern: only adds finalizer if at least one active workspace uses the template.
// Accepts workspaces with DeletionTimestamp set (they'll be deleted eventually).
func ensureTemplateFinalizer(ctx context.Context, k8sClient client.Client, templateName string, templateNamespace string) error {
	if templateName == "" {
		return nil
	}

	// Check if at least 1 active workspace uses this template (limit=1 for efficiency)
	// ListActiveWorkspacesByTemplate filters out workspaces with DeletionTimestamp != nil
	workspaces, _, err := workspaceutil.ListActiveWorkspacesByTemplate(ctx, k8sClient, templateName, templateNamespace, "", 1)
	if err != nil {
		workspacelog.Error(err, "Failed to check workspace usage", "template", templateName, "templateNamespace", templateNamespace)
		return fmt.Errorf("failed to check workspace usage for template %s/%s: %w", templateNamespace, templateName, err)
	}

	// If no active workspaces use the template, don't add finalizer
	// This implements lazy finalizer pattern - controller will add it when needed
	if len(workspaces) == 0 {
		workspacelog.V(1).Info("No active workspaces use template, skipping finalizer", "template", templateName, "templateNamespace", templateNamespace)
		return nil
	}

	// Fetch the template
	template := &workspacev1alpha1.WorkspaceTemplate{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: templateName, Namespace: templateNamespace}, template); err != nil {
		// Don't fail webhook if template doesn't exist - let validation handle it
		workspacelog.Info("Template not found during finalizer check", "template", templateName, "templateNamespace", templateNamespace, "error", err)
		return nil
	}

	// Check if finalizer already exists
	if controllerutil.ContainsFinalizer(template, workspaceutil.TemplateFinalizerName) {
		return nil
	}

	// Add finalizer since active workspace(s) use this template
	controllerutil.AddFinalizer(template, workspaceutil.TemplateFinalizerName)
	if err := k8sClient.Update(ctx, template); err != nil {
		workspacelog.Error(err, "Failed to add finalizer to template", "template", templateName, "templateNamespace", templateNamespace)
		return fmt.Errorf("failed to add finalizer to template %s/%s: %w", templateNamespace, templateName, err)
	}

	workspacelog.Info("Added finalizer to template", "template", templateName, "templateNamespace", templateNamespace)
	return nil
}

// ensureAccessStrategyFinalizer ensures the AccessStrategy has a finalizer to prevent deletion while in use.
// Uses lazy finalizer pattern: only adds finalizer if a workspace actively uses the AccessStrategy.
func ensureAccessStrategyFinalizer(ctx context.Context, k8sClient client.Client, workspace *workspacev1alpha1.Workspace) error {
	// If no AccessStrategy is referenced, there's nothing to do
	if workspace.Spec.AccessStrategy == nil || workspace.Spec.AccessStrategy.Name == "" {
		return nil
	}

	// Skip if workspace is being deleted
	if !workspace.DeletionTimestamp.IsZero() {
		workspacelog.V(1).Info("Workspace is being deleted, skipping AccessStrategy finalizer",
			"workspace", workspace.Name,
			"namespace", workspace.Namespace)
		return nil
	}

	// Determine namespace for the AccessStrategy
	accessStrategyNamespace := workspace.Namespace
	if workspace.Spec.AccessStrategy.Namespace != "" {
		accessStrategyNamespace = workspace.Spec.AccessStrategy.Namespace
	}

	// Fetch the AccessStrategy
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      workspace.Spec.AccessStrategy.Name,
		Namespace: accessStrategyNamespace,
	}, accessStrategy)

	if err != nil {
		if errors.IsNotFound(err) {
			// Fail webhook if AccessStrategy doesn't exist
			workspacelog.Info("AccessStrategy not found",
				"accessStrategy", workspace.Spec.AccessStrategy.Name,
				"namespace", accessStrategyNamespace)
			return fmt.Errorf("referenced AccessStrategy %s not found in namespace %s",
				workspace.Spec.AccessStrategy.Name, accessStrategyNamespace)
		}
		// Other errors
		workspacelog.Error(err, "Failed to get AccessStrategy",
			"accessStrategy", workspace.Spec.AccessStrategy.Name,
			"namespace", accessStrategyNamespace)
		return fmt.Errorf("failed to get AccessStrategy %s in namespace %s: %w",
			workspace.Spec.AccessStrategy.Name, accessStrategyNamespace, err)
	}

	// Check if finalizer already exists
	if controllerutil.ContainsFinalizer(accessStrategy, workspaceutil.AccessStrategyFinalizerName) {
		return nil
	}

	// Use the safe utility to add finalizer (handles conflicts)
	err = workspaceutil.SafelyAddFinalizerToAccessStrategy(ctx, workspacelog, k8sClient, accessStrategy,
		workspaceutil.AccessStrategyFinalizerName)
	if err != nil {
		workspacelog.Error(err, "Failed to add finalizer to AccessStrategy",
			"accessStrategy", accessStrategy.Name,
			"namespace", accessStrategy.Namespace)
		return fmt.Errorf("failed to add finalizer to AccessStrategy %s/%s: %w",
			accessStrategy.Namespace, accessStrategy.Name, err)
	}
	return nil
}

// getEffectiveOwnershipType returns the effective access type, treating empty as Public
// TODO: think of better way to convey defaults to user.
func getEffectiveOwnershipType(ownershipType string) string {
	if ownershipType == "" {
		return webhookconst.OwnershipTypePublic
	}
	return ownershipType
}

// isControllerOrAdminUser checks if the user is the controller service account or has admin privileges
func isControllerOrAdminUser(ctx context.Context) bool {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return false
	}

	// Check if user is controller
	controllerServiceAccount := os.Getenv(controller.ControllerPodServiceAccountEnv)
	controllerNamespace := os.Getenv(controller.ControllerPodNamespaceEnv)
	if controllerServiceAccount != "" && controllerNamespace != "" {
		// Build the full service account name: system:serviceaccount:namespace:name
		fullControllerSA := fmt.Sprintf("system:serviceaccount:%s:%s", controllerNamespace, controllerServiceAccount)
		if req.UserInfo.Username == fullControllerSA {
			return true
		}
	}

	// Check if user is admin
	adminGroups := []string{webhookconst.DefaultAdminGroup}
	if clusterAdminGroup := os.Getenv("CLUSTER_ADMIN_GROUP"); clusterAdminGroup != "" {
		adminGroups = append(adminGroups, clusterAdminGroup)
	}
	for _, group := range req.UserInfo.Groups {
		for _, adminGroup := range adminGroups {
			if group == adminGroup {
				return true
			}
		}
	}

	return false
}

// validateOwnershipPermission checks if the user has permission to modify/delete an OwnerOnly workspace
func validateOwnershipPermission(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	req, err := admission.RequestFromContext(ctx)
	if err != nil {
		return fmt.Errorf("unable to extract user information from request context: %w", err)
	}

	currentUser := stringutil.SanitizeUsername(req.UserInfo.Username)
	workspacelog.Info("Validating ownership permission", "currentUser", currentUser)

	// Check if user is the owner
	if workspace.Annotations != nil {
		if createdBy := workspace.Annotations[controller.AnnotationCreatedBy]; createdBy != "" {
			workspacelog.Info("Checking ownership", "createdBy", createdBy, "currentUser", currentUser, "match", createdBy == currentUser)
			if createdBy == currentUser {
				return nil
			}
		}
	}

	return fmt.Errorf("access denied: only workspace owner can modify OwnerOnly workspaces")
}

// SetupWorkspaceWebhookWithManager registers the webhook for Workspace in the manager.
// RBAC Note: This webhook requires WorkspaceTemplate access (get, update, finalizers/update)
// which is provided by the workspacetemplate controller RBAC markers.
func SetupWorkspaceWebhookWithManager(mgr ctrl.Manager, defaultTemplateNamespace string) error {
	templateValidator := NewTemplateValidator(mgr.GetClient(), defaultTemplateNamespace)
	accessStrategyValidator := NewAccessStrategyValidator(defaultTemplateNamespace)
	templateDefaulter := NewTemplateDefaulter(mgr.GetClient(), defaultTemplateNamespace)
	templateGetter := NewTemplateGetter(mgr.GetClient(), defaultTemplateNamespace)
	serviceAccountValidator := NewServiceAccountValidator(mgr.GetClient())
	serviceAccountDefaulter := NewServiceAccountDefaulter(mgr.GetClient())
	volumeValidator := NewVolumeValidator(mgr.GetClient())
	integrationTemplateRefValidator := NewIntegrationTemplateRefValidator(mgr.GetClient(), defaultTemplateNamespace)

	return ctrl.NewWebhookManagedBy(mgr, &workspacev1alpha1.Workspace{}).
		WithValidator(&WorkspaceCustomValidator{
			templateValidator:               templateValidator,
			accessStrategyValidator:         accessStrategyValidator,
			serviceAccountValidator:         serviceAccountValidator,
			volumeValidator:                 volumeValidator,
			integrationTemplateRefValidator: integrationTemplateRefValidator,
		}).
		WithDefaulter(&WorkspaceCustomDefaulter{
			templateDefaulter:       templateDefaulter,
			serviceAccountDefaulter: serviceAccountDefaulter,
			templateGetter:          templateGetter,
			templateValidator:       templateValidator,
			accessStrategyValidator: accessStrategyValidator,
			client:                  mgr.GetClient(),
		}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-workspace-jupyter-org-v1alpha1-workspace,mutating=true,failurePolicy=fail,sideEffects=None,groups=workspace.jupyter.org,resources=workspaces,verbs=create;update,versions=v1alpha1,name=mworkspace-v1alpha1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

// WorkspaceCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind Workspace when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type WorkspaceCustomDefaulter struct {
	templateDefaulter       *TemplateDefaulter
	serviceAccountDefaulter *ServiceAccountDefaulter
	templateGetter          *TemplateGetter
	templateValidator       *TemplateValidator
	accessStrategyValidator *AccessStrategyValidator
	client                  client.Client
}

var _ admission.Defaulter[*workspacev1alpha1.Workspace] = &WorkspaceCustomDefaulter{}

// Default implements admission.Defaulter so a webhook will be registered for the Kind Workspace.
func (d *WorkspaceCustomDefaulter) Default(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	workspacelog.Info("Defaulting for Workspace", "name", workspace.GetName(), "namespace", workspace.GetNamespace())

	// Skip template defaulting if workspace is being deleted
	// During deletion, only finalizer removal happens and we don't need to apply defaults
	// This prevents webhook failures when template is already deleted
	if !workspace.DeletionTimestamp.IsZero() {
		workspacelog.Info("Skipping defaulting for workspace being deleted", "name", workspace.GetName())
		return nil
	}

	if workspace.Spec.DesiredStatus == "" {
		workspace.Spec.DesiredStatus = controller.DefaultDesiredStatus
	}

	// Add ownership tracking annotations
	if workspace.Annotations == nil {
		workspace.Annotations = make(map[string]string)
	}

	// Extract user info from request context
	if req, err := admission.RequestFromContext(ctx); err == nil {
		sanitizedUsername := stringutil.SanitizeUsername(req.UserInfo.Username)

		// Always set created-by on CREATE operations
		if req.Operation == "CREATE" {
			workspace.Annotations[controller.AnnotationCreatedBy] = sanitizedUsername
			workspacelog.Info("Added created-by annotation", "workspace", workspace.GetName(), "user", sanitizedUsername, "namespace", workspace.GetNamespace())
		}

		// Always set last-updated-by (CREATE and UPDATE operations)
		workspace.Annotations[controller.AnnotationLastUpdatedBy] = sanitizedUsername
		workspacelog.Info("Added last-updated-by annotation", "workspace", workspace.GetName(), "user", sanitizedUsername, "namespace", workspace.GetNamespace())
	}

	// Apply template getter
	if err := d.templateGetter.ApplyTemplateName(ctx, workspace); err != nil {
		workspacelog.Error(err, "Failed to apply template reference", "workspace", workspace.GetName())
		return fmt.Errorf("failed to apply template reference: %w", err)
	}

	// Apply template defaults
	if err := d.templateDefaulter.ApplyTemplateDefaults(ctx, workspace); err != nil {
		workspacelog.Error(err, "Failed to apply template defaults", "workspace", workspace.GetName())
		return fmt.Errorf("failed to apply template defaults: %w", err)
	}

	// Apply service account defaults
	if err := d.serviceAccountDefaulter.ApplyServiceAccountDefaults(ctx, workspace); err != nil {
		workspacelog.Error(err, "Failed to apply service account defaults", "workspace", workspace.GetName())
		return fmt.Errorf("failed to apply service account defaults: %w", err)
	}

	// Set workspace defaults for OwnershipType and AccessType
	setWorkspaceSharingDefaults(workspace)

	// Validate the namespace scope of BOTH referenced resources before stamping ANY protection
	// finalizer. Stamping a finalizer is a side effect on another object that the API server will
	// not roll back when this admission is later rejected. Reordering to validate-all then act-all
	// ensures an out-of-scope access strategy reference can never leave a finalizer behind on the
	// template (and vice versa). Existence and constraint checks still run in the validating webhook.
	if workspace.Spec.TemplateRef != nil && workspace.Spec.TemplateRef.Name != "" {
		if err := d.templateValidator.ValidateNamespaceScope(workspace); err != nil {
			return err
		}
	}
	if err := d.accessStrategyValidator.ValidateCreateWorkspace(workspace); err != nil {
		return err
	}

	// Ensure template has finalizer to prevent deletion while in use.
	if workspace.Spec.TemplateRef != nil && workspace.Spec.TemplateRef.Name != "" {
		templateNamespace := workspaceutil.GetTemplateRefNamespace(workspace)
		if err := ensureTemplateFinalizer(ctx, d.client, workspace.Spec.TemplateRef.Name, templateNamespace); err != nil {
			workspacelog.Error(err, "Failed to add finalizer to template", "workspace", workspace.GetName(), "template", workspace.Spec.TemplateRef.Name, "templateNamespace", templateNamespace)
			return fmt.Errorf("failed to add finalizer to template: %w", err)
		}
	}

	// Ensure AccessStrategy has finalizer to prevent deletion while in use.
	if err := ensureAccessStrategyFinalizer(ctx, d.client, workspace); err != nil {
		workspacelog.Error(err, "Failed to add finalizer to AccessStrategy", "workspace", workspace.GetName())
		return fmt.Errorf("failed to add finalizer to AccessStrategy: %w", err)
	}

	return nil
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate-workspace-jupyter-org-v1alpha1-workspace,mutating=false,failurePolicy=fail,sideEffects=None,groups=workspace.jupyter.org,resources=workspaces,verbs=create;update;delete,versions=v1alpha1,name=vworkspace-v1alpha1.kb.io,admissionReviewVersions=v1,serviceName=jupyter-k8s-controller-manager,servicePort=9443

// WorkspaceCustomValidator struct is responsible for validating the Workspace resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type WorkspaceCustomValidator struct {
	templateValidator               *TemplateValidator
	accessStrategyValidator         *AccessStrategyValidator
	serviceAccountValidator         *ServiceAccountValidator
	volumeValidator                 *VolumeValidator
	integrationTemplateRefValidator *IntegrationTemplateRefValidator
}

var _ admission.Validator[*workspacev1alpha1.Workspace] = &WorkspaceCustomValidator{}

// ValidateCreate implements admission.Validator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateCreate(ctx context.Context, workspace *workspacev1alpha1.Workspace) (admission.Warnings, error) {
	workspacelog.Info("Validation for Workspace upon creation", "name", workspace.GetName(), "namespace", workspace.GetNamespace())

	// Validate template constraints
	if err := v.templateValidator.ValidateCreateWorkspace(ctx, workspace); err != nil {
		return nil, err
	}

	// Validate access strategy namespace scope
	if err := v.accessStrategyValidator.ValidateCreateWorkspace(workspace); err != nil {
		return nil, err
	}

	// Validate integrationTemplateRefs: reject a ref that targets a disallowed namespace, references a
	// missing template, or omits a required parameter (all user errors, applied to every caller on create).
	// Supplied-but-undeclared parameters are surfaced as non-blocking warnings.
	var warnings admission.Warnings
	if v.integrationTemplateRefValidator != nil {
		w, err := v.integrationTemplateRefValidator.Validate(ctx, workspace)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, w...)
	}

	// Validate volume ownership (security check - applies to all users)
	if err := v.volumeValidator.ValidateVolumeOwnership(ctx, workspace); err != nil {
		return nil, err
	}

	// Controller or admin users bypass validation
	if isControllerOrAdminUser(ctx) {
		return warnings, nil
	}

	// Validate no user-submitted reserved prefix labels/annotations
	if err := validateReservedPrefixOnCreate(workspace); err != nil {
		return nil, err
	}

	// Validate service account access
	if err := v.serviceAccountValidator.ValidateServiceAccountAccess(ctx, workspace); err != nil {
		return nil, err
	}

	return warnings, nil
}

// ValidateUpdate implements admission.Validator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateUpdate(ctx context.Context, oldWorkspace, newWorkspace *workspacev1alpha1.Workspace) (admission.Warnings, error) {
	workspacelog.Info("Validation for Workspace upon update", "name", newWorkspace.GetName(), "namespace", newWorkspace.GetNamespace())

	// Skip validation if workspace is being deleted (has deletionTimestamp)
	// This allows finalizer removal even if template is already deleted
	if !newWorkspace.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	// Controller or admin users bypass validation
	isAdmin := isControllerOrAdminUser(ctx)

	// NOTE: Removed templateRef immutability check to enable template mutability (PR #129)
	// Templates can now be changed after workspace creation

	// Admin users bypass user validation
	if isAdmin {
		return nil, nil
	}

	// Validate integrationTemplateRefs -- AFTER the admin/controller bypass and only when the user
	// actually changed the refs, mirroring how templateRef validation runs post-bypass on changed spec.
	//
	// INTENTIONAL ASYMMETRY -- do NOT "align" this with ValidateCreate, which validates BEFORE the bypass.
	// On UPDATE the check MUST sit after the bypass (this is a wedge fix, matching the WorkspaceTemplate
	// precedent below): a controller finalizer/label PATCH, or an admin's out-of-band template edit (e.g.
	// adding a required parameter to, or deleting, a template a live workspace already references), would
	// otherwise re-validate the workspace against a since-changed/-deleted template and wedge every
	// reconcile -- locking the operator (and admins) out of an already-admitted workspace. Placing it after
	// the bypass lets controller/admin writes through, and the integrationRefsChanged gate means ordinary
	// user PATCHes that don't touch the refs are never re-validated either. Moving it before the bypass (to
	// match CREATE) reintroduces the wedge. On CREATE there is no live workspace to wedge, so the check runs
	// before the bypass there. Non-blocking warnings (unused parameters) are surfaced to the user.
	var warnings admission.Warnings
	if v.integrationTemplateRefValidator != nil && integrationRefsChanged(&oldWorkspace.Spec, &newWorkspace.Spec) {
		w, err := v.integrationTemplateRefValidator.Validate(ctx, newWorkspace)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, w...)
	}

	// Validate no user modifications to reserved prefix labels/annotations
	if err := validateReservedPrefixOnUpdate(oldWorkspace, newWorkspace); err != nil {
		return nil, err
	}

	// Validate service account access for new workspace
	if err := v.serviceAccountValidator.ValidateServiceAccountAccess(ctx, newWorkspace); err != nil {
		return nil, err
	}

	originalOwnershipType := getEffectiveOwnershipType(oldWorkspace.Spec.OwnershipType)
	newOwnershipType := getEffectiveOwnershipType(newWorkspace.Spec.OwnershipType)
	workspacelog.Info("Ownership validation check", "originalType", originalOwnershipType, "newType", newOwnershipType)
	// For OwnerOnly workspaces, check if user has permission
	if originalOwnershipType == webhookconst.OwnershipTypeOwnerOnly {
		// Existing OwnerOnly workspace - check against old workspace
		if err := validateOwnershipPermission(ctx, oldWorkspace); err != nil {
			return nil, err
		}
	} else if newOwnershipType == webhookconst.OwnershipTypeOwnerOnly {
		// Changing to OwnerOnly - only allow if user is the original creator
		if err := validateOwnershipPermission(ctx, oldWorkspace); err != nil {
			return nil, err
		}
	}

	// Validate template constraints for new workspace (only changed fields)
	if err := v.templateValidator.ValidateUpdateWorkspace(ctx, oldWorkspace, newWorkspace); err != nil {
		return nil, err
	}

	// Validate access strategy namespace scope
	if err := v.accessStrategyValidator.ValidateUpdateWorkspace(oldWorkspace, newWorkspace); err != nil {
		return nil, err
	}

	// Validate volume ownership (security check - applies to all users)
	if err := v.volumeValidator.ValidateVolumeOwnership(ctx, newWorkspace); err != nil {
		return nil, err
	}

	return warnings, nil
}

// ValidateDelete implements admission.Validator so a webhook will be registered for the type Workspace.
func (v *WorkspaceCustomValidator) ValidateDelete(ctx context.Context, workspace *workspacev1alpha1.Workspace) (admission.Warnings, error) {
	workspacelog.Info("Validation for Workspace upon deletion", "name", workspace.GetName(), "namespace", workspace.GetNamespace())

	// Controller or admin users bypass validation
	if isControllerOrAdminUser(ctx) {
		return nil, nil
	}

	// For OwnerOnly workspaces, check if user has permission
	effectiveOwnershipType := getEffectiveOwnershipType(workspace.Spec.OwnershipType)
	if effectiveOwnershipType == webhookconst.OwnershipTypeOwnerOnly {
		if err := validateOwnershipPermission(ctx, workspace); err != nil {
			return nil, err
		}
	}

	return nil, nil
}
