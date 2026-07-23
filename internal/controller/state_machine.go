/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"time"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
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
	resourceManager     *ResourceManager
	statusManager       *StatusManager
	recorder            record.EventRecorder
	idleChecker         *WorkspaceIdleChecker
	accessStartupProber AccessStartupProberInterface
	// integrationProber runs each integration's report-only statusProbe and writes the verdict into
	// status.integrationStatuses[]. It may be nil (e.g. when pod-exec config is unavailable); the probe step
	// is then skipped and integrations report no status. It also owns the base re-probe cadence
	// (ProbePeriod), mirroring how idleChecker owns CheckInterval.
	integrationProber IntegrationProberInterface
}

// NewStateMachine creates a new StateMachine
func NewStateMachine(
	resourceManager *ResourceManager,
	statusManager *StatusManager,
	recorder record.EventRecorder,
	idleChecker *WorkspaceIdleChecker,
	accessStartupProber AccessStartupProberInterface,
	integrationProber IntegrationProberInterface,
) *StateMachine {
	return &StateMachine{
		resourceManager:     resourceManager,
		statusManager:       statusManager,
		recorder:            recorder,
		idleChecker:         idleChecker,
		accessStartupProber: accessStartupProber,
		integrationProber:   integrationProber,
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

	// Integration workspaces: refresh the frozen resolution (status.resolvedIntegrations) alongside the
	// access strategy fetch, BEFORE building the Deployment, so create/update below replay the current
	// frozen values and reuse the templates loaded here. Re-resolution happens only on an input-token
	// change. A reconcile error is non-fatal (fail-closed -- preserve the running pod); the in-memory
	// status it records is persisted by this reconcile's single status write below.
	var integrationTemplates map[string]*workspacev1alpha1.WorkspaceIntegrationTemplate
	if sm.resourceManager.hasIntegrationTemplateRefs(workspace) {
		var integrationErr error
		integrationTemplates, integrationErr = sm.resourceManager.reconcileIntegrations(ctx, workspace)
		if integrationErr != nil {
			logger.Error(integrationErr, "integration freeze reconcile reported an error; proceeding with preserved frozen values",
				"workspace", workspace.Name)
		}
	}

	// EnsureDeploymentExists creates deployment if missing, or returns existing deployment
	deployment, err := sm.resourceManager.EnsureDeploymentExists(ctx, workspace, accessStrategy, integrationTemplates)
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

	// Apply access strategy when compute and service resources are ready.
	// ReconcileAccessForDesiredRunningStatus assumes instantaneous effect after etcd
	// update — it does not verify that the access route is actually serving traffic.
	accessResourcesReady := false
	requeueDelay := PollRequeueDelay
	if deploymentReady && serviceReady {
		if err := sm.ReconcileAccessForDesiredRunningStatus(ctx, workspace, service, accessStrategy); err != nil {
			return ctrl.Result{}, err
		}

		// Gate on access startup probe before marking Available.
		probeResult, probeErr := sm.ProbeAccessStartup(ctx, workspace, accessStrategy, service)
		if probeErr != nil {
			return ctrl.Result{}, probeErr
		}

		switch probeResult.Status {
		case ProbeNotDefined:
			accessResourcesReady = true
		case ProbeSucceeded:
			accessResourcesReady = true
		case ProbeAlreadySucceeded:
			accessResourcesReady = true
		case ProbeFailureThresholdExceeded:
			if statusErr := sm.statusManager.UpdatePermanentDegradedRunningStatus(
				ctx, workspace, ReasonAccessProbeThresholdExceeded, ReasonAccessNotReady,
				"Access startup probe failed: threshold exceeded",
				snapshotStatus); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			// After status update, exit and stop requeuing
			return ctrl.Result{}, nil
		case ProbeRetrying:
			requeueDelay = probeResult.RequeueAfter
		case ProbePendingRetry:
			requeueDelay = probeResult.RequeueAfter
		}
	}

	if deploymentReady && serviceReady && accessResourcesReady {
		logger.Info("Deployment and Service are both ready, updating to Running status")
		sm.recorder.Event(workspace, corev1.EventTypeNormal, "WorkspaceRunning", "Workspace is now running")

		// Run report-only integration status probes and stage their verdicts into status.integrationStatuses[]
		// BEFORE the status write, so they persist in the same Status().Update as the Running conditions.
		// Non-gating: probe verdicts never change whether the workspace is Available.
		integrationProbeInterval := sm.probeIntegrationStatus(ctx, workspace)

		if err := sm.statusManager.UpdateRunningStatus(ctx, workspace, snapshotStatus); err != nil {
			return ctrl.Result{}, err
		}

		// Handle idle shutdown for running workspaces, honoring the sooner of the idle requeue and the
		// integration probe cadence (there is no watch driving re-probes, so we must requeue to re-run).
		result, err := sm.handleIdleShutdownForRunningWorkspace(ctx, workspace, service)
		if err != nil {
			return result, err
		}
		result.RequeueAfter = getShorterInterval(result.RequeueAfter, integrationProbeInterval)
		return result, nil
	}

	// Resources are being created/started but not fully ready yet
	// Update status to Starting and requeue to check again later
	logger.Info("Resources not fully ready",
		"deploymentReady", deploymentReady, "serviceReady", serviceReady,
		"accessResourcesReady", accessResourcesReady)
	workspace.Status.DeploymentName = deployment.GetName()
	workspace.Status.ServiceName = service.GetName()
	readiness := WorkspaceRunningReadiness{
		computeReady:         deploymentReady,
		serviceReady:         serviceReady,
		accessResourcesReady: accessResourcesReady,
	}
	if err := sm.statusManager.UpdateStartingStatus(
		ctx, workspace, readiness, snapshotStatus); err != nil {
		return ctrl.Result{}, err
	}

	// Requeue to check resource readiness again later
	return ctrl.Result{RequeueAfter: requeueDelay}, nil
}

// probeIntegrationStatus runs each attached integration's report-only statusProbe and writes one
// verdict per integration into workspace.Status.integrationStatuses. It is non-gating and best-effort:
//   - An integration with no FROZEN values (status.resolvedIntegrations record) has not resolved yet --
//     its first-attach capture failed. It contributes a Degraded/NotResolved entry (rather than being
//     silently skipped) so the unresolved integration is visible on the Workspace status, and self-
//     corrects to Ready once capture eventually succeeds.
//   - The statusProbe spec is read from the template (a static admin-authored field); reading it does
//     not re-read the referenced resource, so the freeze contract is preserved. A template that has
//     gone missing contributes a not-ready ProbeError entry rather than failing the reconcile.
//   - With no integrations at all, status.integrationStatuses[] is cleared so a removed integration leaves no
//     stale entry.
//
// Returns the shortest interval after which any probe should re-run, or 0 when nothing needs probing
// (so the caller knows whether to keep requeuing the Running workspace on the probe cadence -- there
// is no watch on the referenced resource driving re-probes).
func (sm *StateMachine) probeIntegrationStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) time.Duration {
	if sm.integrationProber == nil || !sm.resourceManager.hasIntegrationTemplateRefs(workspace) {
		workspace.Status.IntegrationStatuses = nil
		return 0
	}
	logger := logf.FromContext(ctx)

	// Index the prior verdicts by name so an unchanged re-probe can preserve its LastTransitionTime
	// (avoids churning the status subresource on every probe cadence).
	prior := make(map[string]*workspacev1alpha1.IntegrationStatus, len(workspace.Status.IntegrationStatuses))
	for i := range workspace.Status.IntegrationStatuses {
		prior[workspace.Status.IntegrationStatuses[i].Name] = &workspace.Status.IntegrationStatuses[i]
	}

	// Resolve the workspace pod ONCE for the whole pass -- every Probe execs into the same pod, so N
	// integrations must not each list pods. A lookup failure isn't fatal: the pod may still be starting;
	// each probed integration then reports PodNotFound and we retry on the base cadence.
	pod, podErr := sm.integrationProber.FindRunningPod(ctx, workspace)

	var statuses []workspacev1alpha1.IntegrationStatus
	// appendStatus preserves the prior timestamp on an unchanged verdict and records the status. There is
	// no per-verdict interval math: every integration re-probes on the same flat operator cadence (the
	// prober's ProbePeriod), mirroring how the idle checker requeues on a flat CheckInterval. It also
	// records a Kubernetes Event on a verdict TRANSITION (edge-triggered: never on an unchanged re-probe),
	// so a degraded/recovered integration is visible via `kubectl describe workspace`, not only in status.
	appendStatus := func(s workspacev1alpha1.IntegrationStatus) {
		if sm.recorder != nil {
			if ev := getIntegrationStatusEvent(s.Name, prior[s.Name], &s); ev.Type != "" {
				sm.recorder.Event(workspace, ev.Type, ev.Reason, ev.Message)
			}
		}
		preserveConditionTimestamp(prior[s.Name], &s)
		statuses = append(statuses, s)
	}
	for i := range workspace.Spec.IntegrationTemplateRefs {
		ref := &workspace.Spec.IntegrationTemplateRefs[i]
		// An integration with no frozen record has not resolved yet -- its first-attach capture failed
		// (e.g. the referenced resource does not exist, or the template is broken). The freeze reconcile
		// (which ran earlier this reconcile) already logged the detailed cause; surface a Degraded entry
		// so an admin sees the unresolved integration on the Workspace status rather than only in logs.
		if findResolvedIntegration(&workspace.Status, ref.Name) == nil {
			appendStatus(buildIntegrationStatus(ref.Name, false, IntegrationReasonNotResolved,
				"integration has not resolved yet (first-attach capture failed; see operator logs for the cause)"))
			continue
		}
		probe, err := sm.resourceManager.getIntegrationStatusProbeSpec(ctx, workspace, ref)
		if err != nil {
			// The template went missing after freeze: report not-ready rather than dropping the entry.
			logger.Error(err, "Failed to load integration template for status probe", "integration", ref.Name)
			appendStatus(buildIntegrationStatus(ref.Name, false, IntegrationReasonProbeError, err.Error()))
			continue
		}
		if probe == nil {
			// Resolved, but no statusProbe declared -> nothing to report for this integration.
			continue
		}
		if podErr != nil {
			// No running pod yet: report PodNotFound and retry on the base cadence (no exec attempted).
			appendStatus(buildIntegrationStatus(ref.Name, false, IntegrationReasonPodNotFound, podErr.Error()))
			continue
		}
		appendStatus(sm.integrationProber.Probe(ctx, pod, ref.Name, probe))
	}

	// Assign (possibly nil) so a removed or probe-less integration leaves no stale entry behind.
	workspace.Status.IntegrationStatuses = statuses
	// Requeue on the flat operator cadence when there is at least one integration status to refresh;
	// 0 means "no integration timer" (getShorterInterval treats it as absent), so a workspace with no
	// reportable integrations adds no requeue of its own.
	if len(statuses) == 0 {
		return 0
	}
	return sm.integrationProber.ProbePeriod()
}

