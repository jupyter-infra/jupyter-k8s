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

package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

const templateFinalizerName = "workspace.jupyter.org/template-protection"

// WorkspaceTemplateReconciler reconciles a WorkspaceTemplate object
type WorkspaceTemplateReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	recorder         record.EventRecorder
	templateResolver *TemplateResolver
}

// +kubebuilder:rbac:groups=workspace.jupyter.org,resources=workspacetemplates,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=workspace.jupyter.org,resources=workspacetemplates/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *WorkspaceTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling WorkspaceTemplate", "template", req.Name)

	template := &workspacev1alpha1.WorkspaceTemplate{}
	if err := r.Get(ctx, req.NamespacedName, template); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("WorkspaceTemplate not found, assuming deleted")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !template.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, template)
	}

	// Manage finalizer based on workspace usage (lazy finalizer pattern)
	// This follows Kubernetes best practice: only add finalizers when needed
	result, err := r.manageFinalizer(ctx, template)
	if err != nil {
		return result, err
	}

	// Check compliance for all workspaces using this template
	// This is informational only - does not block reconciliation
	if err := r.checkWorkspaceCompliance(ctx, template); err != nil {
		logger.Error(err, "Failed to check workspace compliance")
		// Don't fail reconciliation - compliance checking is best-effort
	}

	return result, nil
}

// checkWorkspaceCompliance checks if all workspaces using this template are compliant
// This is informational only - sets status conditions but does not enforce
// Follows Kubernetes best practices: 1 LIST, 0 additional GETs, N UPDATEs
func (r *WorkspaceTemplateReconciler) checkWorkspaceCompliance(ctx context.Context, template *workspacev1alpha1.WorkspaceTemplate) error {
	logger := logf.FromContext(ctx)

	// Get all workspaces using this template (1 LIST operation)
	workspaces, err := r.templateResolver.ListWorkspacesUsingTemplate(ctx, template.Name)
	if err != nil {
		return fmt.Errorf("failed to list workspaces using template: %w", err)
	}

	if len(workspaces) == 0 {
		logger.V(1).Info("No workspaces using template, skipping compliance check")
		return nil
	}

	logger.V(1).Info("Checking compliance for workspaces", "count", len(workspaces))

	// Check each workspace for compliance (use workspaces from LIST, no additional GETs)
	for i := range workspaces {
		workspace := &workspaces[i] // Use from LIST directly

		// Validate workspace against current template
		result, err := r.templateResolver.ValidateAndResolveTemplate(ctx, workspace)
		if err != nil {
			// System error (e.g., template not found) - log but continue
			logger.Error(err, "Failed to validate workspace against template",
				"workspace", fmt.Sprintf("%s/%s", workspace.Namespace, workspace.Name))
			continue
		}

		// Update compliance status condition based on validation result
		if err := r.updateComplianceStatus(ctx, workspace, result.Valid, result.Violations); err != nil {
			// Log error but continue with other workspaces
			logger.Error(err, "Failed to update compliance status",
				"workspace", fmt.Sprintf("%s/%s", workspace.Namespace, workspace.Name))
			continue
		}
	}

	return nil
}

// updateComplianceStatus updates the TemplateCompliant status condition on a workspace
func (r *WorkspaceTemplateReconciler) updateComplianceStatus(ctx context.Context, workspace *workspacev1alpha1.Workspace, isCompliant bool, violations []TemplateViolation) error {
	logger := logf.FromContext(ctx)

	// Build condition based on compliance
	var condition metav1.Condition
	if isCompliant {
		condition = metav1.Condition{
			Type:               workspacev1alpha1.ConditionTemplateCompliant,
			Status:             metav1.ConditionTrue,
			ObservedGeneration: workspace.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             workspacev1alpha1.ReasonTemplateCompliant,
			Message:            "Workspace configuration complies with current template",
		}
	} else {
		// Format violations into a message
		violationMsg := formatViolations(violations)
		condition = metav1.Condition{
			Type:               workspacev1alpha1.ConditionTemplateCompliant,
			Status:             metav1.ConditionFalse,
			ObservedGeneration: workspace.Generation,
			LastTransitionTime: metav1.Now(),
			Reason:             workspacev1alpha1.ReasonTemplateNonCompliant,
			Message:            fmt.Sprintf("Workspace configuration violates template: %s", violationMsg),
		}
	}

	// Update condition in conditions list
	setCondition(&workspace.Status.Conditions, condition)

	// Update workspace status (handle conflicts with retry logic in reconcile loop)
	if err := r.Status().Update(ctx, workspace); err != nil {
		return fmt.Errorf("failed to update workspace status: %w", err)
	}

	logger.V(1).Info("Updated compliance status",
		"workspace", fmt.Sprintf("%s/%s", workspace.Namespace, workspace.Name),
		"compliant", isCompliant)

	return nil
}

