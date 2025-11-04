package controller

import (
	"context"
	"fmt"
	"time"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// TemplateValidatorInterface defines the interface for template validation
// This allows the controller to use the webhook's TemplateValidator without circular dependency
type TemplateValidatorInterface interface {
	ValidateCreateWorkspace(ctx context.Context, workspace *workspacev1alpha1.Workspace) error
}

// StateMachine handles the state transitions for Workspace
type StateMachine struct {
	resourceManager   *ResourceManager
	statusManager     *StatusManager
	recorder          record.EventRecorder
	idleChecker       *WorkspaceIdleChecker
	templateValidator TemplateValidatorInterface
}

// NewStateMachine creates a new StateMachine
func NewStateMachine(
	resourceManager *ResourceManager,
	statusManager *StatusManager,
	recorder record.EventRecorder,
	idleChecker *WorkspaceIdleChecker,
	templateValidator TemplateValidatorInterface,
) *StateMachine {
	return &StateMachine{
		resourceManager:   resourceManager,
		statusManager:     statusManager,
		recorder:          recorder,
		idleChecker:       idleChecker,
		templateValidator: templateValidator,
	}
}

// ReconcileDesiredState handles the state machine logic for Workspace
func (sm *StateMachine) ReconcileDesiredState(
	ctx context.Context, workspace *workspacev1alpha1.Workspace) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	desiredStatus := sm.getDesiredStatus(workspace)
	snapshotStatus := workspace.DeepCopy().Status

	// Check if compliance check is needed (template constraints changed)
	if result, err := sm.checkComplianceIfNeeded(ctx, workspace, &snapshotStatus); err != nil || result.RequeueAfter > 0 {
		return result, err
	}

	switch desiredStatus {
	case PhaseStopped:
		return sm.reconcileDesiredStoppedStatus(ctx, workspace, &snapshotStatus)
	case "Running":
		return sm.reconcileDesiredRunningStatus(ctx, workspace, &snapshotStatus)
	default:
		err := fmt.Errorf("unknown desired status: %s", desiredStatus)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonDeploymentError, err.Error(), &snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: LongRequeueDelay}, err
	}
}

// getDesiredStatus returns the desired status with default fallback
func (sm *StateMachine) getDesiredStatus(workspace *workspacev1alpha1.Workspace) string {
	if workspace.Spec.DesiredStatus == "" {
		return DefaultDesiredStatus
	}
	return workspace.Spec.DesiredStatus
}

