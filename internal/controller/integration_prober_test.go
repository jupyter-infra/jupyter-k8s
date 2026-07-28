/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// intReadyName is the repeated "happy path" integration name in these prober specs, extracted to
// satisfy goconst. The default namespace reuses the package-level testNamespace const.
const intReadyName = "int-ready"

// probeFailMsg is the sample probe stderr reused across specs, hoisted to satisfy goconst.
const probeFailMsg = "cannot connect to GCS"

// fakeExec implements PodExecWithStderr for unit testing the prober verdicts.
type fakeExec struct {
	stdout, stderr string
	err            error
}

func (f *fakeExec) ExecInPodWithStderr(_ context.Context, _ *corev1.Pod, _ string, _ []string, _ string) (string, string, error) {
	return f.stdout, f.stderr, f.err
}

func runningWSPod(ns, wsName string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: wsName + "-pod", Namespace: ns, Labels: GenerateLabels(wsName)},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func execProbe() *workspacev1alpha1.IntegrationStatusProbe {
	return &workspacev1alpha1.IntegrationStatusProbe{Exec: &corev1.ExecAction{Command: []string{"ray", "status"}}}
}

func TestIntegrationProber_Verdicts(t *testing.T) {
	ns, ws := testNamespace, "p-ws"
	pod := runningWSPod(ns, ws)

	cases := []struct {
		name       string
		exec       *fakeExec
		wantReady  bool
		wantReason string
	}{
		{"probe exits zero -> ready", &fakeExec{stdout: "ok"}, true, IntegrationReasonReady},
		{"probe exits nonzero -> degraded", &fakeExec{stderr: probeFailMsg, err: fmt.Errorf("exit 1")}, false, IntegrationReasonProbeFailed},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prober := NewIntegrationProber(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), tc.exec, DefaultIntegrationProbePeriod)
			st := prober.Probe(context.Background(), pod, "ray-integration", execProbe())

			// KRO-style status: assert State + the single Ready condition (type/status/reason).
			wantState := IntegrationStateReady
			wantCondStatus := metav1.ConditionTrue
			if !tc.wantReady {
				wantState = IntegrationStateDegraded
				wantCondStatus = metav1.ConditionFalse
			}
			if st.State != wantState {
				t.Errorf("%s: got state=%q, want %q", tc.name, st.State, wantState)
			}
			cond := findReadyCond(st.Conditions)
			if cond == nil {
				t.Fatalf("%s: missing Ready condition", tc.name)
			}
			if cond.Status != wantCondStatus || cond.Reason != tc.wantReason {
				t.Errorf("%s: got cond status=%v reason=%q, want status=%v reason=%q",
					tc.name, cond.Status, cond.Reason, wantCondStatus, tc.wantReason)
			}
			if !tc.wantReady && cond.Message != probeFailMsg {
				t.Errorf("%s: want stderr in condition message, got %q", tc.name, cond.Message)
			}
		})
	}
}

// TestIntegrationProber_FindRunningPod covers the pod lookup the caller does once per reconcile: a
// running pod is returned; a terminating pod (DeletionTimestamp set) is skipped; none present errors.
func TestIntegrationProber_FindRunningPod(t *testing.T) {
	ns, ws := testNamespace, "p-ws"
	workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: ws, Namespace: ns}}

	t.Run("running pod found", func(t *testing.T) {
		p := NewIntegrationProber(fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(runningWSPod(ns, ws)).Build(), &fakeExec{}, DefaultIntegrationProbePeriod)
		if _, err := p.FindRunningPod(context.Background(), workspace); err != nil {
			t.Fatalf("expected a running pod, got err %v", err)
		}
	})

	t.Run("no pod -> error", func(t *testing.T) {
		p := NewIntegrationProber(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), &fakeExec{}, DefaultIntegrationProbePeriod)
		if _, err := p.FindRunningPod(context.Background(), workspace); err == nil {
			t.Fatal("expected an error when no running pod exists")
		}
	})

	t.Run("terminating pod skipped -> error", func(t *testing.T) {
		// A Running pod being torn down (DeletionTimestamp set) must not be probed. A finalizer keeps the
		// fake client from dropping the deletion timestamp.
		term := runningWSPod(ns, ws)
		now := metav1.Now()
		term.DeletionTimestamp = &now
		term.Finalizers = []string{"test/keep"}
		p := NewIntegrationProber(fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(term).Build(), &fakeExec{}, DefaultIntegrationProbePeriod)
		if _, err := p.FindRunningPod(context.Background(), workspace); err == nil {
			t.Fatal("expected an error: a terminating pod must be skipped")
		}
	})
}

