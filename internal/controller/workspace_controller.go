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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	mngr "sigs.k8s.io/controller-runtime/pkg/manager"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// WorkspaceControllerOptions contains configuration options for the Workspace controller
type WorkspaceControllerOptions struct {
	// ApplicationImagesPullPolicy defines how application container images should be pulled
	ApplicationImagesPullPolicy corev1.PullPolicy

	// Registry is the prefix to use for all application images
	ApplicationImagesRegistry string

	// RequireTemplate enforces that all workspaces must reference a template
	RequireTemplate bool
}

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	stateMachine  *StateMachine
	statusManager *StatusManager
	recorder      record.EventRecorder
	options       WorkspaceControllerOptions
}

// SetStateMachine sets the state machine for testing purposes
func (r *WorkspaceReconciler) SetStateMachine(sm *StateMachine) {
	r.stateMachine = sm
}

// +kubebuilder:rbac:groups=workspaces.jupyter.org,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=workspaces.jupyter.org,resources=workspaces/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=workspaces.jupyter.org,resources=workspacetemplates,verbs=get;list;watch
// +kubebuilder:rbac:groups=workspaces.jupyter.org,resources=workspacetemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Workspace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Starting reconciliation",
		"workspace", req.NamespacedName)

	// Fetch the Workspace instance
	workspace, err := r.getWorkspace(ctx, req)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Workspace not found, assuming deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get Workspace")
		return ctrl.Result{}, err
	}

	// Enforce template requirement if configured
	if r.options.RequireTemplate && workspace.Spec.TemplateRef == nil {
		logger.Info("Workspace rejected: template reference required")

		// Record rejection event
		r.recorder.Event(workspace, corev1.EventTypeWarning, "TemplateRequired",
			"Workspace creation rejected: template reference is required by policy")

		// Update status to reflect rejection
		if err := r.statusManager.SetTemplateRequired(ctx, workspace); err != nil {
			logger.Error(err, "Failed to update rejection status")
			return ctrl.Result{}, err
		}

		// Don't requeue - policy violation won't auto-resolve
		return ctrl.Result{}, nil
	}

	// Delegate to state machine for business logic
	result, err := r.stateMachine.ReconcileDesiredState(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to reconcile desired state")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciliation completed successfully")
	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&workspacesv1alpha1.Workspace{}).
		Named("workspace").
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}

// SetupWorkspaceController sets up the controller with the Manager and specified options
func SetupWorkspaceController(mgr mngr.Manager, options WorkspaceControllerOptions) error {
	k8sClient := mgr.GetClient()
	scheme := mgr.GetScheme()

	// Create builders with options
	deploymentBuilder := NewDeploymentBuilder(scheme, options, k8sClient)
	serviceBuilder := NewServiceBuilder(scheme)
	pvcBuilder := NewPVCBuilder(scheme)

	// Create template resolver
	templateResolver := NewTemplateResolver(k8sClient)

	// Create managers
	statusManager := NewStatusManager(k8sClient)
	resourceManager := NewResourceManager(k8sClient, deploymentBuilder, serviceBuilder, pvcBuilder, statusManager)

	// Create event recorder
	eventRecorder := mgr.GetEventRecorderFor("workspace-controller")

	// Create state machine with template resolver and event recorder
	stateMachine := NewStateMachine(resourceManager, statusManager, templateResolver, eventRecorder)

	// Create reconciler with dependencies
	reconciler := &WorkspaceReconciler{
		Client:        k8sClient,
		Scheme:        scheme,
		stateMachine:  stateMachine,
		statusManager: statusManager,
		recorder:      eventRecorder, // Use the same instance
		options:       options,
	}

	return reconciler.SetupWithManager(mgr)
}

// getWorkspace retrieves the Workspace resource
func (r *WorkspaceReconciler) getWorkspace(ctx context.Context, req ctrl.Request) (*workspacesv1alpha1.Workspace, error) {
	workspace := &workspacesv1alpha1.Workspace{}
	err := r.Get(ctx, req.NamespacedName, workspace)
	return workspace, err
}
