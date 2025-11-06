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
	"time"

	"golang.org/x/time/rate"
	"k8s.io/apimachinery/pkg/api/errors"
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
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
)

const (
	templateFinalizerName = "workspace.jupyter.org/template-protection"
	// ComplianceMarkingRateLimit is the rate limit for marking workspaces (workspaces per second)
	// This prevents API server overload when marking large numbers of workspaces
	ComplianceMarkingRateLimit = 10
)

// WorkspaceTemplateReconciler reconciles a WorkspaceTemplate object
type WorkspaceTemplateReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=workspace.jupyter.org,resources=workspacetemplates,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=workspace.jupyter.org,resources=workspacetemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=workspace.jupyter.org,resources=workspacetemplates/finalizers,verbs=update
// +kubebuilder:rbac:groups=workspace.jupyter.org,resources=workspaces,verbs=get;list;watch;update;patch

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

	// Handle spec changes and mark workspaces for compliance check if constraints changed
	if err := r.handleSpecChanges(ctx, template); err != nil {
		logger.Error(err, "Failed to handle spec changes")
		return ctrl.Result{}, err
	}

	// Manage finalizer based on workspace usage (lazy finalizer pattern)
	// This follows Kubernetes best practice: only add finalizers when needed
	result, err := r.manageFinalizer(ctx, template)
	if err != nil {
		return result, err
	}

	return result, nil
}

// manageFinalizer implements lazy finalizer management for WorkspaceTemplates
// Following Kubernetes best practices:
// - Finalizers are only added when workspaces start using the template
// - Finalizers are removed when all workspaces stop using the template
// - This minimizes overhead and complexity compared to adding finalizers to all templates
//
// ARCHITECTURAL NOTE: Dual Finalizer Protection Pattern
// This controller works together with the workspace admission webhook for robust finalizer management:
// 1. Webhook (Eager): Adds finalizers immediately during workspace CREATE/UPDATE admission
//   - Provides fail-fast protection at the API boundary
//   - Ensures finalizer exists before workspace is persisted
//
// 2. Controller (Lazy): Manages finalizers during reconciliation (this function)
//   - Acts as safety net if webhook fails or is temporarily unavailable
//   - Handles finalizer removal when all workspaces are deleted (webhooks can't do this)
//   - Watches workspace changes and triggers template reconciliation
//
// This dual approach ensures no race conditions and graceful handling of edge cases.
//
//nolint:unparam // ctrl.Result signature maintained for consistency with controller-runtime patterns
func (r *WorkspaceTemplateReconciler) manageFinalizer(ctx context.Context, template *workspacev1alpha1.WorkspaceTemplate) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	// Check if any active workspaces are using this template
	// Use empty namespace to match all workspaces (backwards compatible)
	workspaces, _, err := workspace.ListByTemplate(ctx, r.Client, template.Name, "", "", 0)
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
	// Use empty namespace to match all workspaces (backwards compatible)
	workspaces, _, err := workspace.ListByTemplate(ctx, r.Client, template.Name, "", "", 0)
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
		if r.recorder != nil {
			r.recorder.Event(template, "Warning", "TemplateInUse", msg)
		}

		// Don't remove finalizer - block deletion
		// Return nil (not error) - we successfully determined template is in use
		// Template will be reconciled again when workspace changes (via watch)
		return ctrl.Result{}, nil
	}

	// No workspaces using template - safe to delete
	logger.Info("No workspaces using template, removing finalizer",
		"templateName", template.Name,
		"workspaceCount", len(workspaces))
	controllerutil.RemoveFinalizer(template, templateFinalizerName)
	if err := r.Update(ctx, template); err != nil {
		logger.Error(err, "Failed to remove finalizer from template",
			"templateName", template.Name)
		return ctrl.Result{}, err
	}

	logger.Info("Template finalizer removed successfully - deletion allowed",
		"templateName", template.Name)
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
	ws, ok := obj.(*workspacev1alpha1.Workspace)
	if !ok {
		return nil
	}

	logger := logf.FromContext(ctx)

	// Use label instead of spec.templateRef because labels persist during deletion
	// When workspace is deleted, spec fields are cleared but labels remain
	// This ensures template reconciliation is triggered even during workspace deletion
	templateName := ws.Labels[workspace.LabelWorkspaceTemplate]
	if templateName == "" {
		logger.V(1).Info("Workspace has no template label, skipping template reconciliation",
			"workspace", fmt.Sprintf("%s/%s", ws.Namespace, ws.Name),
			"deletionTimestamp", ws.DeletionTimestamp)
		return nil
	}

	logger.Info("Workspace changed, enqueueing template reconciliation",
		"workspace", fmt.Sprintf("%s/%s", ws.Namespace, ws.Name),
		"template", templateName,
		"deletionTimestamp", ws.DeletionTimestamp,
		"hasLabel", true)

	// Trigger reconciliation of the template when workspace changes
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Name: templateName,
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

	reconciler := &WorkspaceTemplateReconciler{
		Client:   k8sClient,
		Scheme:   scheme,
		recorder: eventRecorder,
	}

	logger.Info("Calling SetupWithManager for WorkspaceTemplate controller")
	return reconciler.SetupWithManager(mgr)
}

