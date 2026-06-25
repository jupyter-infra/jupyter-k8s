/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fakeExec is a stub PodExecWithStderr that records the command and returns canned output.
type fakeExec struct {
	stdout, stderr string
	err            error
	gotCmd         []string
	gotContainer   string
}

func (f *fakeExec) ExecInPodWithStderr(_ context.Context, _ *corev1.Pod, container string, cmd []string, _ string) (string, string, error) {
	f.gotContainer = container
	f.gotCmd = cmd
	return f.stdout, f.stderr, f.err
}

// proberFixture builds an IntegrationProber backed by a fake client seeded with the given
// pods, plus the supplied fake exec.
func proberFixture(t *testing.T, exec PodExecWithStderr, pods ...*corev1.Pod) *IntegrationProber {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, p := range pods {
		builder = builder.WithObjects(p)
	}
	return NewIntegrationProber(builder.Build(), exec)
}

func runningWorkspacePod(workspaceName, namespace string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workspaceName + "-pod",
			Namespace: namespace,
			Labels:    GenerateLabels(workspaceName),
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func workspaceFixture(name, namespace string) *workspacev1alpha1.Workspace {
	return &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
	}
}

// wiWithExecProbe builds a resolved WorkspaceIntegration child whose spec.statusProbe runs the
// given exec command. This is what the prober reads at reconcile (never the template).
func wiWithExecProbe(command []string) *workspacev1alpha1.WorkspaceIntegration {
	return &workspacev1alpha1.WorkspaceIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "workspace-ws-ray-integration", Namespace: "user-ns"},
		Spec: workspacev1alpha1.WorkspaceIntegrationSpec{
			StatusProbe: &workspacev1alpha1.IntegrationStatusProbe{
				Exec: &corev1.ExecAction{Command: command},
			},
		},
	}
}

func TestIntegrationProber_ExecSucceeds_Ready(t *testing.T) {
	exec := &fakeExec{stdout: "ok", stderr: "", err: nil}
	prober := proberFixture(t, exec, runningWorkspacePod("ws", "user-ns"))

	status := prober.Probe(context.Background(), workspaceFixture("ws", "user-ns"),
		"ray-integration", wiWithExecProbe([]string{"ray", "status"}))

	assert.Equal(t, "ray-integration", status.Name, "status keyed by the integrationRef name")
	assert.True(t, status.Ready)
	assert.Equal(t, IntegrationReasonReady, status.Reason)
	// Probe must run in the workspace container with the frozen command.
	assert.Equal(t, integrationProbeWorkspaceContainer, exec.gotContainer)
	assert.Equal(t, []string{"ray", "status"}, exec.gotCmd)
}

func TestIntegrationProber_ExecFails_StderrInMessage(t *testing.T) {
	exec := &fakeExec{stdout: "", stderr: "Ray runtime not started", err: fmt.Errorf("command terminated with exit code 1")}
	prober := proberFixture(t, exec, runningWorkspacePod("ws", "user-ns"))

	status := prober.Probe(context.Background(), workspaceFixture("ws", "user-ns"),
		"ray-integration", wiWithExecProbe([]string{"ray", "status"}))

	assert.False(t, status.Ready)
	assert.Equal(t, IntegrationReasonProbeFailed, status.Reason)
	// stderr is preferred over stdout / error text for the human-readable message.
	assert.Equal(t, "Ray runtime not started", status.Message)
}

func TestIntegrationProber_ExecFails_FallsBackToErrorWhenNoStderr(t *testing.T) {
	exec := &fakeExec{stdout: "", stderr: "", err: fmt.Errorf("command terminated with exit code 7")}
	prober := proberFixture(t, exec, runningWorkspacePod("ws", "user-ns"))

	status := prober.Probe(context.Background(), workspaceFixture("ws", "user-ns"),
		"ray-integration", wiWithExecProbe([]string{"ray", "status"}))

	assert.False(t, status.Ready)
	assert.Equal(t, IntegrationReasonProbeFailed, status.Reason)
	assert.Contains(t, status.Message, "exit code 7")
}

