/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
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

	// Manage finalizer based on workspace usage (lazy finalizer pattern)
	// Check if any active workspaces are using this AccessStrategy
	hasFinalizer := controllerutil.ContainsFinalizer(accessStrategy, workspace.AccessStrategyFinalizerName)
	hasWorkspaces, err := workspace.HasActiveWorkspacesWithAccessStrategy(ctx, r.Client, accessStrategy.Name, accessStrategy.Namespace)
	if err != nil {
		logger.Error(err, "Failed to list workspaces using AccessStrategy")
		return ctrl.Result{}, err
	}

	logger.V(1).Info("Checking finalizer state",
		"accessStrategyName", accessStrategy.Name,
		"hasFinalizer", hasFinalizer,
		"hasWorkspaces", hasWorkspaces)

	// Case 1: Workspaces exist, but finalizer is missing → Add finalizer
	if hasWorkspaces && !hasFinalizer {
		logger.Info("Adding finalizer to AccessStrategy (workspaces are using it)",
			"finalizer", workspace.AccessStrategyFinalizerName,
			"hasWorkspaces", hasWorkspaces)

		// Use the safe utility to add finalizer (handles conflicts)
		err = workspace.SafelyAddFinalizerToAccessStrategy(ctx, logger, r.Client, accessStrategy)
		if err != nil {
			logger.Error(err, "Failed to add finalizer to AccessStrategy")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Case 2: No workspaces, but finalizer is present → Remove finalizer
	// This handles the case where all workspaces were deleted
	if !hasWorkspaces && hasFinalizer {
		logger.Info("Removing finalizer from AccessStrategy (no workspaces using it)",
			"finalizer", workspace.AccessStrategyFinalizerName)

		// Use the safe utility to remove finalizer (handles conflicts)
		err := workspace.SafelyRemoveFinalizerFromAccessStrategy(ctx, logger, r.Client, accessStrategy, false)
		if err != nil {
			logger.Error(err, "Failed to remove finalizer from AccessStrategy")
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Case 3: State is correct (both have workspaces+finalizer, or neither)
	logger.V(1).Info("Finalizer state is correct, no action needed")
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
