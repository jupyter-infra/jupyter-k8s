/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"
	"unicode/utf8"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// maxProbeMessageBytes bounds the probe verdict message (built from the command's stderr/stdout)
// before it is written to a condition. The CRD caps conditions[].message at 32768 bytes, so a
// chatty probe that dumps kilobytes to stderr would otherwise fail the status write outright; we
// keep it small (a few KB is plenty to diagnose a failure) and well under the CRD ceiling.
const maxProbeMessageBytes = 2048

// The probe execs into the primary workspace container (PrimaryContainerName, defined in
// constants.go) so it observes the pod's real network/auth context -- mirroring the idle detector --
// regardless of which sidecars an integration injects. Not author-selectable. Re-probes happen on a
// flat operator cadence (ProbePeriod, from --integration-probe-period), the same way the idle checker
// requeues on a flat CheckInterval -- there is no per-verdict backoff, since the probe is report-only.

// PodExecWithStderr is the exec dependency (satisfied by *PodExecUtil), injectable for tests.
type PodExecWithStderr interface {
	ExecInPodWithStderr(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (string, string, error)
}

// IntegrationProberInterface allows mocking in tests. FindRunningPod is called once per reconcile and
// the pod is passed into each Probe, so N integrations cost one pod lookup, not N.
type IntegrationProberInterface interface {
	FindRunningPod(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.Pod, error)
	Probe(ctx context.Context, pod *corev1.Pod, integrationName string, probe *workspacev1alpha1.IntegrationStatusProbe) workspacev1alpha1.IntegrationStatus
	// ProbePeriod is the operator-level base re-probe cadence used to schedule the next reconcile.
	ProbePeriod() time.Duration
}

// IntegrationProber execs the integration's statusProbe in the workspace container and returns a
// report-only verdict. No WorkspaceIntegration object is involved -- the probe spec is resolved
// inline from the template (same source as the sidecar) and passed in directly.
type IntegrationProber struct {
	client   client.Client
	execUtil PodExecWithStderr
	// probePeriod is the operator-level base re-probe cadence (from --integration-probe-period). Held
	// here -- mirroring WorkspaceIdleChecker.checkInterval -- so this component owns its own cadence.
	probePeriod time.Duration
}

// NewIntegrationProber creates a new IntegrationProber. probePeriod is the base re-probe cadence: a
// non-positive value falls back to DefaultIntegrationProbePeriod, and a positive value below
// MinIntegrationProbePeriod is clamped up to that floor (mirrors NewWorkspaceIdleChecker).
func NewIntegrationProber(c client.Client, execUtil PodExecWithStderr, probePeriod time.Duration) *IntegrationProber {
	switch {
	case probePeriod <= 0:
		probePeriod = DefaultIntegrationProbePeriod
	case probePeriod < MinIntegrationProbePeriod:
		probePeriod = MinIntegrationProbePeriod
	}
	return &IntegrationProber{client: c, execUtil: execUtil, probePeriod: probePeriod}
}

// ProbePeriod returns the base re-probe cadence (mirrors WorkspaceIdleChecker.CheckInterval).
func (p *IntegrationProber) ProbePeriod() time.Duration {
	return p.probePeriod
}

var _ IntegrationProberInterface = &IntegrationProber{}

// Probe execs the integration's statusProbe command in the given workspace pod and returns the verdict.
// The caller resolves the pod once (FindRunningPod) and invokes this only for an integration that
// declares an Exec statusProbe (see probeIntegrationStatus). The probe is report-only: it never gates
// pod readiness or restarts the pod.
func (p *IntegrationProber) Probe(
	ctx context.Context,
	pod *corev1.Pod,
	integrationName string,
	probe *workspacev1alpha1.IntegrationStatusProbe,
) workspacev1alpha1.IntegrationStatus {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "integration", integrationName)

	// Defensive: getIntegrationStatusProbeSpec already returns nil (so this integration is skipped) for a
	// probe with no exec transport, but guard here too so a probe that reaches this point with an empty
	// Exec.Command reports a non-failing error rather than panicking on probe.Exec.Command below.
	if probe.Exec == nil || len(probe.Exec.Command) == 0 {
		return buildIntegrationStatus(integrationName, false, IntegrationReasonProbeError,
			"integration statusProbe has no exec command to run")
	}

	timeout := time.Duration(resolveIntegrationProbeTimeout(probe)) * time.Second
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdout, stderr, execErr := p.execUtil.ExecInPodWithStderr(
		probeCtx, pod, PrimaryContainerName, probe.Exec.Command, "")
	if execErr != nil {
		msg := truncateProbeMessage(firstNonEmpty(stderr, stdout, execErr.Error()))
		// Distinguish a genuine unhealthy verdict from an infra hiccup so the reported reason is accurate:
		//   - A transient/infra error (context deadline/cancellation, a network dial/timeout, or an API
		//     transport failure) means we never got a command verdict -> ProbeError.
		//   - A real non-zero command exit (an exec.CodeExitError, or any other error we can't classify)
		//     is a true "integration unhealthy" verdict -> ProbeFailed.
		// The classification sets only the reported reason (and the log line); it does not change the
		// requeue cadence -- every verdict re-probes on the same flat ProbePeriod (see probeIntegrationStatus).
		if isTransientProbeError(execErr) {
			logger.V(1).Info("Integration status probe errored (transient)", "reason", IntegrationReasonProbeError, "message", msg)
			return buildIntegrationStatus(integrationName, false, IntegrationReasonProbeError, msg)
		}
		logger.V(1).Info("Integration status probe failed", "reason", IntegrationReasonProbeFailed, "message", msg)
		return buildIntegrationStatus(integrationName, false, IntegrationReasonProbeFailed, msg)
	}

	return buildIntegrationStatus(integrationName, true, IntegrationReasonReady, "")
}

