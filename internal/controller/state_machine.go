/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package controller

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// StateMachineInterface defines the interface for state machine operations
type StateMachineInterface interface {
	ReconcileDesiredState(ctx context.Context, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (ctrl.Result, error)
	ReconcileDeletion(ctx context.Context, workspace *workspacev1alpha1.Workspace) (ctrl.Result, error)
	getDesiredStatus(workspace *workspacev1alpha1.Workspace) string
	GetAccessStrategyForWorkspace(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*workspacev1alpha1.WorkspaceAccessStrategy, error)
}

// StateMachine handles the state transitions for Workspace
type StateMachine struct {
	resourceManager *ResourceManager
	statusManager   *StatusManager
	recorder        record.EventRecorder
	idleChecker     *WorkspaceIdleChecker
}

// NewStateMachine creates a new StateMachine
func NewStateMachine(
	resourceManager *ResourceManager,
	statusManager *StatusManager,
	recorder record.EventRecorder,
	idleChecker *WorkspaceIdleChecker,
) *StateMachine {
	return &StateMachine{
		resourceManager: resourceManager,
		statusManager:   statusManager,
		recorder:        recorder,
		idleChecker:     idleChecker,
	}
}

// ReconcileDesiredState handles the state machine logic for Workspace
func (sm *StateMachine) ReconcileDesiredState(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)

	desiredStatus := sm.getDesiredStatus(workspace)
	snapshotStatus := workspace.DeepCopy().Status

	switch desiredStatus {
	case DesiredStateStopped:
		return sm.reconcileDesiredStoppedStatus(ctx, workspace, &snapshotStatus)
	case DesiredStateRunning:
		return sm.reconcileDesiredRunningStatus(ctx, workspace, &snapshotStatus, accessStrategy)
	default:
		err := fmt.Errorf("unknown desired status: %s", desiredStatus)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonDeploymentError, err.Error(), &snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
			return ctrl.Result{}, err
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

// GetAccessStrategyForWorkspace retrieves the AccessStrategy for a workspace
func (sm *StateMachine) GetAccessStrategyForWorkspace(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
	return sm.resourceManager.GetAccessStrategyForWorkspace(ctx, workspace)
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
		return ctrl.Result{}, err
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
		return ctrl.Result{}, err
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
			return ctrl.Result{}, accessError
		} else if !accessResourcesDeleted {
			// AccessResources are not fully deleted, requeue
			readiness := WorkspaceStoppingReadiness{
				computeStopped:         deploymentDeleted,
				serviceStopped:         serviceDeleted,
				accessResourcesStopped: accessResourcesDeleted,
			}
			if err := sm.statusManager.UpdateStoppingStatus(ctx, workspace, readiness, snapshotStatus); err != nil {
				return ctrl.Result{}, err
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
			accessResourcesStopped: accessResourcesDeleted,
		}
		if err := sm.statusManager.UpdateStoppingStatus(
			ctx, workspace, readiness, snapshotStatus); err != nil {
			return ctrl.Result{}, err
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
	return ctrl.Result{}, err
}

func (sm *StateMachine) reconcileDesiredRunningStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	snapshotStatus *workspacev1alpha1.WorkspaceStatus,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (ctrl.Result, error) {
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
		return ctrl.Result{}, pvcErr
	}

	// EnsureDeploymentExists creates deployment if missing, or returns existing deployment
	deployment, err := sm.resourceManager.EnsureDeploymentExists(ctx, workspace, accessStrategy)
	if err != nil {
		deployErr := fmt.Errorf("failed to ensure deployment exists: %w", err)
		// Update error condition
		if statusErr := sm.statusManager.UpdateErrorStatus(
			ctx, workspace, ReasonDeploymentError, deployErr.Error(), snapshotStatus); statusErr != nil {
			logger.Error(statusErr, "Failed to update error status")
		}
		return ctrl.Result{}, deployErr
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
		return ctrl.Result{}, serviceErr
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
		if err := sm.ReconcileAccessForDesiredRunningStatus(ctx, workspace, service, accessStrategy); err != nil {
			return ctrl.Result{}, err
		}

		// Then only update to running status
		logger.Info("Deployment and Service are both ready, updating to Running status")

		// Record workspace running event
		sm.recorder.Event(workspace, corev1.EventTypeNormal, "WorkspaceRunning", "Workspace is now running")

		if err := sm.statusManager.UpdateRunningStatus(ctx, workspace, snapshotStatus); err != nil {
			return ctrl.Result{}, err
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
		return ctrl.Result{}, err
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
		"idleTimeoutInMinutes", idleConfig.IdleTimeoutInMinutes,
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
				"timeout", idleConfig.IdleTimeoutInMinutes)
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
		fmt.Sprintf("Stopping workspace due to idle timeout of %d minutes", idleConfig.IdleTimeoutInMinutes))

	// Update desired status to trigger stop
	workspace.Spec.DesiredStatus = DesiredStateStopped
	if err := sm.resourceManager.client.Update(ctx, workspace); err != nil {
		logger.Error(err, "Failed to update workspace desired status")
		return ctrl.Result{}, err
	}

	logger.Info("Updated workspace desired status to Stopped")

	// Requeue after a minimal wait
	return ctrl.Result{RequeueAfter: MinimalRequeueDelay}, nil
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

// ReconcileDeletion handles workspace deletion with finalizer management
func (sm *StateMachine) ReconcileDeletion(ctx context.Context, workspace *workspacev1alpha1.Workspace) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Handling workspace deletion", "workspace", workspace.Name)

	if !controllerutil.ContainsFinalizer(workspace, WorkspaceFinalizerName) {
		logger.Info("No finalizer present, allowing deletion")
		return ctrl.Result{}, nil
	}

	// Update status to Deleting
	if err := sm.statusManager.UpdateDeletingStatus(ctx, workspace); err != nil {
		logger.Error(err, "Failed to update deleting status")
		return ctrl.Result{}, err
	}

	// Clean up all workspace resources via resource manager
	allDeleted, err := sm.resourceManager.CleanupAllResources(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to cleanup workspace resources")
		return ctrl.Result{}, err
	}
	if !allDeleted {
		logger.Info("Resources still being deleted, will retry")
		return ctrl.Result{RequeueAfter: PollRequeueDelay}, nil
	}

	// All resources cleaned up, remove finalizer to allow deletion
	logger.Info("All resources cleaned up, removing finalizer")
	controllerutil.RemoveFinalizer(workspace, WorkspaceFinalizerName)
	if err := sm.resourceManager.client.Update(ctx, workspace); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	logger.Info("Finalizer removed, workspace deletion will proceed")
	return ctrl.Result{}, nil
}
