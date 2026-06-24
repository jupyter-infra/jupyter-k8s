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
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Integration readiness probe reasons (machine-readable, CamelCase per k8s convention).
const (
	// IntegrationReasonReady indicates the readiness probe succeeded.
	IntegrationReasonReady = "Ready"
	// IntegrationReasonProbeFailed indicates the readiness command exited non-zero.
	IntegrationReasonProbeFailed = "ProbeFailed"
	// IntegrationReasonPodNotFound indicates no running workspace pod was found to probe.
	IntegrationReasonPodNotFound = "PodNotFound"
	// IntegrationReasonProbeError indicates the probe could not be executed (transport error).
	IntegrationReasonProbeError = "ProbeError"
	// IntegrationReasonResolving indicates the WorkspaceIntegration child exists but has not yet
	// been resolved by the mutating webhook (or has not been created), so there is nothing to probe
	// this cycle. Surfaced as a not-ready entry rather than omitting the integration from status.
	IntegrationReasonResolving = "Resolving"

	// integrationProbeWorkspaceContainer is the container the probe execs into. The probe
	// runs in the workspace container so it observes the pod's real network/auth context
	// (mirrors the idle detector), regardless of which sidecars an integration injects.
	integrationProbeWorkspaceContainer = "workspace"

	// DefaultIntegrationProbeTimeoutSeconds bounds a single exec attempt.
	DefaultIntegrationProbeTimeoutSeconds = 5

	// DefaultIntegrationProbePeriodSeconds is the default cadence at which the operator
	// re-runs the integration readiness probe on a Running workspace.
	DefaultIntegrationProbePeriodSeconds = 30
)

// ResolveIntegrationProbePeriod returns the configured probe period (seconds) as a Duration,
// defaulting when unset. Used by the reconcile loop to requeue a Running workspace so the
// probe re-runs periodically and detects integration staleness without a watch.
func ResolveIntegrationProbePeriod(probe *workspacev1alpha1.IntegrationStatusProbe) time.Duration {
	if probe != nil && probe.PeriodSeconds > 0 {
		return time.Duration(probe.PeriodSeconds) * time.Second
	}
	return time.Duration(DefaultIntegrationProbePeriodSeconds) * time.Second
}

// IntegrationProberInterface allows mocking in tests.
type IntegrationProberInterface interface {
	Probe(
		ctx context.Context,
		workspace *workspacev1alpha1.Workspace,
		integrationName string,
		wi *workspacev1alpha1.WorkspaceIntegration,
	) workspacev1alpha1.IntegrationStatus
}

// IntegrationProber runs an integration's readiness probe and produces a single readiness
// verdict for workspace.status.integrations[]. It is a structural sibling of
// WorkspaceIdleChecker: it locates the workspace pod by label and execs in the workspace
// container. The probe is non-gating — it reports health into status and never restarts
// the pod.
type IntegrationProber struct {
	client   client.Client
	execUtil PodExecWithStderr
}

// PodExecWithStderr is the subset of PodExecUtil the prober needs. Defined here (consumer
// side) so the prober can be unit-tested with a fake exec implementation.
type PodExecWithStderr interface {
	ExecInPodWithStderr(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (stdout, stderr string, err error)
}

// NewIntegrationProber creates a new IntegrationProber.
func NewIntegrationProber(k8sClient client.Client, execUtil PodExecWithStderr) *IntegrationProber {
	return &IntegrationProber{client: k8sClient, execUtil: execUtil}
}

// Probe runs the integration's frozen statusProbe (wi.spec.statusProbe, if any) against
// the workspace pod and returns the resulting per-integration status. It reads ONLY the frozen
// WorkspaceIntegration child -- it never fetches the WorkspaceIntegrationTemplate at reconcile time.
//
// Today only the Exec handler is implemented. integrationName (the integrationRef name) is used as
// IntegrationStatus.Name so it keys the status.integrations[] listMap consistently with what the
// user authored.
func (p *IntegrationProber) Probe(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	integrationName string,
	wi *workspacev1alpha1.WorkspaceIntegration,
) workspacev1alpha1.IntegrationStatus {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name, "integration", integrationName)

	status := workspacev1alpha1.IntegrationStatus{Name: integrationName}

	var probe *workspacev1alpha1.IntegrationStatusProbe
	if wi != nil {
		probe = wi.Spec.StatusProbe
	}
	if probe == nil || probe.Exec == nil || len(probe.Exec.Command) == 0 {
		// Only the Exec handler is implemented today. A probe without it is a no-op that
		// reports ready (nothing to check) rather than failing the integration.
		status.Ready = true
		status.Reason = IntegrationReasonReady
		return status
	}

	pod, err := p.findWorkspacePod(ctx, workspace)
	if err != nil {
		logger.V(1).Info("Integration probe: no running workspace pod", "error", err)
		status.Ready = false
		status.Reason = IntegrationReasonPodNotFound
		status.Message = err.Error()
		return status
	}

	timeout := time.Duration(resolveIntegrationProbeTimeout(probe)) * time.Second
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdout, stderr, execErr := p.execUtil.ExecInPodWithStderr(
		probeCtx, pod, integrationProbeWorkspaceContainer, probe.Exec.Command, "")

	if execErr != nil {
		status.Ready = false
		status.Reason = IntegrationReasonProbeFailed
		status.Message = firstNonEmpty(stderr, stdout, execErr.Error())
		logger.V(1).Info("Integration readiness probe failed",
			"reason", status.Reason, "message", status.Message)
		return status
	}

	status.Ready = true
	status.Reason = IntegrationReasonReady
	logger.V(1).Info("Integration readiness probe succeeded")
	return status
}

// findWorkspacePod locates a running pod for the workspace by its labels. Mirrors
// WorkspaceIdleChecker.findWorkspacePod so the prober shares the proven lookup path.
func (p *IntegrationProber) findWorkspacePod(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.Pod, error) {
	podList := &corev1.PodList{}
	labels := GenerateLabels(workspace.Name)

	if err := p.client.List(ctx, podList, client.InNamespace(workspace.Namespace), client.MatchingLabels(labels)); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}

	for i := range podList.Items {
		if podList.Items[i].Status.Phase == corev1.PodRunning {
			return &podList.Items[i], nil
		}
	}

	return nil, fmt.Errorf("no running pod found for workspace")
}

func resolveIntegrationProbeTimeout(probe *workspacev1alpha1.IntegrationStatusProbe) int32 {
	if probe.TimeoutSeconds > 0 {
		return probe.TimeoutSeconds
	}
	return DefaultIntegrationProbeTimeoutSeconds
}

// firstNonEmpty returns the first non-empty string, used to prefer stderr > stdout > error
// text when building the status message.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
