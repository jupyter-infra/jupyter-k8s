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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ReconcileAccessForDesiredRunningStatus reconciles the access strategy for a Workspace whose desired state is Running
func (sm *StateMachine) ReconcileAccessForDesiredRunningStatus(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	service *corev1.Service,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) error {
	logger := logf.FromContext(ctx)
	accessStrategyRef := workspace.Spec.AccessStrategy

	// CASE 1: there is an AccessStrategy
	// ensure the AccessResources exist
	if accessStrategyRef != nil {

		ensureAccessResourceErr := sm.resourceManager.EnsureAccessResourcesExist(ctx, workspace, accessStrategy, service)
		if ensureAccessResourceErr != nil {
			logger.Error(ensureAccessResourceErr, "Failed to apply access strategy")
			return ensureAccessResourceErr
		}

		accessUrl, accessUrlErr := sm.resourceManager.accessResourcesBuilder.ResolveAccessURL(workspace, accessStrategy, service)
		if accessUrlErr != nil {
			logger.Error(accessUrlErr, "Failed to retrieve Access URL from access strategy")
			return accessUrlErr
		}
		workspace.Status.AccessURL = accessUrl
		workspace.Status.AccessResourceSelector = sm.resourceManager.accessResourcesBuilder.ResolveAccessResourceSelector(
			workspace, accessStrategy)
		return nil
	}
	// END OF CASE 1

	// CASE 2: there is no AccessStrategy (it may have been removed by an update)
	workspace.Status.AccessURL = ""
	workspace.Status.AccessResourceSelector = ""
	workspace.Status.AccessStartupProbeSucceeded = false
	workspace.Status.ObservedAccessStrategyVersion = ""
	clearProbeState(workspace)

	err := sm.resourceManager.EnsureAccessResourcesDeleted(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to delete access resources")
		return err
	}
	return nil
}

// ReconcileAccessForDesiredStoppedStatus reconciles the access strategy for a Workspace whose desired state is Stopped
func (sm *StateMachine) ReconcileAccessForDesiredStoppedStatus(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	logger := logf.FromContext(ctx)

	workspace.Status.AccessURL = ""
	workspace.Status.AccessResourceSelector = ""
	workspace.Status.AccessStartupProbeSucceeded = false
	workspace.Status.ObservedAccessStrategyVersion = ""
	clearProbeState(workspace)

	err := sm.resourceManager.EnsureAccessResourcesDeleted(ctx, workspace)
	if err != nil {
		logger.Error(err, "Failed to delete access resources")
		return err
	}
	return nil
}

// ProbeStatus indicates the outcome of an access startup probe cycle.
type ProbeStatus int

// ProbeStatus values returned by ProbeAccessStartup.
const (
	ProbeNotDefined ProbeStatus = iota
	ProbeSucceeded
	ProbeRetrying
	ProbePendingRetry
	ProbeFailureThresholdExceeded
	ProbeAlreadySucceeded
)

// ProbeResult carries the outcome and requeue delay from an access startup probe cycle.
type ProbeResult struct {
	Status       ProbeStatus
	RequeueAfter time.Duration
}

