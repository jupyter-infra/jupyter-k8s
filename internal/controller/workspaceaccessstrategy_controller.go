/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"

	"github.com/go-logr/logr"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/retry"
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

	// Manage the two protection finalizers independently (lazy finalizer pattern). Workspace and
	// template references each get their own finalizer, so each is added/removed purely on its own
	// signal and Kubernetes keeps the AccessStrategy alive until BOTH referrer types release it.
	hasWorkspaces, err := workspace.HasActiveWorkspacesWithAccessStrategy(ctx, r.Client, accessStrategy.Name, accessStrategy.Namespace)
	if err != nil {
		logger.Error(err, "Failed to list workspaces using AccessStrategy")
		return ctrl.Result{}, err
	}

	hasTemplates, err := workspace.HasActiveTemplatesWithAccessStrategy(ctx, r.Client, accessStrategy.Name, accessStrategy.Namespace)
	if err != nil {
		logger.Error(err, "Failed to list templates using AccessStrategy")
		return ctrl.Result{}, err
	}

	logger.V(1).Info("Checking finalizer state",
		"accessStrategyName", accessStrategy.Name,
		"hasWorkspaces", hasWorkspaces,
		"hasTemplates", hasTemplates)

	// Reconcile both protection finalizers in a single update: the workspace-protection finalizer against
	// workspace references and the template-protection finalizer against template references. Folding them
	// into one Update converges in a single reconcile pass and avoids a second API round-trip (and the
	// extra conflict window) that separate per-finalizer updates would incur.
	if changed, err := r.reconcileFinalizers(ctx, logger, accessStrategy, hasWorkspaces, hasTemplates); err != nil {
		return ctrl.Result{}, err
	} else if changed {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Finalizer state is correct, no action needed")
	return ctrl.Result{}, nil
}

// reconcileFinalizers brings both protection finalizers into agreement with the current reference state
// in a single Update. The workspace-protection finalizer tracks workspace references and the
// template-protection finalizer tracks template references; each is added when its referrer type
// references the AccessStrategy and removed when it does not. It returns true when it changed (and
// persisted) the finalizer set, so the caller can requeue via the update event.
//
// On a conflict the object is re-fetched and the desired finalizer set is recomputed and reapplied, so a
// concurrent writer cannot make us clobber an unrelated change.
func (r *WorkspaceAccessStrategyReconciler) reconcileFinalizers(
	ctx context.Context,
	logger logr.Logger,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	hasWorkspaces bool,
	hasTemplates bool) (bool, error) {

	// applyDesired mutates the AccessStrategy in place to match the reference state and reports whether
	// anything changed. controllerutil.{Add,Remove}Finalizer are no-ops when already in the desired state.
	applyDesired := func(as *workspacev1alpha1.WorkspaceAccessStrategy) bool {
		changed := false
		changed = setFinalizer(as, workspace.AccessStrategyFinalizerName, hasWorkspaces) || changed
		changed = setFinalizer(as, workspace.AccessStrategyTemplateFinalizerName, hasTemplates) || changed
		return changed
	}

	if !applyDesired(accessStrategy) {
		return false, nil
	}

	logger.Info("Updating AccessStrategy protection finalizers",
		"hasWorkspaces", hasWorkspaces, "hasTemplates", hasTemplates,
		"finalizers", accessStrategy.Finalizers)

	if err := r.Update(ctx, accessStrategy); err != nil {
		if !errors.IsConflict(err) {
			logger.Error(err, "Failed to update AccessStrategy finalizers")
			return false, err
		}
		// Conflict: re-fetch the latest version, recompute the desired finalizer set on it, and retry.
		logger.V(1).Info("Conflict updating AccessStrategy finalizers, retrying on latest version")
		retryErr := retry.RetryOnConflict(retry.DefaultRetry, func() error {
			latest := &workspacev1alpha1.WorkspaceAccessStrategy{}
			if getErr := r.Get(ctx, types.NamespacedName{
				Name:      accessStrategy.Name,
				Namespace: accessStrategy.Namespace,
			}, latest); getErr != nil {
				return getErr
			}
			if !applyDesired(latest) {
				return nil
			}
			return r.Update(ctx, latest)
		})
		if retryErr != nil {
			logger.Error(retryErr, "Failed to update AccessStrategy finalizers after retry")
			return false, retryErr
		}
	}

	return true, nil
}

// setFinalizer adds or removes the named finalizer to match present, returning whether the finalizer set
// changed.
func setFinalizer(as *workspacev1alpha1.WorkspaceAccessStrategy, finalizerName string, present bool) bool {
	if present {
		return controllerutil.AddFinalizer(as, finalizerName)
	}
	return controllerutil.RemoveFinalizer(as, finalizerName)
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
		Watches(
			&workspacev1alpha1.WorkspaceTemplate{},
			handler.EnqueueRequestsFromMapFunc(r.findAccessStrategiesForTemplate),
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

// findAccessStrategiesForTemplate maps a WorkspaceTemplate to the WorkspaceAccessStrategy it references
// via spec.defaultAccessStrategy, so the AccessStrategy is reconciled (finalizer added/removed) when a
// template starts or stops referencing it.
//
// Uses the label rather than spec.defaultAccessStrategy: on an Update event the map func is called for
// both the old and new objects, and on a dereference (or deletion) the spec is cleared while the OLD
// object's label still points at the AccessStrategy - that mapping is what drives finalizer removal.
func (r *WorkspaceAccessStrategyReconciler) findAccessStrategiesForTemplate(ctx context.Context, obj client.Object) []reconcile.Request {
	template, ok := obj.(*workspacev1alpha1.WorkspaceTemplate)
	if !ok {
		return nil
	}

	logger := logf.FromContext(ctx)

	accessStrategyName := template.Labels[workspace.LabelAccessStrategyName]
	accessStrategyNamespace := template.Labels[workspace.LabelAccessStrategyNamespace]
	if accessStrategyName == "" || accessStrategyNamespace == "" {
		logger.V(1).Info("Template has no AccessStrategy labels, skipping AccessStrategy reconciliation",
			"template", template.Name,
			"templateNamespace", template.Namespace)
		return nil
	}

	logger.Info("Template changed, enqueueing AccessStrategy reconciliation",
		"template", template.Name,
		"templateNamespace", template.Namespace,
		"accessStrategy", accessStrategyName,
		"accessStrategyNamespace", accessStrategyNamespace,
		"deletionTimestamp", template.DeletionTimestamp)

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