// isTransientProbeError reports whether an ExecInPodWithStderr error is an infrastructure/transport
// failure (we never obtained a command verdict) rather than a genuine non-zero command exit. It selects
// the reported reason only (ProbeError vs ProbeFailed), not the requeue cadence. A non-zero exit surfaces
// as an exec.CodeExitError (which reports Exited()==true), so anything satisfying that interface -- or any
// error not otherwise recognized as transport-level -- is treated as a real command failure.
func isTransientProbeError(err error) bool {
	if err == nil {
		return false
	}
	// A command that ran and exited non-zero is NOT transient, regardless of any wrapped context error.
	var exitErr exitStatusError
	if errors.As(err, &exitErr) && exitErr.Exited() {
		return false
	}
	// Context deadline/cancellation: the probe timed out or the reconcile was cancelled mid-exec.
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return true
	}
	// Network-level failure reaching the apiserver/exec stream (dial, timeout, reset).
	var netErr net.Error
	return errors.As(err, &netErr)
}

// exitStatusError matches k8s.io/client-go/util/exec.ExitError (and CodeExitError) without importing the
// package: a non-zero command exit satisfies Exited(). Kept as a local interface so the classifier does
// not take a new dependency just to type-assert.
type exitStatusError interface {
	error
	Exited() bool
	ExitStatus() int
}

// truncateProbeMessage bounds a probe verdict message to maxProbeMessageBytes on a rune boundary, so it
// can never blow past the CRD conditions[].message ceiling (32768) and fail the status write. A "(truncated)"
// marker is appended when the message is clipped so a reader knows the tail was dropped.
func truncateProbeMessage(msg string) string {
	if len(msg) <= maxProbeMessageBytes {
		return msg
	}
	clipped := msg[:maxProbeMessageBytes]
	// Back off to the last valid UTF-8 boundary so we never split a multi-byte rune.
	for len(clipped) > 0 && !utf8.ValidString(clipped) {
		clipped = clipped[:len(clipped)-1]
	}
	return clipped + " ...(truncated)"
}

// buildIntegrationStatus builds a KRO-style IntegrationStatus: a coarse State plus a single "Ready"
// condition carrying the machine-readable reason and human-readable message. LastTransitionTime is
// stamped now (the CRD schema requires it); callers that re-probe on a cadence should preserve the
// prior timestamp when the condition has not materially changed (see preserveConditionTimestamp) so
// an unchanged verdict does not churn the status on every probe.
func buildIntegrationStatus(name string, ready bool, reason, message string) workspacev1alpha1.IntegrationStatus {
	condStatus := metav1.ConditionFalse
	state := IntegrationStateDegraded
	if ready {
		condStatus = metav1.ConditionTrue
		state = IntegrationStateReady
	}
	return workspacev1alpha1.IntegrationStatus{
		Name:  name,
		State: state,
		Conditions: []metav1.Condition{{
			Type:               IntegrationConditionTypeReady,
			Status:             condStatus,
			Reason:             reason,
			Message:            message,
			LastTransitionTime: metav1.Now(),
		}},
	}
}

// preserveConditionTimestamp copies the prior Ready-condition LastTransitionTime onto next when the
// verdict is unchanged (same Status and Reason), so a report-only re-probe does not churn the timestamp
// every cadence. Message is deliberately EXCLUDED from the comparison: a failing probe's message carries
// stderr, which can vary run-to-run (e.g. a timestamped error) without the verdict changing, so it must
// not count as a change that resets the transition time.
func preserveConditionTimestamp(prior, next *workspacev1alpha1.IntegrationStatus) {
	if prior == nil || next == nil || len(prior.Conditions) == 0 || len(next.Conditions) == 0 {
		return
	}
	p, n := prior.Conditions[0], &next.Conditions[0]
	if p.Status == n.Status && p.Reason == n.Reason {
		n.LastTransitionTime = p.LastTransitionTime
	}
}

// integrationEvent describes a Kubernetes Event to record for an integration status transition. A zero
// value (Type == "") means "no event": the verdict did not transition in a way worth recording.
type integrationEvent struct {
	Type    string // corev1.EventTypeNormal or corev1.EventTypeWarning
	Reason  string // IntegrationEventDegraded / IntegrationEventRecovered
	Message string
}