func TestNewIntegrationProber_ClampsProbePeriod(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
	// Non-positive -> default.
	if got := NewIntegrationProber(c, &fakeExec{}, 0).ProbePeriod(); got != DefaultIntegrationProbePeriod {
		t.Errorf("zero period: want default %v, got %v", DefaultIntegrationProbePeriod, got)
	}
	// Below the floor -> raised to the floor.
	if got := NewIntegrationProber(c, &fakeExec{}, time.Nanosecond).ProbePeriod(); got != MinIntegrationProbePeriod {
		t.Errorf("tiny period: want floor %v, got %v", MinIntegrationProbePeriod, got)
	}
	// A sensible explicit value is kept as-is.
	if got := NewIntegrationProber(c, &fakeExec{}, 2*time.Minute).ProbePeriod(); got != 2*time.Minute {
		t.Errorf("explicit period: want 2m, got %v", got)
	}
}

// findReadyCond returns the integration Ready condition (the only condition these specs assert on).
func findReadyCond(conds []metav1.Condition) *metav1.Condition {
	for i := range conds {
		if conds[i].Type == IntegrationConditionTypeReady {
			return &conds[i]
		}
	}
	return nil
}

// mockIntegrationProber stubs IntegrationProberInterface so the state-machine driver
// (probeIntegrationStatus) can be tested without a real pod or exec. It records how many times the pod
// was looked up and which pod each Probe received, so the "one lookup, shared pod" contract is assertable.
type mockIntegrationProber struct {
	pod         *corev1.Pod
	podErr      error
	probeByName map[string]workspacev1alpha1.IntegrationStatus
	findCalls   int
	probedNames []string
	probedPods  []*corev1.Pod
	probePeriod time.Duration
}

func (m *mockIntegrationProber) FindRunningPod(_ context.Context, _ *workspacev1alpha1.Workspace) (*corev1.Pod, error) {
	m.findCalls++
	return m.pod, m.podErr
}

// ProbePeriod returns the configured period, or the default when unset -- so existing driver tests that
// don't care about cadence need not set it.
func (m *mockIntegrationProber) ProbePeriod() time.Duration {
	if m.probePeriod <= 0 {
		return DefaultIntegrationProbePeriod
	}
	return m.probePeriod
}

func (m *mockIntegrationProber) Probe(_ context.Context, pod *corev1.Pod, name string, _ *workspacev1alpha1.IntegrationStatusProbe) workspacev1alpha1.IntegrationStatus {
	m.probedNames = append(m.probedNames, name)
	m.probedPods = append(m.probedPods, pod)
	if st, ok := m.probeByName[name]; ok {
		return st
	}
	return buildIntegrationStatus(name, true, IntegrationReasonReady, "")
}

func proberTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = workspacev1alpha1.AddToScheme(s)
	return s
}

// proberResourceManager builds a ResourceManager backed by a fake client seeded with the given template
// objects, so getIntegrationStatusProbeSpec can read (or fail to find) a template like it does in prod.
func proberResourceManager(objs ...client.Object) *ResourceManager {
	c := fake.NewClientBuilder().WithScheme(proberTestScheme()).WithObjects(objs...).Build()
	return NewResourceManager(
		c, c.Scheme(),
		NewDeploymentBuilder(c.Scheme(), WorkspaceControllerOptions{}),
		NewServiceBuilder(c.Scheme()),
		NewPVCBuilder(c.Scheme()),
		NewAccessResourcesBuilder(),
		NewStatusManager(c),
	)
}

