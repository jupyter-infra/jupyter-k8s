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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

const templateFinalizerName = "workspaces.jupyter.org/template-protection"

// WorkspaceTemplateReconciler reconciles a WorkspaceTemplate object
type WorkspaceTemplateReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	recorder         record.EventRecorder
	templateResolver *TemplateResolver
}

// +kubebuilder:rbac:groups=workspaces.jupyter.org,resources=workspacetemplates,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=workspaces.jupyter.org,resources=workspacetemplates/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *WorkspaceTemplateReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling WorkspaceTemplate", "template", req.Name)

	template := &workspacesv1alpha1.WorkspaceTemplate{}
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
	return r.manageFinalizer(ctx, template)
}

// manageFinalizer implements lazy finalizer management for WorkspaceTemplates
// Following Kubernetes best practices:
// - Finalizers are only added when workspaces start using the template
// - Finalizers are removed when all workspaces stop using the template
// - This minimizes overhead and complexity compared to adding finalizers to all templates
func (r *WorkspaceTemplateReconciler) manageFinalizer(ctx context.Context, template *workspacesv1alpha1.WorkspaceTemplate) (ctrl.Result, error) {
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

func (r *WorkspaceTemplateReconciler) handleDeletion(ctx context.Context, template *workspacesv1alpha1.WorkspaceTemplate) (ctrl.Result, error) {
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
		return ctrl.Result{}, fmt.Errorf("template %s is in use by %d workspaces", template.Name, len(workspaces))
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
		For(&workspacesv1alpha1.WorkspaceTemplate{}).
		Watches(
			&workspacesv1alpha1.Workspace{},
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
	workspace, ok := obj.(*workspacesv1alpha1.Workspace)
	if !ok {
		return nil
	}

	if workspace.Spec.TemplateRef == nil || *workspace.Spec.TemplateRef == "" {
		return nil
	}

	logger := logf.FromContext(ctx)
	logger.V(1).Info("Workspace changed, enqueueing template reconciliation",
		"workspace", fmt.Sprintf("%s/%s", workspace.Namespace, workspace.Name),
		"template", *workspace.Spec.TemplateRef)

	// Trigger reconciliation of the template when workspace changes
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Name: *workspace.Spec.TemplateRef,
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
