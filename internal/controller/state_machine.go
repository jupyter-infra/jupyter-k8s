package controller

import (
	"context"
	"fmt"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// StateMachine handles the state transitions for Workspace
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

// ReconcileDesiredState handles the state machine logic for Workspace
func (sm *StateMachine) ReconcileDesiredState(ctx context.Context, workspace *workspacesv1alpha1.Workspace) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	desiredStatus := sm.getDesiredStatus(workspace)
	logger.Info("Reconciling desired state", "desiredStatus", desiredStatus)

	switch desiredStatus {
	case "Stopped":
		return sm.reconcileDesiredStoppedStatus(ctx, workspace)
	case "Running":
		return sm.reconcileDesiredRunningStatus(ctx, workspace)
	default:
		err := fmt.Errorf("unknown desired status: %s", desiredStatus)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, workspace, ReasonDeploymentError, err.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: LongRequeueDelay}, err
	}
}

// getDesiredStatus returns the desired status with default fallback
func (sm *StateMachine) getDesiredStatus(workspace *workspacesv1alpha1.Workspace) string {
	if workspace.Spec.DesiredStatus == "" {
		return DefaultDesiredStatus
	}
	return workspace.Spec.DesiredStatus
}

func (sm *StateMachine) reconcileDesiredStoppedStatus(ctx context.Context, workspace *workspacesv1alpha1.Workspace) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Attempting to bring Workspace status to 'Stopped'")

	// Remove access strategy resources first
	accessError := sm.ReconcileAccessForDesiredStoppedStatus(ctx, workspace)
	if accessError != nil {
		logger.Error(accessError, "Failed to remove access strategy resources")
		// Continue with deletion of other resources, don't block on access strategy
	}

	// Ensure deployment is deleted - this is an asynchronous operation
	// EnsureDeploymentDeleted only ensures the delete API request is accepted by K8s
	// It does not wait for the deployment to be fully removed
	deployment, deploymentErr := sm.resourceManager.EnsureDeploymentDeleted(ctx, workspace)
	if deploymentErr != nil {
		err := fmt.Errorf("failed to get deployment: %w", deploymentErr)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, workspace, ReasonDeploymentError, err.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Ensure service is deleted - this is an asynchronous operation
	// EnsureServiceDeleted only ensures the delete API request is accepted by K8s
	// It does not wait for the service to be fully removed
	service, serviceErr := sm.resourceManager.EnsureServiceDeleted(ctx, workspace)
	if serviceErr != nil {
		err := fmt.Errorf("failed to get service: %w", serviceErr)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, workspace, ReasonServiceError, err.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Check if resources are fully deleted (asynchronous deletion check)
	// A nil resource means the resource has been fully deleted
	deploymentDeleted := sm.resourceManager.IsDeploymentMissingOrDeleting(deployment)
	serviceDeleted := sm.resourceManager.IsServiceMissingOrDeleting(service)

	if deploymentDeleted && serviceDeleted {
		// Flag as Error if AccessResources failed to delete
		if accessError != nil {
			if statusErr := sm.statusManager.UpdateErrorStatus(ctx, workspace, ReasonServiceError, accessError.Error()); statusErr != nil {
				logger.Error(statusErr, "Failed to update error status")
			}
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, accessError
		} else {
			// All resources are fully deleted, update to stopped status
			logger.Info("Deployment and Service are both deleted, updating to Stopped status")
			if err := sm.statusManager.UpdateStoppedStatus(ctx, workspace); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	// If EITHER deployment OR service is still in the process of being deleted
	// Update status to Stopping and requeue to check again later
	if deploymentDeleted || serviceDeleted {
		logger.Info("Resources still being deleted", "deploymentDeleted", deploymentDeleted, "serviceDeleted", serviceDeleted)
		readiness := WorkspaceStoppingReadiness{
			computeStopped:         deploymentDeleted,
			serviceStopped:         serviceDeleted,
			accessResourcesStopped: true,
		}
		if err := sm.statusManager.UpdateStoppingStatus(ctx, workspace, readiness); err != nil {
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}
		// Requeue to check deletion progress again later
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
	}

	// This should not happen, return an error
	err := fmt.Errorf("unexpected state: both deployment and service should be in deletion process")
	// Update error condition
	if statusErr := sm.statusManager.UpdateErrorStatus(ctx, workspace, ReasonDeploymentError, err.Error()); statusErr != nil {
		logger.Error(statusErr, "Failed to update error status")
	}
	return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
}

func (sm *StateMachine) reconcileDesiredRunningStatus(ctx context.Context, workspace *workspacesv1alpha1.Workspace) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Attempting to bring Workspace status to 'Running'")

	// Ensure PVC exists first (if storage is configured)
	_, err := sm.resourceManager.EnsurePVCExists(ctx, workspace)
	if err != nil {
		pvcErr := fmt.Errorf("failed to ensure PVC exists: %w", err)
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, workspace, ReasonDeploymentError, pvcErr.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, pvcErr
	}

	// Ensure deployment exists
	// EnsureDeploymentExists only ensures the API request is accepted by K8s
	// It does not wait for the deployment to be fully ready
	deployment, err := sm.resourceManager.EnsureDeploymentExists(ctx, workspace)
	if err != nil {
		deployErr := fmt.Errorf("failed to ensure deployment exists: %w", err)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, workspace, ReasonDeploymentError, deployErr.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, deployErr
	}

	// Ensure service exists - this is an asynchronous operation
	// EnsureServiceExists only ensures the API request is accepted by K8s
	// It does not wait for the service to be fully ready
	service, err := sm.resourceManager.EnsureServiceExists(ctx, workspace)
	if err != nil {
		serviceErr := fmt.Errorf("failed to ensure service exists: %w", err)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, workspace, ReasonServiceError, serviceErr.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, serviceErr
	}

	// Check if resources are fully ready (asynchronous readiness check)
	// For deployments, we check the Available condition and/or replica counts
	// For services, we just check if the Service object exists
	deploymentReady := sm.resourceManager.IsDeploymentAvailable(deployment)
	serviceReady := sm.resourceManager.IsServiceAvailable(service)

	// Apply access strategy when compute and service resources are ready
	if deploymentReady && serviceReady {
		if err := sm.ReconcileAccessForDesiredRunningStatus(ctx, workspace, service); err != nil {
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}

		// Then only update to running status
		logger.Info("Deployment and Service are both ready, updating to Running status")
		if err := sm.statusManager.UpdateRunningStatus(ctx, workspace); err != nil {
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}
		return ctrl.Result{}, nil
	}

	// Resources are being created/started but not fully ready yet
	// Update status to Starting and requeue to check again later
	logger.Info("Resources not fully ready", "deploymentReady", deploymentReady, "serviceReady", serviceReady)
	workspace.Status.DeploymentName = deployment.GetName()
	workspace.Status.ServiceName = service.GetName()
	readiness := WorkspaceRunningReadiness{
		computeReady:         deploymentReady,
		serviceReady:         serviceReady,
		accessResourcesReady: false,
	}
	if err := sm.statusManager.UpdateStartingStatus(ctx, workspace, readiness); err != nil {
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Requeue to check resource readiness again later
	return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
}