func probeTemplate(name, ns string, withProbe bool) *workspacev1alpha1.WorkspaceIntegrationTemplate {
	tmpl := &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
	}
	if withProbe {
		tmpl.Spec.StatusProbe = execProbe()
	}
	return tmpl
}

func ref(name string) workspacev1alpha1.IntegrationTemplateRef {
	return workspacev1alpha1.IntegrationTemplateRef{Name: name}
}

func integrationStatusByName(sts []workspacev1alpha1.IntegrationStatus, name string) *workspacev1alpha1.IntegrationStatus {
	for i := range sts {
		if sts[i].Name == name {
			return &sts[i]
		}
	}
	return nil
}

// TestProbeIntegrationStatus_Driver covers the per-integration branch matrix in probeIntegrationStatus
// (state_machine.go): the pod is looked up ONCE for all integrations, and each ref lands in exactly one
// of ready / not-resolved / no-probe-skip / probe-error, plus PodNotFound and the empty-refs clear.
func TestProbeIntegrationStatus_Driver(t *testing.T) {
	ns, wsName := testNamespace, "driver-ws"

	// Four refs exercising four branches; frozen records exist for all but int-notresolved.
	newWorkspace := func() *workspacev1alpha1.Workspace {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: wsName, Namespace: ns},
			Spec: workspacev1alpha1.WorkspaceSpec{
				IntegrationTemplateRefs: []workspacev1alpha1.IntegrationTemplateRef{
					ref(intReadyName), ref("int-notresolved"), ref("int-noprobe"), ref("int-probeerror"),
				},
			},
		}
		ws.Status.ResolvedIntegrations = []workspacev1alpha1.ResolvedIntegration{
			{Name: intReadyName}, {Name: "int-noprobe"}, {Name: "int-probeerror"},
		}
		return ws
	}

	// Templates: int-ready has a probe, int-noprobe has none, int-probeerror is intentionally absent.
	rm := proberResourceManager(
		probeTemplate(intReadyName, ns, true),
		probeTemplate("int-noprobe", ns, false),
	)
	pod := runningWSPod(ns, wsName)
	mock := &mockIntegrationProber{pod: pod}
	sm := &StateMachine{resourceManager: rm, integrationProber: mock}

	ws := newWorkspace()
	sm.probeIntegrationStatus(context.Background(), ws)
	got := ws.Status.IntegrationStatuses

	// int-noprobe resolves but declares no probe -> no entry. The other three each produce one entry.
	if len(got) != 3 {
		t.Fatalf("want 3 status entries (ready/notresolved/probeerror), got %d: %+v", len(got), got)
	}
	assertReason := func(name, wantState, wantReason string) {
		s := integrationStatusByName(got, name)
		if s == nil {
			t.Fatalf("%s: missing status entry", name)
		}
		cond := findReadyCond(s.Conditions)
		if cond == nil {
			t.Fatalf("%s: missing Ready condition", name)
		}
		if s.State != wantState || cond.Reason != wantReason {
			t.Errorf("%s: got state=%q reason=%q, want state=%q reason=%q", name, s.State, cond.Reason, wantState, wantReason)
		}
	}
	assertReason(intReadyName, IntegrationStateReady, IntegrationReasonReady)
	assertReason("int-notresolved", IntegrationStateDegraded, IntegrationReasonNotResolved)
	assertReason("int-probeerror", IntegrationStateDegraded, IntegrationReasonProbeError)
	if integrationStatusByName(got, "int-noprobe") != nil {
		t.Error("int-noprobe declares no statusProbe and must not produce a status entry")
	}

	// Pod looked up ONCE for all four refs; only int-ready reaches Probe (and gets that shared pod).
	if mock.findCalls != 1 {
		t.Errorf("want exactly 1 pod lookup for N integrations, got %d", mock.findCalls)
	}
	if len(mock.probedNames) != 1 || mock.probedNames[0] != intReadyName {
		t.Errorf("want only int-ready probed, got %v", mock.probedNames)
	}
	if len(mock.probedPods) == 1 && mock.probedPods[0] != pod {
		t.Error("Probe must receive the pod resolved by the single FindRunningPod call")
	}

	t.Run("pod not found -> all resolved integrations report PodNotFound, no exec", func(t *testing.T) {
		rm := proberResourceManager(probeTemplate(intReadyName, ns, true))
		mock := &mockIntegrationProber{podErr: fmt.Errorf("no running pod found for workspace")}
		sm := &StateMachine{resourceManager: rm, integrationProber: mock}
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: wsName, Namespace: ns},
			Spec:       workspacev1alpha1.WorkspaceSpec{IntegrationTemplateRefs: []workspacev1alpha1.IntegrationTemplateRef{ref(intReadyName)}},
		}
		ws.Status.ResolvedIntegrations = []workspacev1alpha1.ResolvedIntegration{{Name: intReadyName}}

		sm.probeIntegrationStatus(context.Background(), ws)

		s := integrationStatusByName(ws.Status.IntegrationStatuses, intReadyName)
		if s == nil || s.State != IntegrationStateDegraded {
			t.Fatalf("want Degraded int-ready, got %+v", s)
		}
		if cond := findReadyCond(s.Conditions); cond == nil || cond.Reason != IntegrationReasonPodNotFound {
			t.Errorf("want reason=%q, got %+v", IntegrationReasonPodNotFound, cond)
		}
		if len(mock.probedNames) != 0 {
			t.Errorf("no exec must be attempted when the pod is missing, probed=%v", mock.probedNames)
		}
	})

	t.Run("no integrations clears stale status and returns 0", func(t *testing.T) {
		rm := proberResourceManager()
		sm := &StateMachine{resourceManager: rm, integrationProber: &mockIntegrationProber{}}
		ws := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: wsName, Namespace: ns}}
		ws.Status.IntegrationStatuses = []workspacev1alpha1.IntegrationStatus{{Name: "stale"}}

		if d := sm.probeIntegrationStatus(context.Background(), ws); d != 0 {
			t.Errorf("no integrations: want 0 requeue, got %v", d)
		}
		if ws.Status.IntegrationStatuses != nil {
			t.Errorf("no integrations: stale status must be cleared, got %+v", ws.Status.IntegrationStatuses)
		}
	})

	t.Run("unchanged verdict preserves prior LastTransitionTime", func(t *testing.T) {
		rm := proberResourceManager(probeTemplate(intReadyName, ns, true))
		mock := &mockIntegrationProber{pod: runningWSPod(ns, wsName)}
		sm := &StateMachine{resourceManager: rm, integrationProber: mock}
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: wsName, Namespace: ns},
			Spec:       workspacev1alpha1.WorkspaceSpec{IntegrationTemplateRefs: []workspacev1alpha1.IntegrationTemplateRef{ref(intReadyName)}},
		}
		ws.Status.ResolvedIntegrations = []workspacev1alpha1.ResolvedIntegration{{Name: intReadyName}}
		old := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
		ws.Status.IntegrationStatuses = []workspacev1alpha1.IntegrationStatus{{
			Name:  intReadyName,
			State: IntegrationStateReady,
			Conditions: []metav1.Condition{{
				Type: IntegrationConditionTypeReady, Status: metav1.ConditionTrue,
				Reason: IntegrationReasonReady, LastTransitionTime: old,
			}},
		}}

		sm.probeIntegrationStatus(context.Background(), ws)

		s := integrationStatusByName(ws.Status.IntegrationStatuses, intReadyName)
		cond := findReadyCond(s.Conditions)
		if !cond.LastTransitionTime.Equal(&old) {
			t.Errorf("unchanged Ready verdict must keep prior timestamp %v, got %v", old, cond.LastTransitionTime)
		}
	})
}

