/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"testing"
	"time"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// recordingProber is a stub IntegrationProberInterface that records the (name, wi) pairs it was
// asked to probe and returns a canned ready verdict per call.
type recordingProber struct {
	probed []string // integrationName values, in call order
}

func (r *recordingProber) Probe(
	_ context.Context,
	_ *workspacev1alpha1.Workspace,
	integrationName string,
	_ *workspacev1alpha1.WorkspaceIntegration,
) workspacev1alpha1.IntegrationStatus {
	r.probed = append(r.probed, integrationName)
	return workspacev1alpha1.IntegrationStatus{Name: integrationName, Ready: true, Reason: IntegrationReasonReady}
}

func probeStateMachine(t *testing.T, prober IntegrationProberInterface, children ...*workspacev1alpha1.WorkspaceIntegration) *StateMachine {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, c := range children {
		builder = builder.WithObjects(c)
	}
	c := builder.Build()
	return &StateMachine{
		integrationManager: NewWorkspaceIntegrationManager(c, scheme),
		integrationProber:  prober,
	}
}

func bakedChild(name string, probe *workspacev1alpha1.IntegrationStatusProbe) *workspacev1alpha1.WorkspaceIntegration {
	return &workspacev1alpha1.WorkspaceIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "user-ns",
			Labels: map[string]string{workspaceutil.LabelWorkspaceName: "ws-foo"},
		},
		// A resolved child: carries the frozen statusProbe directly on its spec. Also set a
		// shareProcessNamespace so workspaceIntegrationResolved() treats a probe-less child as
		// resolved (rather than as a not-yet-resolved shell).
		Spec: workspacev1alpha1.WorkspaceIntegrationSpec{
			StatusProbe:           probe,
			ShareProcessNamespace: ptrBool(true),
		},
	}
}

func ptrBool(b bool) *bool { return &b }

func probeTestWorkspace(refNames ...string) *workspacev1alpha1.Workspace {
	refs := make([]workspacev1alpha1.IntegrationTemplateRef, 0, len(refNames))
	for _, n := range refNames {
		refs = append(refs, workspacev1alpha1.IntegrationTemplateRef{Name: n})
	}
	return &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-foo", Namespace: "user-ns"},
		Spec:       workspacev1alpha1.WorkspaceSpec{IntegrationRefs: refs},
	}
}

// TestProbeIntegrationStatus_ProbesBakedChildAndWritesStatus verifies the loop matches an
// integrationRef to its baked child, probes it, writes status.integrations[], and returns the
// probe cadence.
func TestProbeIntegrationStatus_ProbesBakedChildAndWritesStatus(t *testing.T) {
	probe := &workspacev1alpha1.IntegrationStatusProbe{
		Exec:          &corev1.ExecAction{Command: []string{"ray", "status"}},
		PeriodSeconds: 15,
	}
	child := bakedChild("workspace-ws-foo-ray-integration", probe)
	prober := &recordingProber{}
	sm := probeStateMachine(t, prober, child)
	ws := probeTestWorkspace("ray-integration")

	interval := sm.probeIntegrationStatus(context.Background(), ws)

	assert.Equal(t, []string{"ray-integration"}, prober.probed, "the baked child must be probed")
	require.Len(t, ws.Status.Integrations, 1)
	assert.Equal(t, "ray-integration", ws.Status.Integrations[0].Name)
	assert.True(t, ws.Status.Integrations[0].Ready)
	assert.Equal(t, 15*time.Second, interval, "returns the probe cadence for requeue")
}

// TestProbeIntegrationStatus_SkipsChildWithoutProbe verifies an integration whose frozen child has
// no statusProbe contributes no status entry and no probe call.
func TestProbeIntegrationStatus_SkipsChildWithoutProbe(t *testing.T) {
	child := bakedChild("workspace-ws-foo-ray-integration", nil) // resolved but no statusProbe
	prober := &recordingProber{}
	sm := probeStateMachine(t, prober, child)
	ws := probeTestWorkspace("ray-integration")

	interval := sm.probeIntegrationStatus(context.Background(), ws)

	assert.Empty(t, prober.probed, "no probe when the child declares no statusProbe")
	assert.Empty(t, ws.Status.Integrations)
	assert.Equal(t, time.Duration(0), interval)
}

// TestProbeIntegrationStatus_UnresolvedChildReportsResolving verifies a not-yet-resolved child (a
// bare shell with no resolved output) surfaces a not-ready "Resolving" status entry rather than
// silently vanishing from status.integrations[], and that it is not probed.
func TestProbeIntegrationStatus_UnresolvedChildReportsResolving(t *testing.T) {
	unresolved := &workspacev1alpha1.WorkspaceIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "workspace-ws-foo-ray-integration", Namespace: "user-ns",
			Labels: map[string]string{workspaceutil.LabelWorkspaceName: "ws-foo"},
		},
	}
	prober := &recordingProber{}
	sm := probeStateMachine(t, prober, unresolved)
	ws := probeTestWorkspace("ray-integration")

	interval := sm.probeIntegrationStatus(context.Background(), ws)

	assert.Empty(t, prober.probed, "an unresolved child must not be probed")
	require.Len(t, ws.Status.Integrations, 1, "unresolved integration is surfaced, not dropped")
	assert.Equal(t, "ray-integration", ws.Status.Integrations[0].Name)
	assert.False(t, ws.Status.Integrations[0].Ready)
	assert.Equal(t, IntegrationReasonResolving, ws.Status.Integrations[0].Reason)
	// A re-check is scheduled so status flips once the webhook resolves the child.
	assert.Equal(t, time.Duration(DefaultIntegrationProbePeriodSeconds)*time.Second, interval)
}

// TestProbeIntegrationStatus_ClearsStaleStatus verifies that with no integrationRefs, any prior
// status.integrations[] is cleared.
func TestProbeIntegrationStatus_ClearsStaleStatus(t *testing.T) {
	prober := &recordingProber{}
	sm := probeStateMachine(t, prober)
	ws := probeTestWorkspace() // no refs
	ws.Status.Integrations = []workspacev1alpha1.IntegrationStatus{{Name: "old", Ready: true}}

	interval := sm.probeIntegrationStatus(context.Background(), ws)

	assert.Empty(t, ws.Status.Integrations, "removed integration leaves no stale status entry")
	assert.Equal(t, time.Duration(0), interval)
}

// TestProbeIntegrationStatus_NilProberClearsStatus verifies the nil-prober guard clears status and
// returns no cadence (probing disabled, e.g. outside a cluster).
func TestProbeIntegrationStatus_NilProberClearsStatus(t *testing.T) {
	sm := probeStateMachine(t, nil)
	sm.integrationProber = nil
	ws := probeTestWorkspace("ray-integration")
	ws.Status.Integrations = []workspacev1alpha1.IntegrationStatus{{Name: "stale"}}

	interval := sm.probeIntegrationStatus(context.Background(), ws)

	assert.Nil(t, ws.Status.Integrations)
	assert.Equal(t, time.Duration(0), interval)
}