func (sm *StateMachine) reconcileDesiredStoppedStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	snapshotStatus *workspacev1alpha1.WorkspaceStatus) (ctrl.Result, error) {
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
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonDeploymentError, err.Error(), snapshotStatus); statusErr != nil {
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
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonServiceError, err.Error(), snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Check if resources are fully deleted (asynchronous deletion check)
	// A nil resource means the resource has been fully deleted
	deploymentDeleted := sm.resourceManager.IsDeploymentMissingOrDeleting(deployment)
	serviceDeleted := sm.resourceManager.IsServiceMissingOrDeleting(service)
	accessResourcesDeleted := sm.resourceManager.AreAccessResourcesDeleted(workspace)

	if deploymentDeleted && serviceDeleted {
		// Flag as Error if AccessResources failed to delete
		if accessError != nil {
			if statusErr := sm.statusManager.UpdateErrorStatus(
				ctx, workspace, ReasonServiceError, accessError.Error(), snapshotStatus); statusErr != nil {
				logger.Error(statusErr, "Failed to update error status")
			}
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, accessError
		} else if !accessResourcesDeleted {
			// AccessResources are not fully deleted, requeue
			readiness := WorkspaceStoppingReadiness{
				computeStopped:         deploymentDeleted,
				serviceStopped:         serviceDeleted,
				accessResourcesStopped: accessResourcesDeleted,
			}
			if err := sm.statusManager.UpdateStoppingStatus(ctx, workspace, readiness, snapshotStatus); err != nil {
				return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
			}
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
		} else {
			// All resources are fully deleted, update to stopped status
			logger.Info("Deployment and Service are both deleted, updating to Stopped status")

			// Record workspace stopped event with specific message for preemption
			if workspace.Annotations != nil && workspace.Annotations[PreemptionReasonAnnotation] == PreemptedReason {
				sm.recorder.Event(workspace, corev1.EventTypeNormal, "WorkspaceStopped", PreemptedReason)
			} else {
				sm.recorder.Event(workspace, corev1.EventTypeNormal, "WorkspaceStopped", "Workspace has been stopped")
			}

			if err := sm.statusManager.UpdateStoppedStatus(ctx, workspace, snapshotStatus); err != nil {
				return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
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
			accessResourcesStopped: accessResourcesDeleted,
		}
		if err := sm.statusManager.UpdateStoppingStatus(
			ctx, workspace, readiness, snapshotStatus); err != nil {
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}
		// Requeue to check deletion progress again later
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
	}

	// This should not happen, return an error
	err := fmt.Errorf("unexpected state: both deployment and service should be in deletion process")
	// Update error condition
	if statusErr := sm.statusManager.UpdateErrorStatus(
		ctx, workspace, ReasonDeploymentError, err.Error(), snapshotStatus); statusErr != nil {
		logger.Error(statusErr, "Failed to update error status")
	}
	return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
}

func (sm *StateMachine) reconcileDesiredRunningStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	snapshotStatus *workspacev1alpha1.WorkspaceStatus) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Attempting to bring Workspace status to 'Running'")

	// Ensure PVC exists first (if storage is configured)
	_, err := sm.resourceManager.EnsurePVCExists(ctx, workspace)
	if err != nil {
		pvcErr := fmt.Errorf("failed to ensure PVC exists: %w", err)
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonDeploymentError, pvcErr.Error(), snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, pvcErr
	}

	// EnsureDeploymentExists creates deployment if missing, or returns existing deployment
	deployment, err := sm.resourceManager.EnsureDeploymentExists(ctx, workspace)
	if err != nil {
		deployErr := fmt.Errorf("failed to ensure deployment exists: %w", err)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonDeploymentError, deployErr.Error(), snapshotStatus); statusErr != nil {
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
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonServiceError, serviceErr.Error(), snapshotStatus); statusErr != nil {
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
		// ReconcileAccess returns nil (no error) only when it successfully initiated
		// the creation of all AccessRessources.
		// TODO: add probe and requeue https://github.com/jupyter-infra/jupyter-k8s/issues/36
		if err := sm.ReconcileAccessForDesiredRunningStatus(ctx, workspace, service); err != nil {
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}

		// Then only update to running status
		logger.Info("Deployment and Service are both ready, updating to Running status")

		// Record workspace running event
		sm.recorder.Event(workspace, corev1.EventTypeNormal, "WorkspaceRunning", "Workspace is now running")

		if err := sm.statusManager.UpdateRunningStatus(ctx, workspace, snapshotStatus); err != nil {
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
		}

		// Handle idle shutdown for running workspaces
		return sm.handleIdleShutdownForRunningWorkspace(ctx, workspace)
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
	if err := sm.statusManager.UpdateStartingStatus(
		ctx, workspace, readiness, snapshotStatus); err != nil {
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Requeue to check resource readiness again later
	return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
}

// handleIdleShutdownForRunningWorkspace handles idle shutdown logic for running workspaces
func (sm *StateMachine) handleIdleShutdownForRunningWorkspace(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace) (ctrl.Result, error) {

	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name, "resourceVersion", workspace.ResourceVersion)

	idleConfig := workspace.Spec.IdleShutdown

	// If idle shutdown is not enabled, no requeue needed
	if idleConfig == nil || !idleConfig.Enabled {
		logger.V(2).Info("Idle shutdown not enabled")
		return ctrl.Result{}, nil
	}

	logger.Info("Processing idle shutdown",
		"enabled", idleConfig.Enabled,
		"timeoutMinutes", idleConfig.TimeoutMinutes,
		"hasHTTPGet", idleConfig.Detection.HTTPGet != nil,
		"workspace", workspace.Name,
		"namespace", workspace.Namespace)

	// Check if pods are actually ready for idle checking
	podsReady, err := sm.isAtLeastOneWorkspacePodReady(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to check pod readiness")
		return ctrl.Result{RequeueAfter: IdleCheckInterval}, nil
	}

	if !podsReady {
		logger.Info("Workspace pods not ready yet, skipping idle check")
		return ctrl.Result{RequeueAfter: IdleCheckInterval}, nil
	}

	result, err := sm.idleChecker.CheckWorkspaceIdle(ctx, workspace, idleConfig)
	if err != nil {
		if !result.ShouldRetry {
			logger.Error(err, "Permanent failure checking idle status, disabling idle shutdown for this workspace")
			return ctrl.Result{}, nil // No requeue - permanent failure
		}
		// Temporary errors - keep retrying
		logger.Error(err, "Temporary failure checking idle status, will retry")
	} else {
		logger.V(1).Info("Successfully checked idle status", "isIdle", result.IsIdle)
		if result.IsIdle {
			logger.Info("Workspace idle timeout reached, stopping workspace",
				"timeout", idleConfig.TimeoutMinutes)
			return sm.stopWorkspaceDueToIdle(ctx, workspace, idleConfig)
		}
	}

	// Requeue for next idle check
	logger.V(1).Info("Scheduling next idle check", "interval", IdleCheckInterval)
	return ctrl.Result{RequeueAfter: IdleCheckInterval}, nil
}