// getShorterInterval returns the shorter of two requeue delays. A delay of 0 means "no timer"
// (controller-runtime's meaning for RequeueAfter == 0) and is skipped, so it does not win as the
// numerically smallest value: with one side 0 the other wins, with both 0 the result is 0 (no
// requeue), and with both set the smaller wins. Commutative. Used to pick the shorter of the
// idle-shutdown requeue and the integration probe cadence.
func getShorterInterval(a, b time.Duration) time.Duration {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	return min(a, b)
}

// handleIdleShutdownForRunningWorkspace handles idle shutdown logic for running workspaces
func (sm *StateMachine) handleIdleShutdownForRunningWorkspace(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	service *corev1.Service) (ctrl.Result, error) {

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

	// No explicit pod-readiness check needed here: this code only runs after
	// IsDeploymentAvailable()==true, which guarantees at least one ready pod.
	// If a pod crashes between the deployment check and the idle probe, the
	// probe returns ShouldRetry=true and we retry next cycle.
	result, err := sm.idleChecker.CheckWorkspaceIdle(ctx, workspace, service, idleConfig)
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
	logger.V(1).Info("Scheduling next idle check", "interval", sm.idleChecker.CheckInterval())
	return ctrl.Result{RequeueAfter: sm.idleChecker.CheckInterval()}, nil
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

// ReconcileDeletion handles workspace deletion with finalizer management
func (sm *StateMachine) ReconcileDeletion(ctx context.Context, workspace *workspacev1alpha1.Workspace) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Handling workspace deletion", "workspace", workspace.Name)

	if !controllerutil.ContainsFinalizer(workspace, WorkspaceFinalizerName) {
		logger.Info("No finalizer present, allowing deletion")
		return ctrl.Result{}, nil
	}

	// Update status to Deleting
	snapshotStatus := workspace.Status.DeepCopy()
	if err := sm.statusManager.UpdateDeletingStatus(ctx, workspace, snapshotStatus); err != nil {
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
