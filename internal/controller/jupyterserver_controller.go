package controller

import (
	"context"

	"github.com/jupyter-k8s/jupyter-k8s/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
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

// SetupWithManager sets up the controller with the Manager.
func (r *JupyterServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.JupyterServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

// SetupJupyterServerController sets up the controller with the Manager
func SetupJupyterServerController(mgr manager.Manager) error {
	client := mgr.GetClient()
	scheme := mgr.GetScheme()

	// Create builders
	deploymentBuilder := NewDeploymentBuilder(scheme)
	serviceBuilder := NewServiceBuilder(scheme)

	// Create managers
	statusManager := NewStatusManager(client)
	resourceManager := NewResourceManager(client, deploymentBuilder, serviceBuilder, statusManager)

	// Create state machine
	stateMachine := NewStateMachine(resourceManager, statusManager)

	// Create reconciler with dependencies
	reconciler := &JupyterServerReconciler{
		Client:       client,
		Scheme:       scheme,
		stateMachine: stateMachine,
	}

	return reconciler.SetupWithManager(mgr)
}

// +kubebuilder:rbac:groups=servers.jupyter.org,resources=jupyterservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=servers.jupyter.org,resources=jupyterservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=servers.jupyter.org,resources=jupyterservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *JupyterServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
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

// getJupyterServer retrieves the JupyterServer resource
func (r *JupyterServerReconciler) getJupyterServer(ctx context.Context, req ctrl.Request) (*v1alpha1.JupyterServer, error) {
	jupyterServer := &v1alpha1.JupyterServer{}
	err := r.Get(ctx, req.NamespacedName, jupyterServer)
	return jupyterServer, err
}