func TestIntegrationProber_NoRunningPod_PodNotFound(t *testing.T) {
	exec := &fakeExec{stdout: "ok"}
	// No pod seeded — findWorkspacePod returns an error.
	prober := proberFixture(t, exec)

	status := prober.Probe(context.Background(), workspaceFixture("ws", "user-ns"),
		"ray-integration", wiWithExecProbe([]string{"ray", "status"}))

	assert.False(t, status.Ready)
	assert.Equal(t, IntegrationReasonPodNotFound, status.Reason)
	assert.NotEmpty(t, status.Message)
	// Exec must not be attempted when there is no pod.
	assert.Nil(t, exec.gotCmd)
}

func TestIntegrationProber_NonRunningPodIgnored(t *testing.T) {
	exec := &fakeExec{stdout: "ok"}
	pendingPod := runningWorkspacePod("ws", "user-ns")
	pendingPod.Status.Phase = corev1.PodPending
	prober := proberFixture(t, exec, pendingPod)

	status := prober.Probe(context.Background(), workspaceFixture("ws", "user-ns"),
		"ray-integration", wiWithExecProbe([]string{"ray", "status"}))

	assert.False(t, status.Ready)
	assert.Equal(t, IntegrationReasonPodNotFound, status.Reason)
}

func TestIntegrationProber_NoProbe_ReportsReady(t *testing.T) {
	exec := &fakeExec{}
	prober := proberFixture(t, exec, runningWorkspacePod("ws", "user-ns"))

	// Resolved child with no statusProbe at all: nothing to check, report ready, no exec.
	wi := &workspacev1alpha1.WorkspaceIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "workspace-ws-ray-integration", Namespace: "user-ns"},
		Spec:       workspacev1alpha1.WorkspaceIntegrationSpec{},
	}

	status := prober.Probe(context.Background(), workspaceFixture("ws", "user-ns"), "ray-integration", wi)

	assert.True(t, status.Ready)
	assert.Equal(t, IntegrationReasonReady, status.Reason)
	assert.Nil(t, exec.gotCmd)
}

func TestIntegrationProber_ProbeWithEmptyCommand_ReportsReady(t *testing.T) {
	exec := &fakeExec{}
	prober := proberFixture(t, exec, runningWorkspacePod("ws", "user-ns"))

	// statusProbe present but Exec command empty — treated as no-op (ready), no exec.
	status := prober.Probe(context.Background(), workspaceFixture("ws", "user-ns"),
		"ray-integration", wiWithExecProbe(nil))

	assert.True(t, status.Ready)
	assert.Equal(t, IntegrationReasonReady, status.Reason)
	assert.Nil(t, exec.gotCmd)
}

func TestIntegrationProber_NoStatusProbe_ReportsReady(t *testing.T) {
	exec := &fakeExec{}
	prober := proberFixture(t, exec, runningWorkspacePod("ws", "user-ns"))

	// Child with no resolved statusProbe: no probe to run, report ready, no exec.
	wi := &workspacev1alpha1.WorkspaceIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "workspace-ws-ray-integration", Namespace: "user-ns"},
	}

	status := prober.Probe(context.Background(), workspaceFixture("ws", "user-ns"), "ray-integration", wi)

	assert.True(t, status.Ready)
	assert.Equal(t, IntegrationReasonReady, status.Reason)
	assert.Nil(t, exec.gotCmd)
}

func TestResolveIntegrationProbeTimeout(t *testing.T) {
	assert.Equal(t, int32(DefaultIntegrationProbeTimeoutSeconds),
		resolveIntegrationProbeTimeout(&workspacev1alpha1.IntegrationStatusProbe{}))
	assert.Equal(t, int32(12),
		resolveIntegrationProbeTimeout(&workspacev1alpha1.IntegrationStatusProbe{TimeoutSeconds: 12}))
}

func TestResolveIntegrationProbePeriod(t *testing.T) {
	// Unset period -> default cadence so a Running workspace keeps re-probing.
	assert.Equal(t, time.Duration(DefaultIntegrationProbePeriodSeconds)*time.Second,
		ResolveIntegrationProbePeriod(&workspacev1alpha1.IntegrationStatusProbe{}))
	// Explicit period is honored.
	assert.Equal(t, 7*time.Second,
		ResolveIntegrationProbePeriod(&workspacev1alpha1.IntegrationStatusProbe{PeriodSeconds: 7}))
}
