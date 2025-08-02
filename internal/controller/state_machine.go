package controller

import (
	"context"
	"fmt"

	"github.com/jupyter-k8s/jupyter-k8s/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
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
func (sm *StateMachine) ReconcileDesiredState(ctx context.Context, jupyterServer *v1alpha1.JupyterServer) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	desiredStatus := sm.getDesiredStatus(jupyterServer)
	logger.Info("Reconciling desired state", "desiredStatus", desiredStatus)

	switch desiredStatus {
	case "Stopped":
		return sm.handleStoppedState(ctx, jupyterServer)
	case "Running":
		return sm.handleRunningState(ctx, jupyterServer)
	default:
		return ctrl.Result{}, fmt.Errorf("unknown desired status: %s", desiredStatus)
	}
}

// getDesiredStatus returns the desired status with default fallback
func (sm *StateMachine) getDesiredStatus(jupyterServer *v1alpha1.JupyterServer) string {
	if jupyterServer.Spec.DesiredStatus == "" {
		return DefaultDesiredStatus
	}
	return jupyterServer.Spec.DesiredStatus
}

func (sm *StateMachine) handleStoppedState(ctx context.Context, jupyterServer *v1alpha1.JupyterServer) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling stopped state")

	// Check if deployment exists
	deployment, deploymentErr := sm.resourceManager.GetDeployment(ctx, jupyterServer)
	if deploymentErr != nil && !errors.IsNotFound(deploymentErr) {
		return ctrl.Result{}, fmt.Errorf("failed to get deployment: %w", deploymentErr)
	}

	// Check if service exists
	service, serviceErr := sm.resourceManager.GetService(ctx, jupyterServer)
	if serviceErr != nil && !errors.IsNotFound(serviceErr) {
		return ctrl.Result{}, fmt.Errorf("failed to get service: %w", serviceErr)
	}

	// Delete deployment if it exists
	if deploymentErr == nil && deployment != nil {
		logger.Info("Stopping JupyterServer by deleting Deployment")
		if err := sm.resourceManager.DeleteDeployment(ctx, deployment); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete deployment: %w", err)
		}
	}

	// Delete service if it exists
	if serviceErr == nil && service != nil {
		logger.Info("Stopping JupyterServer by deleting Service")
		if err := sm.resourceManager.DeleteService(ctx, service); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to delete service: %w", err)
		}
	}

	// Update status to stopped
	if err := sm.statusManager.UpdateStoppedStatus(ctx, jupyterServer); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// handleRunningState manages the "Running" state logic
func (sm *StateMachine) handleRunningState(ctx context.Context, jupyterServer *v1alpha1.JupyterServer) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Handling running state")

	// Ensure deployment exists
	deployment, err := sm.resourceManager.EnsureDeploymentExists(ctx, jupyterServer)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure deployment exists: %w", err)
	}

	// Ensure service exists
	service, err := sm.resourceManager.EnsureServiceExists(ctx, jupyterServer)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to ensure service exists: %w", err)
	}

	// Update status based on deployment readiness
	return sm.updateRunningStatus(ctx, jupyterServer, deployment, service)
}

// updateStoppedStatusIfNeeded updates status to stopped if it's not already
func (sm *StateMachine) updateStoppedStatusIfNeeded(ctx context.Context, jupyterServer *v1alpha1.JupyterServer) (ctrl.Result, error) {
	if jupyterServer.Status.Phase != PhaseStopped {
		if err := sm.statusManager.UpdateStoppedStatus(ctx, jupyterServer); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// updateRunningStatus updates the status based on deployment readiness
func (sm *StateMachine) updateRunningStatus(ctx context.Context, jupyterServer *v1alpha1.JupyterServer, deployment interface{}, service interface{}) (ctrl.Result, error) {
	// If resources were just created, update to Creating status
	if jupyterServer.Status.Phase == "" || jupyterServer.Status.Phase == PhaseStopped {
		deploymentName := GenerateDeploymentName(jupyterServer.Name)
		serviceName := GenerateServiceName(jupyterServer.Name)

		if err := sm.statusManager.UpdateCreatingStatus(ctx, jupyterServer, deploymentName, serviceName); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// Check if we should update to Running status
	if deploymentObj, ok := deployment.(*appsv1.Deployment); ok {
		if sm.statusManager.ShouldUpdateToRunning(jupyterServer, deploymentObj) {
			if err := sm.statusManager.UpdateRunningStatus(ctx, jupyterServer); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}
