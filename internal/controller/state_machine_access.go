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
			now := metav1.Now()
			workspace.Status.LastAccessStartupProbeTime = &now
			return &ProbeResult{
				Status:       ProbeRetrying,
				RequeueAfter: time.Duration(probe.InitialDelaySeconds) * time.Second,
			}, nil
		}
	}

	// Enforce minimum spacing between probes. Status updates trigger
	// watch events that re-reconcile faster than RequeueAfter; skip
	// the actual HTTP probe until enough time has elapsed.
	if remaining := timeUntilNextProbe(workspace, periodSeconds); remaining > 0 {
		return &ProbeResult{
			Status:       ProbePendingRetry,
			RequeueAfter: remaining,
		}, nil
	}

	now := metav1.Now()
	workspace.Status.LastAccessStartupProbeTime = &now
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
		workspace.Status.LastAccessStartupProbeTime = nil
		return &ProbeResult{Status: ProbeFailureThresholdExceeded}, nil
	}

	logger.Info("Access startup probe failed, retrying",
		"failures", failures, "failureThreshold", failureThreshold,
		"retryAfterSeconds", periodSeconds)
	return &ProbeResult{
		Status:       ProbeRetrying,
		RequeueAfter: time.Duration(periodSeconds) * time.Second,
	}, nil
}

func clearProbeState(workspace *workspacev1alpha1.Workspace) {
	workspace.Status.AccessStartupProbeFailures = nil
	workspace.Status.LastAccessStartupProbeTime = nil
}

func timeUntilNextProbe(workspace *workspacev1alpha1.Workspace, periodSeconds int32) time.Duration {
	if workspace.Status.LastAccessStartupProbeTime == nil {
		return 0
	}
	elapsed := time.Since(workspace.Status.LastAccessStartupProbeTime.Time)
	period := time.Duration(periodSeconds) * time.Second
	if elapsed < period {
		return period - elapsed
	}
	return 0
}