// ProbeAccessStartup performs the access startup probe cycle.
// It mutates workspace.Status.AccessStartupProbeFailures in memory; the caller
// is responsible for persisting status and updating conditions.
func (sm *StateMachine) ProbeAccessStartup(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
) (*ProbeResult, error) {
	if accessStrategy == nil || accessStrategy.Spec.AccessStartupProbe == nil {
		return &ProbeResult{Status: ProbeNotDefined}, nil
	}

	// Reset probe state when the AccessStrategy identity or spec changes.
	// Without this, degraded workspaces cannot self-heal after an
	// AccessStrategy fix, and already-succeeded probes are never
	// re-evaluated after a strategy or ref change. (issue #368)
	currentVersionedId := versionedAccessStrategyID(accessStrategy)
	if workspace.Status.ObservedAccessStrategyVersion != currentVersionedId {
		clearProbeState(workspace)
		workspace.Status.AccessStartupProbeSucceeded = false
		workspace.Status.ObservedAccessStrategyVersion = currentVersionedId
	}

	if workspace.Status.AccessStartupProbeSucceeded {
		return &ProbeResult{Status: ProbeAlreadySucceeded}, nil
	}

	logger := logf.FromContext(ctx)
	probe := accessStrategy.Spec.AccessStartupProbe

	failureThreshold := resolveFailureThreshold(probe)
	periodSeconds := resolvePeriodSeconds(probe)

	// Already exceeded threshold on a previous reconcile — stay Degraded
	// without re-probing. The counter is cleared when the workspace stops
	// or the access strategy is removed, which restarts the probe.
	if workspace.Status.AccessStartupProbeFailures != nil &&
		*workspace.Status.AccessStartupProbeFailures >= failureThreshold {
		return &ProbeResult{Status: ProbeFailureThresholdExceeded}, nil
	}

	// First attempt: handle initial delay
	if workspace.Status.AccessStartupProbeFailures == nil {
		zero := int32(0)
		workspace.Status.AccessStartupProbeFailures = &zero

		if probe.InitialDelaySeconds > 0 {
			logger.Info("Access startup probe: waiting for initial delay",
				"initialDelaySeconds", probe.InitialDelaySeconds)
			delay := time.Duration(probe.InitialDelaySeconds) * time.Second
			deadline := metav1.NewTime(time.Now().Add(delay))
			workspace.Status.EarliestNextProbeTime = &deadline
			return &ProbeResult{
				Status:       ProbeRetrying,
				RequeueAfter: delay,
			}, nil
		}
	}

	// Enforce minimum spacing between probes. Status updates trigger
	// watch events that re-reconcile faster than RequeueAfter; skip
	// the actual HTTP probe until the deadline has passed.
	if remaining := timeUntilProbeDeadline(workspace); remaining > 0 {
		return &ProbeResult{
			Status:       ProbePendingRetry,
			RequeueAfter: remaining,
		}, nil
	}

	ready, err := sm.accessStartupProber.Probe(ctx, workspace, accessStrategy, service)
	if err != nil {
		return nil, fmt.Errorf("access startup probe error: %w", err)
	}

	if ready {
		logger.Info("Access startup probe succeeded")
		workspace.Status.AccessStartupProbeSucceeded = true
		clearProbeState(workspace)
		return &ProbeResult{Status: ProbeSucceeded}, nil
	}

	// Probe failed — increment counter
	failures := *workspace.Status.AccessStartupProbeFailures + 1
	workspace.Status.AccessStartupProbeFailures = &failures

	if failures >= failureThreshold {
		logger.Info("Access startup probe failed: threshold exceeded",
			"failures", failures, "failureThreshold", failureThreshold)
		workspace.Status.EarliestNextProbeTime = nil
		return &ProbeResult{Status: ProbeFailureThresholdExceeded}, nil
	}

	retrySeconds := probeRetrySeconds(periodSeconds, failures, failureThreshold)

	delay := time.Duration(retrySeconds) * time.Second
	deadline := metav1.NewTime(time.Now().Add(delay))
	workspace.Status.EarliestNextProbeTime = &deadline

	logger.Info("Access startup probe failed, retrying",
		"failures", failures, "failureThreshold", failureThreshold,
		"retryAfterSeconds", retrySeconds)
	return &ProbeResult{
		Status:       ProbeRetrying,
		RequeueAfter: delay,
	}, nil
}

func clearProbeState(workspace *workspacev1alpha1.Workspace) {
	workspace.Status.AccessStartupProbeFailures = nil
	workspace.Status.EarliestNextProbeTime = nil
}

func versionedAccessStrategyID(as *workspacev1alpha1.WorkspaceAccessStrategy) string {
	return fmt.Sprintf("%s.%d", as.UID, as.Generation)
}

func probeRetrySeconds(periodSeconds int32, failures int32, failureThreshold int32) int32 {
	backoffStart := max(failureThreshold-ProbeBackoffThreshold, 0)
	if failures <= backoffStart {
		return periodSeconds
	}
	backoff := periodSeconds
	for i := int32(0); i < failures-backoffStart; i++ {
		backoff *= 2
		if backoff >= ProbeBackoffMaxRetrySeconds {
			return ProbeBackoffMaxRetrySeconds
		}
	}
	return backoff
}

func timeUntilProbeDeadline(workspace *workspacev1alpha1.Workspace) time.Duration {
	if workspace.Status.EarliestNextProbeTime == nil {
		return 0
	}
	remaining := time.Until(workspace.Status.EarliestNextProbeTime.Time)
	if remaining > 0 {
		return remaining
	}
	return 0
}