func TestGetShorterInterval(t *testing.T) {
	m, h := 5*time.Minute, 10*time.Minute
	cases := []struct {
		a, b, want time.Duration
	}{
		{0, m, m}, // idle unset -> take the probe cadence (NOT 0, which would cancel the timer)
		{m, 0, m}, // probe unset -> keep idle's
		{0, 0, 0}, // neither wants a timer -> no requeue
		{h, m, m}, // both set -> smaller wins
		{m, h, m}, // commutative
		{m, m, m}, // equal
	}
	for _, c := range cases {
		if got := getShorterInterval(c.a, c.b); got != c.want {
			t.Errorf("getShorterInterval(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestPreserveConditionTimestamp(t *testing.T) {
	t0 := metav1.NewTime(time.Now().Add(-time.Hour).Truncate(time.Second))
	t1 := metav1.NewTime(time.Now().Truncate(time.Second))

	mkStatus := func(status metav1.ConditionStatus, reason, msg string, ts metav1.Time) *workspacev1alpha1.IntegrationStatus {
		return &workspacev1alpha1.IntegrationStatus{
			Conditions: []metav1.Condition{{
				Type: IntegrationConditionTypeReady, Status: status, Reason: reason, Message: msg, LastTransitionTime: ts,
			}},
		}
	}

	t.Run("same status+reason -> prior timestamp kept", func(t *testing.T) {
		prior := mkStatus(metav1.ConditionTrue, IntegrationReasonReady, "", t0)
		next := mkStatus(metav1.ConditionTrue, IntegrationReasonReady, "", t1)
		preserveConditionTimestamp(prior, next)
		if !next.Conditions[0].LastTransitionTime.Equal(&t0) {
			t.Errorf("want prior timestamp %v kept, got %v", t0, next.Conditions[0].LastTransitionTime)
		}
	})

	t.Run("changed status -> new timestamp kept", func(t *testing.T) {
		prior := mkStatus(metav1.ConditionTrue, IntegrationReasonReady, "", t0)
		next := mkStatus(metav1.ConditionFalse, IntegrationReasonProbeFailed, "", t1)
		preserveConditionTimestamp(prior, next)
		if !next.Conditions[0].LastTransitionTime.Equal(&t1) {
			t.Errorf("verdict changed: want new timestamp %v, got %v", t1, next.Conditions[0].LastTransitionTime)
		}
	})

	t.Run("same status+reason but different message -> prior timestamp kept (Message excluded)", func(t *testing.T) {
		// A failing probe's stderr can vary run-to-run without the verdict changing; the timestamp must
		// stay stable so a report-only re-probe does not churn LastTransitionTime every cadence.
		prior := mkStatus(metav1.ConditionFalse, IntegrationReasonProbeFailed, "cannot connect at 10:00", t0)
		next := mkStatus(metav1.ConditionFalse, IntegrationReasonProbeFailed, "cannot connect at 10:05", t1)
		preserveConditionTimestamp(prior, next)
		if !next.Conditions[0].LastTransitionTime.Equal(&t0) {
			t.Errorf("message-only change must keep prior timestamp %v, got %v", t0, next.Conditions[0].LastTransitionTime)
		}
	})

	t.Run("nil / empty conditions -> no panic, next unchanged", func(t *testing.T) {
		next := mkStatus(metav1.ConditionTrue, IntegrationReasonReady, "", t1)
		preserveConditionTimestamp(nil, next)
		preserveConditionTimestamp(&workspacev1alpha1.IntegrationStatus{}, next)
		if !next.Conditions[0].LastTransitionTime.Equal(&t1) {
			t.Errorf("no prior: timestamp must be untouched %v, got %v", t1, next.Conditions[0].LastTransitionTime)
		}
	})
}

// fakeExitError satisfies the local exitStatusError interface (mirrors k8s.io/.../exec.CodeExitError):
// a command that ran and exited non-zero. It must classify as a real failure (ProbeFailed), never transient.
type fakeExitError struct{ code int }

func (e fakeExitError) Error() string {
	return fmt.Sprintf("command terminated with exit code %d", e.code)
}
func (e fakeExitError) Exited() bool    { return true }
func (e fakeExitError) ExitStatus() int { return e.code }

// fakeNetError satisfies net.Error: a transport-level failure reaching the exec stream.
type fakeNetError struct{}

func (fakeNetError) Error() string   { return "dial tcp: connection refused" }
func (fakeNetError) Timeout() bool   { return false }
func (fakeNetError) Temporary() bool { return true }

// TestIsTransientProbeError covers the ProbeError-vs-ProbeFailed classifier directly: transport/infra
// failures (context deadline/cancel, net.Error) are transient (never got a verdict -> ProbeError), while a
// real non-zero command exit (exec.CodeExitError-shaped) or any unclassified error is a genuine failure
// (ProbeFailed). A nil error is not transient.
func TestIsTransientProbeError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil -> not transient", nil, false},
		{"context deadline -> transient", context.DeadlineExceeded, true},
		{"context cancelled -> transient", context.Canceled, true},
		{"net error -> transient", fakeNetError{}, true},
		{"wrapped net error -> transient", fmt.Errorf("exec stream: %w", fakeNetError{}), true},
		{"non-zero exit -> not transient", fakeExitError{code: 1}, false},
		{"wrapped non-zero exit -> not transient", fmt.Errorf("exec failed: %w", fakeExitError{code: 7}), false},
		// A command that exited non-zero AND carries a wrapped deadline must still be a real failure:
		// the exit-status check takes precedence over the context-error check.
		{"exit error wrapping deadline -> not transient", fmt.Errorf("%w (%w)", fakeExitError{code: 1}, context.DeadlineExceeded), false},
		{"plain unclassified error -> not transient (treated as real failure)", fmt.Errorf("something odd"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isTransientProbeError(tc.err); got != tc.want {
				t.Errorf("isTransientProbeError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestTruncateProbeMessage covers the CRD-ceiling guard: a short message passes through untouched, an
// over-cap message is clipped to maxProbeMessageBytes (plus the marker) on a valid UTF-8 boundary so a
// multi-byte rune is never split.
func TestTruncateProbeMessage(t *testing.T) {
	t.Run("under cap -> unchanged", func(t *testing.T) {
		msg := probeFailMsg
		if got := truncateProbeMessage(msg); got != msg {
			t.Errorf("short message must pass through unchanged, got %q", got)
		}
	})

	t.Run("over cap -> clipped with marker, stays valid UTF-8", func(t *testing.T) {
		got := truncateProbeMessage(strings.Repeat("a", maxProbeMessageBytes+500))
		if len(got) > maxProbeMessageBytes+len(" ...(truncated)") {
			t.Errorf("clipped message exceeds cap+marker: len=%d", len(got))
		}
		if !strings.HasSuffix(got, " ...(truncated)") {
			t.Errorf("clipped message must carry the truncation marker, got tail %q", got[len(got)-20:])
		}
		if !utf8.ValidString(got) {
			t.Error("clipped message must remain valid UTF-8")
		}
	})

	t.Run("over cap on a multi-byte boundary -> never splits a rune", func(t *testing.T) {
		// Fill exactly up to one byte past the cap with 3-byte runes so the naive cut at
		// maxProbeMessageBytes lands mid-rune; the boundary back-off must repair it.
		msg := strings.Repeat("世", maxProbeMessageBytes) // each "世" is 3 bytes
		got := truncateProbeMessage(msg)
		if !utf8.ValidString(got) {
			t.Error("truncation split a multi-byte rune -> invalid UTF-8")
		}
		if !strings.HasSuffix(got, " ...(truncated)") {
			t.Error("multi-byte clip must still carry the truncation marker")
		}
	})
}

// TestIntegrationProber_EmptyExecCommand covers the defensive guard in Probe: a probe that reaches Probe
// with a nil/empty Exec.Command reports ProbeError (not-ready) rather than panicking on the exec call.
func TestIntegrationProber_EmptyExecCommand(t *testing.T) {
	pod := runningWSPod(testNamespace, "empty-cmd-ws")
	prober := NewIntegrationProber(fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(), &fakeExec{}, DefaultIntegrationProbePeriod)

	cases := map[string]*workspacev1alpha1.IntegrationStatusProbe{
		"nil exec":      {Exec: nil},
		"empty command": {Exec: &corev1.ExecAction{Command: []string{}}},
	}
	for name, probe := range cases {
		t.Run(name, func(t *testing.T) {
			st := prober.Probe(context.Background(), pod, "ray-integration", probe)
			if st.State != IntegrationStateDegraded {
				t.Errorf("%s: want Degraded, got state=%q", name, st.State)
			}
			cond := findReadyCond(st.Conditions)
			if cond == nil || cond.Reason != IntegrationReasonProbeError {
				t.Errorf("%s: want reason=%q, got %+v", name, IntegrationReasonProbeError, cond)
			}
		})
	}
}

// statusWithReason builds an IntegrationStatus carrying a single Ready condition of the given verdict, for
// exercising the edge-triggered event decision. nil reason/ready combinations are the caller's choice.
func statusWithReason(name string, ready bool, reason, msg string) *workspacev1alpha1.IntegrationStatus {
	s := buildIntegrationStatus(name, ready, reason, msg)
	return &s
}

// TestIntegrationStatusEvent covers the edge-triggered event decision directly: an event fires only on a
// verdict TRANSITION (never on an unchanged re-probe), a healthy first attach is silent, a not-ready ->
// Ready flip recovers, and a reason change while still not-ready re-warns. This is the anti-spam contract.
func TestIntegrationStatusEvent(t *testing.T) {
	const name = "ray"
	ready := func(msg string) *workspacev1alpha1.IntegrationStatus {
		return statusWithReason(name, true, IntegrationReasonReady, msg)
	}
	failed := func(reason, msg string) *workspacev1alpha1.IntegrationStatus {
		return statusWithReason(name, false, reason, msg)
	}

	cases := []struct {
		name       string
		prior      *workspacev1alpha1.IntegrationStatus
		next       *workspacev1alpha1.IntegrationStatus
		wantType   string // "" means no event expected
		wantReason string
	}{
		{"healthy first attach -> silent", nil, ready(""), "", ""},
		{"steady-state Ready re-probe -> silent", ready(""), ready(""), "", ""},
		{"first-seen not-ready -> Warning", nil, failed(IntegrationReasonProbeFailed, probeFailMsg), corev1.EventTypeWarning, IntegrationEventDegraded},
		{"Ready -> not-ready -> Warning", ready(""), failed(IntegrationReasonProbeFailed, probeFailMsg), corev1.EventTypeWarning, IntegrationEventDegraded},
		{"unchanged not-ready (same reason) -> silent", failed(IntegrationReasonProbeFailed, "at 10:00"), failed(IntegrationReasonProbeFailed, "at 10:05"), "", ""},
		{"not-ready reason change -> Warning", failed(IntegrationReasonPodNotFound, ""), failed(IntegrationReasonProbeFailed, probeFailMsg), corev1.EventTypeWarning, IntegrationEventDegraded},
		{"not-ready -> Ready -> Recovered", failed(IntegrationReasonProbeFailed, probeFailMsg), ready(""), corev1.EventTypeNormal, IntegrationEventRecovered},
		{"nil next -> silent", ready(""), nil, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := getIntegrationStatusEvent(name, tc.prior, tc.next)
			if ev.Type != tc.wantType || ev.Reason != tc.wantReason {
				t.Errorf("got type=%q reason=%q, want type=%q reason=%q", ev.Type, ev.Reason, tc.wantType, tc.wantReason)
			}
			if tc.wantType != "" && !strings.Contains(ev.Message, name) {
				t.Errorf("event message must name the integration %q, got %q", name, ev.Message)
			}
		})
	}
}

// drainRecorder collects all buffered events from a FakeRecorder without blocking (the channel is buffered),
// so a spec can assert exactly which reasons fired across a sequence of probe cycles.
func drainRecorder(r *record.FakeRecorder) []string {
	var out []string
	for {
		select {
		case e := <-r.Events:
			out = append(out, e)
		default:
			return out
		}
	}
}

// TestProbeIntegrationStatus_EmitsEdgeTriggeredEvents drives the full state-machine probe path with a real
// FakeRecorder to prove the anti-spam contract end-to-end: a degraded integration emits ONE Warning on the
// transition and stays silent while it remains degraded, then emits ONE Normal event when it recovers.
func TestProbeIntegrationStatus_EmitsEdgeTriggeredEvents(t *testing.T) {
	ns, wsName := testNamespace, "event-ws"
	rm := proberResourceManager(probeTemplate(intReadyName, ns, true))
	pod := runningWSPod(ns, wsName)

	newWS := func() *workspacev1alpha1.Workspace {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: wsName, Namespace: ns},
			Spec:       workspacev1alpha1.WorkspaceSpec{IntegrationTemplateRefs: []workspacev1alpha1.IntegrationTemplateRef{ref(intReadyName)}},
		}
		ws.Status.ResolvedIntegrations = []workspacev1alpha1.ResolvedIntegration{{Name: intReadyName}}
		return ws
	}

	recorder := record.NewFakeRecorder(10)
	failVerdict := buildIntegrationStatus(intReadyName, false, IntegrationReasonProbeFailed, probeFailMsg)
	mock := &mockIntegrationProber{pod: pod, probeByName: map[string]workspacev1alpha1.IntegrationStatus{intReadyName: failVerdict}}
	sm := &StateMachine{resourceManager: rm, integrationProber: mock, recorder: recorder}

	ws := newWS()

	// Cycle 1: first probe returns ProbeFailed -> exactly one Warning IntegrationDegraded.
	sm.probeIntegrationStatus(context.Background(), ws)
	events := drainRecorder(recorder)
	if len(events) != 1 || !strings.Contains(events[0], IntegrationEventDegraded) || !strings.Contains(events[0], "Warning") {
		t.Fatalf("cycle 1: want one Warning %s event, got %v", IntegrationEventDegraded, events)
	}

	// Cycle 2: still ProbeFailed (unchanged verdict) -> NO new event (edge-triggered, not per-cadence).
	sm.probeIntegrationStatus(context.Background(), ws)
	if events := drainRecorder(recorder); len(events) != 0 {
		t.Fatalf("cycle 2: unchanged degraded verdict must emit no event, got %v", events)
	}

	// Cycle 3: probe now returns Ready -> exactly one Normal IntegrationRecovered.
	mock.probeByName[intReadyName] = buildIntegrationStatus(intReadyName, true, IntegrationReasonReady, "")
	sm.probeIntegrationStatus(context.Background(), ws)
	events = drainRecorder(recorder)
	if len(events) != 1 || !strings.Contains(events[0], IntegrationEventRecovered) || !strings.Contains(events[0], "Normal") {
		t.Fatalf("cycle 3: want one Normal %s event, got %v", IntegrationEventRecovered, events)
	}

	// Cycle 4: steady-state Ready re-probe -> silent.
	sm.probeIntegrationStatus(context.Background(), ws)
	if events := drainRecorder(recorder); len(events) != 0 {
		t.Fatalf("cycle 4: steady-state Ready re-probe must emit no event, got %v", events)
	}
}

// TestProbeIntegrationStatus_HealthyFirstAttachIsSilent guards the common case: a workspace whose
// integration is Ready on the very first probe must not emit an event (no prior verdict to transition from).
func TestProbeIntegrationStatus_HealthyFirstAttachIsSilent(t *testing.T) {
	ns, wsName := testNamespace, "healthy-ws"
	rm := proberResourceManager(probeTemplate(intReadyName, ns, true))
	recorder := record.NewFakeRecorder(10)
	mock := &mockIntegrationProber{pod: runningWSPod(ns, wsName)} // default Probe() returns Ready
	sm := &StateMachine{resourceManager: rm, integrationProber: mock, recorder: recorder}

	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: wsName, Namespace: ns},
		Spec:       workspacev1alpha1.WorkspaceSpec{IntegrationTemplateRefs: []workspacev1alpha1.IntegrationTemplateRef{ref(intReadyName)}},
	}
	ws.Status.ResolvedIntegrations = []workspacev1alpha1.ResolvedIntegration{{Name: intReadyName}}

	sm.probeIntegrationStatus(context.Background(), ws)
	if events := drainRecorder(recorder); len(events) != 0 {
		t.Fatalf("healthy first attach must be silent, got %v", events)
	}
}