// manageFinalizer implements lazy finalizer management for WorkspaceTemplates
// Following Kubernetes best practices:
// - Finalizers are only added when workspaces start using the template
// - Finalizers are removed when all workspaces stop using the template
// - This minimizes overhead and complexity compared to adding finalizers to all templates
//
//nolint:unparam // ctrl.Result signature maintained for consistency with controller-runtime patterns
func (r *WorkspaceTemplateReconciler) manageFinalizer(ctx context.Context, template *workspacev1alpha1.WorkspaceTemplate) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Check if any active workspaces are using this template
	workspaces, err := r.templateResolver.ListWorkspacesUsingTemplate(ctx, template.Name)
	if err != nil {
		logger.Error(err, "Failed to list workspaces using template")
		return ctrl.Result{}, err
	}

	hasFinalizer := controllerutil.ContainsFinalizer(template, templateFinalizerName)
	hasWorkspaces := len(workspaces) > 0

	logger.V(1).Info("Checking finalizer state",
		"templateName", template.Name,
		"hasFinalizer", hasFinalizer,
		"workspaceCount", len(workspaces))

	// Case 1: Workspaces exist, but finalizer is missing → Add finalizer
	if hasWorkspaces && !hasFinalizer {
		logger.Info("Adding finalizer to template (workspaces are using it)",
			"finalizer", templateFinalizerName,
			"workspaceCount", len(workspaces))
		controllerutil.AddFinalizer(template, templateFinalizerName)
		if err := r.Update(ctx, template); err != nil {
			logger.Error(err, "Failed to add finalizer to template")
			return ctrl.Result{}, err
		}
		logger.Info("Successfully added finalizer to template")
		return ctrl.Result{}, nil
	}

	// Case 2: No workspaces, but finalizer is present → Remove finalizer
	// This handles the case where all workspaces were deleted
	if !hasWorkspaces && hasFinalizer {
		logger.Info("Removing finalizer from template (no workspaces using it)",
			"finalizer", templateFinalizerName)
		controllerutil.RemoveFinalizer(template, templateFinalizerName)
		if err := r.Update(ctx, template); err != nil {
			logger.Error(err, "Failed to remove finalizer from template")
			return ctrl.Result{}, err
		}
		logger.Info("Successfully removed finalizer from template")
		return ctrl.Result{}, nil
	}

	// Case 3: State is correct (both have workspaces+finalizer, or neither)
	logger.V(1).Info("Finalizer state is correct, no action needed")
	return ctrl.Result{}, nil
}

func (r *WorkspaceTemplateReconciler) handleDeletion(ctx context.Context, template *workspacev1alpha1.WorkspaceTemplate) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Handling template deletion", "templateName", template.Name)

	if !controllerutil.ContainsFinalizer(template, templateFinalizerName) {
		logger.Info("No finalizer present, allowing deletion", "templateName", template.Name)
		return ctrl.Result{}, nil
	}

	// Check if any workspaces are using this template
	workspaces, err := r.templateResolver.ListWorkspacesUsingTemplate(ctx, template.Name)
	if err != nil {
		logger.Error(err, "Failed to list workspaces using template")
		return ctrl.Result{}, err
	}

	// Log workspace names for debugging
	workspaceNames := make([]string, len(workspaces))
	for i, ws := range workspaces {
		workspaceNames[i] = fmt.Sprintf("%s/%s", ws.Namespace, ws.Name)
	}
	logger.Info("Checked workspaces using template",
		"templateName", template.Name,
		"workspaceCount", len(workspaces),
		"workspaceNames", workspaceNames)

	if len(workspaces) > 0 {
		msg := fmt.Sprintf("Cannot delete template: in use by %d workspace(s)", len(workspaces))
		logger.Info(msg, "workspaces", len(workspaces))
		r.recorder.Event(template, "Warning", "TemplateInUse", msg)

		// Don't remove finalizer - block deletion
		// Return nil (not error) - we successfully determined template is in use
		// Template will be reconciled again when workspace changes (via watch)
		return ctrl.Result{}, nil
	}

	// No workspaces using template - safe to delete
	logger.Info("No workspaces using template, removing finalizer", "templateName", template.Name)
	controllerutil.RemoveFinalizer(template, templateFinalizerName)
	if err := r.Update(ctx, template); err != nil {
		logger.Error(err, "Failed to remove finalizer from template")
		return ctrl.Result{}, err
	}

	logger.Info("Template finalizer removed - deletion allowed", "templateName", template.Name)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