// stopWorkspaceDueToIdle stops the workspace due to idle timeout
func (sm *StateMachine) stopWorkspaceDueToIdle(ctx context.Context, workspace *workspacev1alpha1.Workspace, idleConfig *workspacev1alpha1.IdleShutdownSpec) (ctrl.Result, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	// Record event
	sm.recorder.Event(workspace, corev1.EventTypeNormal, "IdleShutdown",
		fmt.Sprintf("Stopping workspace due to idle timeout of %d minutes", idleConfig.TimeoutMinutes))

	// Update desired status to trigger stop
	workspace.Spec.DesiredStatus = PhaseStopped
	if err := sm.resourceManager.client.Update(ctx, workspace); err != nil {
		logger.Error(err, "Failed to update workspace desired status")
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	logger.Info("Updated workspace desired status to Stopped")

	// Requeue after a minimal wait
	return ctrl.Result{RequeueAfter: 10 * time.Millisecond}, nil
}

// isAtLeastOneWorkspacePodReady checks if workspace pods are ready for idle checking
func (sm *StateMachine) isAtLeastOneWorkspacePodReady(ctx context.Context, workspace *workspacev1alpha1.Workspace) (bool, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	// List pods with the workspace labels
	podList := &corev1.PodList{}
	labels := GenerateLabels(workspace.Name)

	if err := sm.resourceManager.client.List(ctx, podList, client.InNamespace(workspace.Namespace), client.MatchingLabels(labels)); err != nil {
		return false, fmt.Errorf("failed to list pods: %w", err)
	}

	// Check if we have any running and ready pods
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			// Check if pod is ready (all readiness probes passed)
			for _, condition := range pod.Status.Conditions {
				if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
					logger.V(1).Info("Found ready workspace pod", "pod", pod.Name)
					return true, nil
				}
			}
			logger.V(1).Info("Found running but not ready pod", "pod", pod.Name)
		}
	}

	logger.V(1).Info("No ready workspace pods found")
	return false, nil
}

