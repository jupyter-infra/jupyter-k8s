/*
MIT License

Copyright (c) 2025 Amazon Web Services

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

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	webhookconst "github.com/jupyter-ai-contrib/jupyter-k8s/internal/webhook"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
	corev1 "k8s.io/api/core/v1"
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
)

// Intentionally no logger defined here, using logf.FromContext in Reconcile

// WorkspaceAccessStrategyReconciler reconciles a WorkspaceAccessStrategy object
type WorkspaceAccessStrategyReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder
}

// +kubebuilder:rbac:groups=workspace.jupyter.org,resources=workspaceaccessstrategies/finalizers,verbs=update

// Reconcile handles WorkspaceAccessStrategy finalizer logic to prevent deletion
// when the AccessStrategy is still referenced by Workspaces.
func (r *WorkspaceAccessStrategyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx).WithValues(
		"workspaceaccessstrategy", req.Name,
		"namespace", req.Namespace)

	// Get the AccessStrategy
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{}
	if err := r.Get(ctx, req.NamespacedName, accessStrategy); err != nil {
		if errors.IsNotFound(err) {
			// Already deleted, nothing to do
			logger.V(1).Info("WorkspaceAccessStrategy not found, it may have been deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get WorkspaceAccessStrategy")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if !accessStrategy.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, accessStrategy)
	}

	// Manage finalizer based on workspace usage (lazy finalizer pattern)
	// Check if any active workspaces are using this AccessStrategy
	workspaces, _, err := workspace.ListActiveWorkspacesByAccessStrategy(ctx, r.Client, accessStrategy.Name, accessStrategy.Namespace, "", 0)
	if err != nil {
		logger.Error(err, "Failed to list workspaces using AccessStrategy")
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	hasFinalizer := controllerutil.ContainsFinalizer(accessStrategy, webhookconst.AccessStrategyFinalizerName)
	hasWorkspaces := len(workspaces) > 0

	logger.V(1).Info("Checking finalizer state",
		"accessStrategyName", accessStrategy.Name,
		"hasFinalizer", hasFinalizer,
		"workspaceCount", len(workspaces))

	// Case 1: Workspaces exist, but finalizer is missing → Add finalizer
	if hasWorkspaces && !hasFinalizer {
		logger.Info("Adding finalizer to AccessStrategy (workspaces are using it)",
			"finalizer", webhookconst.AccessStrategyFinalizerName,
			"workspaceCount", len(workspaces))

		// Use the safe utility to add finalizer (handles conflicts)
		err = workspace.SafelyAddFinalizerToAccessStrategy(ctx, logger, r.Client, accessStrategy)
		if err != nil {
			logger.Error(err, "Failed to add finalizer to AccessStrategy")
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}
		return ctrl.Result{}, nil
	}

	// Case 2: No workspaces, but finalizer is present → Remove finalizer
	// This handles the case where all workspaces were deleted
	if !hasWorkspaces && hasFinalizer {
		logger.Info("Removing finalizer from AccessStrategy (no workspaces using it)",
			"finalizer", webhookconst.AccessStrategyFinalizerName)

		// Use the safe utility to remove finalizer (handles conflicts)
		err := workspace.SafelyRemoveFinalizerFromAccessStrategy(ctx, logger, r.Client, accessStrategy, false)
		if err != nil {
			logger.Error(err, "Failed to remove finalizer from AccessStrategy")
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}
		return ctrl.Result{}, nil
	}

	// Case 3: State is correct (both have workspaces+finalizer, or neither)
	logger.V(1).Info("Finalizer state is correct, no action needed")
	return ctrl.Result{}, nil
}

// handleDeletion handles deletion of AccessStrategy and manages finalizers
// based on whether any workspaces are still using the AccessStrategy
func (r *WorkspaceAccessStrategyReconciler) handleDeletion(ctx context.Context, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Handling AccessStrategy deletion", "accessStrategyName", accessStrategy.Name)

	if !controllerutil.ContainsFinalizer(accessStrategy, webhookconst.AccessStrategyFinalizerName) {
		logger.Info("No finalizer present, allowing deletion", "accessStrategyName", accessStrategy.Name)
		return ctrl.Result{}, nil
	}

	// Check if any workspaces are using this AccessStrategy
	// Uses efficient label-based lookup via the cache (not direct API calls)
	workspaces, _, err := workspace.ListActiveWorkspacesByAccessStrategy(ctx, r.Client, accessStrategy.Name, accessStrategy.Namespace, "", 0)
	if err != nil {
		logger.Error(err, "Failed to list workspaces using AccessStrategy")
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	if len(workspaces) > 0 {
		logger.Info("AccessStrategy is in use, blocking deletion",
			"accessStrategyName", accessStrategy.Name,
			"exampleWorkspace", workspaces[0].Name,
			"exampleWorkspaceNamespace", workspaces[0].Namespace,
			"totalWorkspaces", len(workspaces))

		// Record an event to notify users
		msg := "Cannot delete AccessStrategy: in use by workspace(s)"
		r.EventRecorder.Event(accessStrategy, corev1.EventTypeWarning, "AccessStrategyInUse", msg)

		// Don't remove finalizer - block deletion
		// Return nil error - we successfully determined AccessStrategy is in use
		// Will be reconciled again when workspace changes
		return ctrl.Result{RequeueAfter: LongRequeueDelay}, nil
	}

	// No workspaces using AccessStrategy - safe to delete
	logger.Info("No workspaces using AccessStrategy, removing finalizer",
		"accessStrategyName", accessStrategy.Name)

	// Use the safe utility to remove finalizer (handles conflicts)
	// Set deletedOk to true since we're in the deletion handler and
	// it's fine if the resource is already gone
	err = workspace.SafelyRemoveFinalizerFromAccessStrategy(ctx, logger, r.Client, accessStrategy, true)
	if err != nil {
		logger.Error(err, "Failed to remove finalizer from AccessStrategy",
			"accessStrategyName", accessStrategy.Name)
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
// It configures watches for WorkspaceAccessStrategy resources and triggers reconciliation
// when Workspaces change to manage finalizers based on AccessStrategy usage.
func (r *WorkspaceAccessStrategyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	logger := mgr.GetLogger().WithName("workspaceaccessstrategy-setup")
	logger.Info("Setting up WorkspaceAccessStrategy controller")

	err := ctrl.NewControllerManagedBy(mgr).
		For(&workspacev1alpha1.WorkspaceAccessStrategy{}).
		Watches(
			&workspacev1alpha1.Workspace{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessStrategiesForWorkspace),
		).
		Named("workspaceaccessstrategy").
		Complete(r)

	if err != nil {
		logger.Error(err, "Failed to setup WorkspaceAccessStrategy controller")
		return err
	}

	logger.Info("Successfully registered WorkspaceAccessStrategy controller with manager")
	return nil
}

// findAccessStrategiesForWorkspace maps a Workspace to the WorkspaceAccessStrategy it references
// This ensures the AccessStrategy is reconciled when a workspace using it is created/updated/deleted
func (r *WorkspaceAccessStrategyReconciler) findAccessStrategiesForWorkspace(ctx context.Context, obj client.Object) []reconcile.Request {
	ws, ok := obj.(*workspacev1alpha1.Workspace)
	if !ok {
		return nil
	}

	logger := logf.FromContext(ctx)

	// Use label instead of spec.accessStrategy because labels persist during deletion
	// When workspace is deleted, spec fields are cleared but labels remain
	accessStrategyName := ws.Labels[workspace.LabelAccessStrategyName]
	accessStrategyNamespace := ws.Labels[workspace.LabelAccessStrategyNamespace]
	if accessStrategyName == "" || accessStrategyNamespace == "" {
		logger.V(1).Info("Workspace has incomplete AccessStrategy labels, skipping AccessStrategy reconciliation",
			"workspace", ws.Name,
			"workspaceNamespace", ws.Namespace,
			"accessStrategyName", accessStrategyName,
			"accessStrategyNamespace", accessStrategyNamespace,
			"deletionTimestamp", ws.DeletionTimestamp)
		return nil
	}

	logger.Info("Workspace changed, enqueueing AccessStrategy reconciliation",
		"workspace", ws.Name,
		"workspaceNamespace", ws.Namespace,
		"accessStrategy", accessStrategyName,
		"accessStrategyNamespace", accessStrategyNamespace,
		"deletionTimestamp", ws.DeletionTimestamp,
		"hasLabel", true)

	// Trigger reconciliation of the AccessStrategy when workspace changes
	return []reconcile.Request{
		{NamespacedName: types.NamespacedName{
			Name:      accessStrategyName,
			Namespace: accessStrategyNamespace,
		}},
	}
}

// SetupWorkspaceAccessStrategyController sets up the controller with the Manager.
func SetupWorkspaceAccessStrategyController(mgr ctrl.Manager) error {
	k8sClient := mgr.GetClient()
	scheme := mgr.GetScheme()
	eventRecorder := mgr.GetEventRecorderFor("workspaceaccessstrategy-controller")

	reconciler := &WorkspaceAccessStrategyReconciler{
		Client:        k8sClient,
		Scheme:        scheme,
		EventRecorder: eventRecorder,
	}

	return reconciler.SetupWithManager(mgr)
}