// getIntegrationStatusEvent decides whether an integration's probe-verdict change warrants a Kubernetes
// Event on the Workspace, and of what kind. It is EDGE-TRIGGERED to avoid event spam on the flat
// report-only probe cadence: an event fires only on a verdict TRANSITION, never for an unchanged re-probe.
//
//   - newly-seen not-ready (no prior), Ready->not-ready, or a reason change while still not-ready
//     (e.g. PodNotFound->ProbeFailed) -> Warning IntegrationDegraded.
//   - not-ready -> Ready -> Normal IntegrationRecovered.
//   - an unchanged verdict (same condition Status+Reason), or a healthy first attach (no prior, Ready)
//     -> no event: a steady-state healthy integration is silent.
//
// Message is deliberately excluded from the change test (mirrors preserveConditionTimestamp): a failing
// probe's stderr can vary run-to-run without the verdict changing, and must not trigger a fresh event.
func getIntegrationStatusEvent(name string, prior, next *workspacev1alpha1.IntegrationStatus) integrationEvent {
	if next == nil || len(next.Conditions) == 0 {
		return integrationEvent{}
	}
	nextCond := next.Conditions[0]
	nextReady := nextCond.Status == metav1.ConditionTrue

	var hadPrior, priorReady bool
	var priorReason string
	if prior != nil && len(prior.Conditions) > 0 {
		hadPrior = true
		priorReady = prior.Conditions[0].Status == metav1.ConditionTrue
		priorReason = prior.Conditions[0].Reason
	}

	if nextReady {
		// Announce a RECOVERY only (was not-ready -> now Ready). A healthy first attach (no prior) or a
		// steady-state Ready re-probe stays silent.
		if hadPrior && !priorReady {
			return integrationEvent{
				Type:    corev1.EventTypeNormal,
				Reason:  IntegrationEventRecovered,
				Message: fmt.Sprintf("integration %q is ready", name),
			}
		}
		return integrationEvent{}
	}

	// next is not ready. Skip an unchanged not-ready verdict (same reason); emit on newly-seen, a
	// transition from Ready, or a reason change while still not-ready.
	if hadPrior && !priorReady && priorReason == nextCond.Reason {
		return integrationEvent{}
	}
	msg := fmt.Sprintf("integration %q is not ready (%s)", name, nextCond.Reason)
	if nextCond.Message != "" {
		msg = fmt.Sprintf("%s: %s", msg, nextCond.Message)
	}
	return integrationEvent{
		Type:    corev1.EventTypeWarning,
		Reason:  IntegrationEventDegraded,
		Message: msg,
	}
}

// FindRunningPod locates a running pod for the workspace by its labels (mirrors the idle checker).
func (p *IntegrationProber) FindRunningPod(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.Pod, error) {
	podList := &corev1.PodList{}
	if err := p.client.List(ctx, podList,
		client.InNamespace(workspace.Namespace),
		client.MatchingLabels(GenerateLabels(workspace.Name)),
	); err != nil {
		return nil, fmt.Errorf("failed to list pods: %w", err)
	}
	for i := range podList.Items {
		// Skip a terminating pod: during a Recreate rollout the old pod can still report Running while it
		// is being torn down, and probing it would yield a stale verdict for the incoming pod.
		if podList.Items[i].Status.Phase == corev1.PodRunning && podList.Items[i].DeletionTimestamp == nil {
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// getIntegrationStatusProbeSpec returns the template's statusProbe with its exec command resolved from
// the FROZEN values (nil if the template declares none). Reads the template, not the referenced resource,
// so the freeze contract holds. The command carries the same {{ resource }} / {{ .Parameters }}
// expressions as the sidecar fields, so it must be rendered from the frozen record before it is exec'd --
// otherwise the probe runs against literal "{{ resource ... }}" text. A missing template errors so the
// caller reports not-ready rather than skip.
func (rm *ResourceManager) getIntegrationStatusProbeSpec(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	ref *workspacev1alpha1.IntegrationTemplateRef,
) (*workspacev1alpha1.IntegrationStatusProbe, error) {
	template, err := getIntegrationTemplate(ctx, rm.client, workspace, ref)
	if err != nil {
		return nil, err
	}
	if template.Spec.StatusProbe == nil {
		return nil, nil
	}
	frozen := findResolvedIntegration(&workspace.Status, ref.Name)
	if frozen == nil {
		// Caller gates on this: an unresolved integration is reported NotResolved before we get here.
		return nil, fmt.Errorf("integration %q has no frozen values to resolve its statusProbe command", ref.Name)
	}
	resolver := NewIntegrationTemplateResolver(NewFrozenResourceValueProvider(frozen.Values))
	resolved, err := resolveStatusProbeCommand(resolver, template.Spec.StatusProbe, buildIntegrationTemplateData(workspace, ref))
	if err != nil {
		return nil, err
	}
	// Exec is the only transport today, so a resolved probe with no exec command has nothing to run.
	// Return nil (no probe) rather than let an exec-less probe flow downstream and nil-deref in Probe.
	if resolved == nil || resolved.Exec == nil || len(resolved.Exec.Command) == 0 {
		return nil, nil
	}
	return resolved, nil
}
