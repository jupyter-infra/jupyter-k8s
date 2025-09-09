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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	mngr "sigs.k8s.io/controller-runtime/pkg/manager"

	serversv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// JupyterServerReconciler reconciles a JupyterServer object
type JupyterServerReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	stateMachine *StateMachine
}

// SetStateMachine sets the state machine for testing purposes
func (r *JupyterServerReconciler) SetStateMachine(sm *StateMachine) {
	r.stateMachine = sm
}

// +kubebuilder:rbac:groups=servers.jupyter.org,resources=jupyterservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=servers.jupyter.org,resources=jupyterservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=servers.jupyter.org,resources=jupyterservers/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the JupyterServer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *JupyterServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Starting reconciliation",
		"jupyterserver", req.NamespacedName)

	// Fetch the JupyterServer instance
	jupyterServer, err := r.getJupyterServer(ctx, req)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("JupyterServer not found, assuming deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get JupyterServer")
		return ctrl.Result{}, err
	}

	// Delegate to state machine for business logic
	result, err := r.stateMachine.ReconcileDesiredState(ctx, jupyterServer)
	if err != nil {
		logger.Error(err, "Failed to reconcile desired state")
		return ctrl.Result{}, err
	}

	logger.Info("Reconciliation completed successfully")
	return result, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *JupyterServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&serversv1alpha1.JupyterServer{}).
		Named("jupyterserver").
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

// SetupJupyterServerController sets up the controller with the Manager
func SetupJupyterServerController(mgr mngr.Manager) error {
	ctrl_client := mgr.GetClient()
	scheme := mgr.GetScheme()

	// Create builders
	deploymentBuilder := NewDeploymentBuilder(scheme)
	serviceBuilder := NewServiceBuilder(scheme)

	// Create managers
	statusManager := NewStatusManager(ctrl_client)
	resourceManager := NewResourceManager(ctrl_client, deploymentBuilder, serviceBuilder, statusManager)

	// Create state machine
	stateMachine := NewStateMachine(resourceManager, statusManager)

	// Create reconciler with dependencies
	reconciler := &JupyterServerReconciler{
		Client:       ctrl_client,
		Scheme:       scheme,
		stateMachine: stateMachine,
	}

	return reconciler.SetupWithManager(mgr)
}

// getJupyterServer retrieves the JupyterServer resource
func (r *JupyterServerReconciler) getJupyterServer(ctx context.Context, req ctrl.Request) (*serversv1alpha1.JupyterServer, error) {
	jupyterServer := &serversv1alpha1.JupyterServer{}
	err := r.Get(ctx, req.NamespacedName, jupyterServer)
	return jupyterServer, err
}