// checkComplianceIfNeeded checks if template compliance validation is needed and performs it
// This is triggered when template constraints change and workspaces need re-validation
//
// ARCHITECTURAL NOTE: Compliance checking belongs in the controller (not webhook) because:
//  1. Webhooks have strict timeouts (<10s) - validating all workspaces during template update would timeout
//  2. Only controllers can update resource status - webhooks don't have status subresource access
//  3. This implements an asynchronous pattern: template webhook marks workspaces with a label,
//     then this controller validates marked workspaces during normal reconciliation
//  4. This follows standard Kubernetes patterns for label-triggered reconciliation
//
// The flow is: Template updated → Webhook adds compliance label → Controller validates → Status updated
func (sm *StateMachine) checkComplianceIfNeeded(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	snapshotStatus *workspacev1alpha1.WorkspaceStatus) (ctrl.Result, error) {

	logger := logf.FromContext(ctx)

	// Check if compliance check is needed
	if workspace.Labels == nil || workspace.Labels[LabelComplianceCheckNeeded] != "true" {
		return ctrl.Result{}, nil
	}

	logger.Info("Compliance check needed, re-validating workspace against current template constraints",
		"workspace", workspace.Name,
		"namespace", workspace.Namespace,
		"template", getTemplateRef(workspace))

	// Perform compliance check if validator is available
	var validationErr error
	if sm.templateValidator != nil {
		validationErr = sm.templateValidator.ValidateCreateWorkspace(ctx, workspace)
	} else {
		logger.Info("Template validator not available, skipping compliance check")
	}

	// Remove compliance label regardless of validation result (idempotent operation)
	// This ensures we don't get stuck in validation loops
	if err := sm.removeComplianceLabel(ctx, workspace); err != nil {
		logger.Error(err, "Failed to remove compliance label, will retry")
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, err
	}

	// Handle validation result
	if validationErr != nil {
		logger.Info("Workspace failed compliance check",
			"workspace", workspace.Name,
			"namespace", workspace.Namespace,
			"error", validationErr.Error())

		// Record event for visibility
		sm.recorder.Event(workspace, corev1.EventTypeWarning, "ComplianceCheckFailed",
			fmt.Sprintf("Workspace no longer complies with template constraints: %s", validationErr.Error()))

		// Parse violation from error to set status properly
		validation := &TemplateValidationResult{
			Violations: []TemplateViolation{
				{
					Message: validationErr.Error(),
					Field:   "template constraints",
				},
			},
		}

		// Update status to Invalid
		if statusErr := sm.statusManager.SetInvalid(ctx, workspace, validation, snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update invalid status")
			return ctrl.Result{RequeueAfter: PollRequeueDelay}, statusErr
		}

		// Return error to requeue and allow user to fix the workspace
		return ctrl.Result{RequeueAfter: LongRequeueDelay}, validationErr
	}

	logger.Info("Workspace passed compliance check",
		"workspace", workspace.Name,
		"namespace", workspace.Namespace)

	// Record success event
	sm.recorder.Event(workspace, corev1.EventTypeNormal, "ComplianceCheckPassed",
		"Workspace complies with current template constraints")

	// Continue with normal reconciliation
	return ctrl.Result{}, nil
}

// removeComplianceLabel removes the compliance check label from the workspace
func (sm *StateMachine) removeComplianceLabel(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	logger := logf.FromContext(ctx)

	// Remove the label
	delete(workspace.Labels, LabelComplianceCheckNeeded)

	// Update the workspace to persist label removal
	if err := sm.resourceManager.client.Update(ctx, workspace); err != nil {
		logger.Error(err, "Failed to remove compliance check label")
		return fmt.Errorf("failed to remove compliance label: %w", err)
	}

	logger.V(1).Info("Removed compliance check label",
		"workspace", workspace.Name,
		"namespace", workspace.Namespace)

	return nil
}

// getTemplateRef returns the template ref name or empty string if not set
func getTemplateRef(workspace *workspacev1alpha1.Workspace) string {
	if workspace.Spec.TemplateRef == nil {
		return ""
	}
	return workspace.Spec.TemplateRef.Name
}