// It configures watches for WorkspaceTemplate resources and triggers reconciliation
// when Workspaces change to manage finalizers based on template usage.
func (r *WorkspaceTemplateReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("workspacetemplate-setup")
	logger.Info("Setting up WorkspaceTemplate controller")

	err := ctrl.NewControllerManagedBy(mgr).
		For(&workspacev1alpha1.WorkspaceTemplate{}).
		Watches(
			&workspacev1alpha1.Workspace{},
			handler.EnqueueRequestsFromMapFunc(r.findTemplatesForWorkspace),
		).
		Named("workspacetemplate").
		Complete(r)

	if err != nil {
		logger.Error(err, "Failed to setup WorkspaceTemplate controller")
		return err
	}

	logger.Info("Successfully registered WorkspaceTemplate controller with manager")
	return nil
}

// findTemplatesForWorkspace maps a Workspace to the WorkspaceTemplate it references
// This ensures the template is reconciled when a workspace using it is created/updated/deleted
func (r *WorkspaceTemplateReconciler) findTemplatesForWorkspace(ctx context.Context, obj client.Object) []reconcile.Request {
	workspace, ok := obj.(*workspacev1alpha1.Workspace)
	if !ok {
		return nil
	}

	if workspace.Spec.TemplateRef == nil || workspace.Spec.TemplateRef.Name == "" {
		return nil
	}

	logger := logf.FromContext(ctx)
	logger.V(1).Info("Workspace changed, enqueueing template reconciliation",
		"workspace", fmt.Sprintf("%s/%s", workspace.Namespace, workspace.Name),
		"template", workspace.Spec.TemplateRef.Name)

	// Trigger reconciliation of the template when workspace changes
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Name: workspace.Spec.TemplateRef.Name,
		}},
	}
}

// SetupWorkspaceTemplateController sets up the WorkspaceTemplate controller with the Manager
func SetupWorkspaceTemplateController(mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("workspacetemplate-init")
	logger.Info("Initializing WorkspaceTemplate controller")

	k8sClient := mgr.GetClient()
	scheme := mgr.GetScheme()
	eventRecorder := mgr.GetEventRecorderFor("workspacetemplate-controller")
	templateResolver := NewTemplateResolver(k8sClient)

	reconciler := &WorkspaceTemplateReconciler{
		Client:           k8sClient,
		Scheme:           scheme,
		recorder:         eventRecorder,
		templateResolver: templateResolver,
	}

	logger.Info("Calling SetupWithManager for WorkspaceTemplate controller")
	return reconciler.SetupWithManager(mgr)
}

// formatViolations formats template violations into a human-readable message
func formatViolations(violations []TemplateViolation) string {
	if len(violations) == 0 {
		return "no violations"
	}

	msg := ""
	for i, v := range violations {
		if i > 0 {
			msg += "; "
		}
		msg += fmt.Sprintf("%s: %s (allowed: %s, actual: %s)", v.Field, v.Message, v.Allowed, v.Actual)
	}
	return msg
}

// setCondition updates or adds a condition to the conditions list
// This follows the Kubernetes meta/v1 Condition pattern
func setCondition(conditions *[]metav1.Condition, newCondition metav1.Condition) {
	if conditions == nil {
		return
	}

	// Find existing condition with same type
	for i := range *conditions {
		if (*conditions)[i].Type == newCondition.Type {
			// Update existing condition
			(*conditions)[i] = newCondition
			return
		}
	}

	// Condition not found, append new one
	*conditions = append(*conditions, newCondition)
}