// handleSpecChanges detects template spec changes using Generation field and marks workspaces for compliance
// Uses Generation-based change detection following Kubernetes API conventions:
// - metadata.generation is auto-incremented by kube-apiserver when spec changes
// - status.observedGeneration tracks the last generation processed by this controller
// - Only process each generation once (idempotent)
// This pattern is standard across all Kubernetes resources (Deployments, StatefulSets, etc.)
func (r *WorkspaceTemplateReconciler) handleSpecChanges(ctx context.Context, template *workspacev1alpha1.WorkspaceTemplate) error {
	logger := logf.FromContext(ctx)
	currentGeneration := template.Generation
	observedGeneration := template.Status.ObservedGeneration

	logger.V(1).Info("Checking template generation",
		"currentGeneration", currentGeneration,
		"observedGeneration", observedGeneration)

	// Already processed this generation
	if observedGeneration >= currentGeneration {
		logger.V(1).Info("Generation already processed, skipping")
		return nil
	}

	// First creation (generation=1, never processed before)
	if currentGeneration == 1 && observedGeneration == 0 {
		logger.Info("Template created, marking generation as observed without compliance labeling")
		return r.updateStatusObservedGeneration(ctx, template, 1)
	}

	// Spec was updated - mark workspaces for compliance check
	logger.Info("Template spec changed, marking workspaces for compliance check",
		"templateName", template.Name,
		"oldGeneration", observedGeneration,
		"newGeneration", currentGeneration)

	if err := r.markWorkspacesForCompliance(ctx, template.Name); err != nil {
		logger.Error(err, "Failed to mark workspaces for compliance")
		return err
	}

	// Update observed generation in status
	return r.updateStatusObservedGeneration(ctx, template, currentGeneration)
}

// markWorkspacesForCompliance labels all workspaces using the template with compliance-check-needed=true
// Uses controller-runtime's cached client which serves lists from memory (no pagination needed)
// Rate-limits updates to prevent API server overload when marking large numbers of workspaces
// Continues on individual failures to maximize workspaces marked
func (r *WorkspaceTemplateReconciler) markWorkspacesForCompliance(ctx context.Context, templateName string) error {
	logger := logf.FromContext(ctx)
	startTime := time.Now()
	marked := 0
	failed := 0

	// Create rate limiter: ComplianceMarkingRateLimit workspaces per second, burst of 10
	rateLimiter := rate.NewLimiter(rate.Limit(ComplianceMarkingRateLimit), 10)

	logger.Info("Starting workspace compliance marking",
		"templateName", templateName,
		"rateLimit", ComplianceMarkingRateLimit,
		"timestamp", startTime)

	// List all workspaces using this template from cache (no pagination)
	// Controllers use cached clients, so all workspaces are already in memory
	workspaces, _, err := workspace.ListByTemplate(ctx, r.Client, templateName, "", "", 0)
	if err != nil {
		return fmt.Errorf("failed to list workspaces using template %s: %w", templateName, err)
	}

	logger.V(1).Info("Listed workspaces from cache",
		"templateName", templateName,
		"workspaceCount", len(workspaces))

	// Mark all workspaces
	for i := range workspaces {
		ws := &workspaces[i]
		if ws.Labels == nil {
			ws.Labels = make(map[string]string)
		}

		// Check if already marked
		if ws.Labels[workspace.LabelComplianceCheckNeeded] == "true" {
			logger.V(1).Info("Workspace already marked for compliance check",
				"workspace", fmt.Sprintf("%s/%s", ws.Namespace, ws.Name))
			marked++
			continue
		}

		// Wait for rate limiter before updating
		if err := rateLimiter.Wait(ctx); err != nil {
			logger.Error(err, "Rate limiter context cancelled")
			// Context cancelled - stop processing but don't fail entire operation
			break
		}

		// Add label
		ws.Labels[workspace.LabelComplianceCheckNeeded] = "true"
		if err := r.Update(ctx, ws); err != nil {
			logger.Error(err, "Failed to mark workspace for compliance check",
				"workspace", fmt.Sprintf("%s/%s", ws.Namespace, ws.Name))
			failed++
			continue
		}

		logger.Info("Marked workspace for compliance check",
			"workspace", fmt.Sprintf("%s/%s", ws.Namespace, ws.Name),
			"timestamp", time.Now())
		marked++
	}

	duration := time.Since(startTime)
	logger.Info("Finished marking workspaces for compliance",
		"templateName", templateName,
		"markedCount", marked,
		"failedCount", failed,
		"duration", duration)

	if failed > 0 {
		return fmt.Errorf("failed to mark %d workspaces for compliance (marked %d successfully)", failed, marked)
	}

	return nil
}

// updateStatusObservedGeneration updates the template's status.observedGeneration field
// Following Kubernetes API conventions for status tracking
func (r *WorkspaceTemplateReconciler) updateStatusObservedGeneration(ctx context.Context, template *workspacev1alpha1.WorkspaceTemplate, generation int64) error {
	logger := logf.FromContext(ctx)

	template.Status.ObservedGeneration = generation

	if err := r.Status().Update(ctx, template); err != nil {
		logger.Error(err, "Failed to update status.observedGeneration",
			"templateName", template.Name,
			"generation", generation)
		return fmt.Errorf("failed to update status: %w", err)
	}

	logger.V(1).Info("Updated status.observedGeneration",
		"templateName", template.Name,
		"generation", generation)

	return nil
}
