package controller

import (
	"context"
	"fmt"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// StateMachine handles the state transitions for Workspace
type StateMachine struct {
	resourceManager  *ResourceManager
	statusManager    *StatusManager
	templateResolver *TemplateResolver
	recorder         record.EventRecorder
}

// NewStateMachine creates a new StateMachine
func NewStateMachine(resourceManager *ResourceManager, statusManager *StatusManager, templateResolver *TemplateResolver, recorder record.EventRecorder) *StateMachine {
	return &StateMachine{
		resourceManager:  resourceManager,
		statusManager:    statusManager,
		templateResolver: templateResolver,
		recorder:         recorder,
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
		// Both resources are fully deleted, update to stopped status
		logger.Info("Deployment and Service are both deleted, updating to Stopped status")

		// Record workspace stopped event
		sm.recorder.Event(workspace, corev1.EventTypeNormal, "WorkspaceStopped", "Workspace has been stopped")

		if err := sm.statusManager.UpdateStoppedStatus(ctx, workspace); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	// If EITHER deployment OR service is still in the process of being deleted
	// Update status to Stopping and requeue to check again later
	if deploymentDeleted || serviceDeleted {
		logger.Info("Resources still being deleted", "deploymentDeleted", deploymentDeleted, "serviceDeleted", serviceDeleted)
		if err := sm.statusManager.UpdateStoppingStatus(ctx, workspace, deploymentDeleted, serviceDeleted); err != nil {
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

	// Validate template BEFORE creating any resources
	var resolvedTemplate *ResolvedTemplate
	if workspace.Spec.TemplateRef != nil {
		validation, err := sm.templateResolver.ValidateAndResolveTemplate(ctx, workspace)
		if err != nil {
			// System error (couldn't fetch template, etc.)
			logger.Error(err, "Failed to validate template")
			if statusErr := sm.statusManager.UpdateErrorStatus(ctx, workspace, ReasonDeploymentError, err.Error()); statusErr != nil {
				logger.Error(statusErr, "Failed to update error status")
			}
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}

		if !validation.Valid {
			// Validation failed - this is a SUCCESS (policy enforced)
			logger.Info("Validation failed, rejecting workspace", "violations", len(validation.Violations))

			// Record validation failure event
			templateName := *workspace.Spec.TemplateRef
			message := fmt.Sprintf("Validation failed for %s with %d violations", templateName, len(validation.Violations))
			sm.recorder.Event(workspace, corev1.EventTypeWarning, "ValidationFailed", message)

			if statusErr := sm.statusManager.SetInvalid(ctx, workspace, validation); statusErr != nil {
				logger.Error(statusErr, "Failed to update validation status")
			}
			// No error returned - we successfully enforced policy
			return ctrl.Result{}, nil
		}

		resolvedTemplate = validation.Template
		logger.Info("Validation passed")

		// Record successful validation event
		templateName := *workspace.Spec.TemplateRef
		message := "Validation passed for " + templateName
		sm.recorder.Event(workspace, corev1.EventTypeNormal, "Validated", message)

		if statusErr := sm.statusManager.SetValid(ctx, workspace); statusErr != nil {
			logger.Error(statusErr, "Failed to update validation status")
		}
	}

	// Ensure PVC exists first (if storage is configured)
	_, err := sm.resourceManager.EnsurePVCExists(ctx, workspace, resolvedTemplate)
	if err != nil {
		pvcErr := fmt.Errorf("failed to ensure PVC exists: %w", err)
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, workspace, ReasonDeploymentError, pvcErr.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, pvcErr
	}

	// Ensure deployment exists (pass the resolved template)
	// EnsureDeploymentExists internally fetches the deployment and returns it with current status
	// On first reconciliation: creates deployment (status will be empty initially)
	// On subsequent reconciliations: returns existing deployment with populated status
	deployment, err := sm.resourceManager.EnsureDeploymentExists(ctx, workspace, resolvedTemplate)
	if err != nil {
		deployErr := fmt.Errorf("failed to ensure deployment exists: %w", err)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(ctx, workspace, ReasonDeploymentError, deployErr.Error()); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, deployErr
	}

	// Ensure service exists
	// EnsureServiceExists internally fetches the service and returns it with current status
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

	if deploymentReady && serviceReady {
		// Both resources are ready, update to running status
		logger.Info("Deployment and Service are both ready, updating to Running status")

		// Record workspace running event
		sm.recorder.Event(workspace, corev1.EventTypeNormal, "WorkspaceRunning", "Workspace is now running")

		if err := sm.statusManager.UpdateRunningStatus(ctx, workspace); err != nil {
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}
		return ctrl.Result{}, nil
	}

	// Resources are being created/started but not fully ready yet
	// Update status to Starting and requeue to check again later
	logger.Info("Resources not fully ready", "deploymentReady", deploymentReady, "serviceReady", serviceReady)
	if err := sm.statusManager.UpdateStartingStatus(
		ctx, workspace, deploymentReady, serviceReady, deployment.GetName(), service.GetName()); err != nil {
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Requeue to check resource readiness again later
	return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
}
