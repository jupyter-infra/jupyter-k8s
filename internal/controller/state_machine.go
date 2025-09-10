package controller

import (
	"context"
	"fmt"

	serversv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// StateMachine handles the state transitions for JupyterServer
type StateMachine struct {
	resourceManager *ResourceManager
	statusManager   *StatusManager
}

// NewStateMachine creates a new StateMachine
func NewStateMachine(resourceManager *ResourceManager, statusManager *StatusManager) *StateMachine {
	return &StateMachine{
		resourceManager: resourceManager,
		statusManager:   statusManager,
	}
}

// ReconcileDesiredState handles the state machine logic for JupyterServer
func (sm *StateMachine) ReconcileDesiredState(ctx context.Context, jupyterServer *serversv1alpha1.JupyterServer) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	desiredStatus := sm.getDesiredStatus(jupyterServer)
	logger.Info("Reconciling desired state", "desiredStatus", desiredStatus)

	switch desiredStatus {
	case "Stopped":
		return sm.reconcileDesiredStoppedStatus(ctx, jupyterServer)
	case "Running":
		return sm.reconcileDesiredRunningStatus(ctx, jupyterServer)
	default:
		err := fmt.Errorf("unknown desired status: %s", desiredStatus)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, jupyterServer, ReasonDeploymentError, err.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: LongRequeueDelay}, err
	}
}

// getDesiredStatus returns the desired status with default fallback
func (sm *StateMachine) getDesiredStatus(jupyterServer *serversv1alpha1.JupyterServer) string {
	if jupyterServer.Spec.DesiredStatus == "" {
		return DefaultDesiredStatus
	}
	return jupyterServer.Spec.DesiredStatus
}

func (sm *StateMachine) reconcileDesiredStoppedStatus(ctx context.Context, jupyterServer *serversv1alpha1.JupyterServer) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Attempting to bring JupyterServer status to 'Stopped'")

	// Ensure deployment is deleted - this is an asynchronous operation
	// EnsureDeploymentDeleted only ensures the delete API request is accepted by K8s
	// It does not wait for the deployment to be fully removed
	deployment, deploymentErr := sm.resourceManager.EnsureDeploymentDeleted(ctx, jupyterServer)
	if deploymentErr != nil {
		err := fmt.Errorf("failed to get deployment: %w", deploymentErr)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, jupyterServer, ReasonDeploymentError, err.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Ensure service is deleted - this is an asynchronous operation
	// EnsureServiceDeleted only ensures the delete API request is accepted by K8s
	// It does not wait for the service to be fully removed
	service, serviceErr := sm.resourceManager.EnsureServiceDeleted(ctx, jupyterServer)
	if serviceErr != nil {
		err := fmt.Errorf("failed to get service: %w", serviceErr)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, jupyterServer, ReasonServiceError, err.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Check if resources are fully deleted (asynchronous deletion check)
	// A nil resource means the resource has been fully deleted
	deploymentDeleted := sm.resourceManager.IsDeploymentMissingOrDeleting(deployment)
	serviceDeleted := sm.resourceManager.IsServiceMissingOrDeleting(service)

	if deploymentDeleted && serviceDeleted {
		// Both resources are fully deleted, update to stopped status
		logger.Info("Deployment and Service are both deleted, updating to Stopped status")
		if err := sm.statusManager.UpdateStoppedStatus(ctx, jupyterServer); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// If EITHER deployment OR service is still in the process of being deleted
	// Update status to Stopping and requeue to check again later
	if deploymentDeleted || serviceDeleted {
		logger.Info("Resources still being deleted", "deploymentDeleted", deploymentDeleted, "serviceDeleted", serviceDeleted)
		if err := sm.statusManager.UpdateStoppingStatus(ctx, jupyterServer, deploymentDeleted, serviceDeleted); err != nil {
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}
		// Requeue to check deletion progress again later
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
	}

	// This should not happen, return an error
	err := fmt.Errorf("unexpected state: both deployment and service should be in deletion process")
	// Update error condition
	if statusErr := sm.statusManager.UpdateErrorStatus(ctx, jupyterServer, ReasonDeploymentError, err.Error()); statusErr != nil {
		logger.Error(statusErr, "Failed to update error status")
	}
	return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
}

func (sm *StateMachine) reconcileDesiredRunningStatus(ctx context.Context, jupyterServer *serversv1alpha1.JupyterServer) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Attempting to bring JupyterServer status to 'Running'")

	// Ensure deployment exists - this is an asynchronous operation
	// EnsureDeploymentExists only ensures the API request is accepted by K8s
	// It does not wait for the deployment to be fully ready
	deployment, err := sm.resourceManager.EnsureDeploymentExists(ctx, jupyterServer)
	if err != nil {
		deployErr := fmt.Errorf("failed to ensure deployment exists: %w", err)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, jupyterServer, ReasonDeploymentError, deployErr.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, deployErr
	}

	// Ensure service exists - this is an asynchronous operation
	// EnsureServiceExists only ensures the API request is accepted by K8s
	// It does not wait for the service to be fully ready
	service, err := sm.resourceManager.EnsureServiceExists(ctx, jupyterServer)
	if err != nil {
		serviceErr := fmt.Errorf("failed to ensure service exists: %w", err)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, jupyterServer, ReasonServiceError, serviceErr.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, serviceErr
	}

	// Check if resources are fully ready (asynchronous readiness check)
	// For deployments, we check the Available condition and/or replica counts
	// For services, we just check if the Service object exists
	deploymentReady := sm.resourceManager.IsDeploymentAvailable(deployment)
	serviceReady := sm.resourceManager.IsServiceAvailable(service)

	if deploymentReady && serviceReady {
		// Both resources are ready, update to running status
		logger.Info("Deployment and Service are both ready, updating to Running status")
		if err := sm.statusManager.UpdateRunningStatus(ctx, jupyterServer); err != nil {
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}
		return ctrl.Result{}, nil
	}

	// Resources are being created/started but not fully ready yet
	// Update status to Starting and requeue to check again later
	logger.Info("Resources not fully ready", "deploymentReady", deploymentReady, "serviceReady", serviceReady)
	if err := sm.statusManager.UpdateStartingStatus(
		ctx, jupyterServer, deploymentReady, serviceReady, deployment.GetName(), service.GetName()); err != nil {
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Requeue to check resource readiness again later
	return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
}
