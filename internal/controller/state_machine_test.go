/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

func TestWorkspaceIdleChecker_CheckInterval_CustomValue(t *testing.T) {
	checker := NewWorkspaceIdleChecker(nil, 30*time.Second)
	assert.Equal(t, 30*time.Second, checker.CheckInterval())
}

func TestWorkspaceIdleChecker_CheckInterval_ZeroFallsBackToDefault(t *testing.T) {
	checker := NewWorkspaceIdleChecker(nil, 0)
	assert.Equal(t, DefaultIdleCheckInterval, checker.CheckInterval())
}

func TestWorkspaceIdleChecker_CheckInterval_NegativeFallsBackToDefault(t *testing.T) {
	checker := NewWorkspaceIdleChecker(nil, -1*time.Second)
	assert.Equal(t, DefaultIdleCheckInterval, checker.CheckInterval())
}

func TestGetDesiredStatus_ExplicitRunning(t *testing.T) {
	sm := &StateMachine{}
	ws := &workspacev1alpha1.Workspace{
		Spec: workspacev1alpha1.WorkspaceSpec{DesiredStatus: DesiredStateRunning},
	}
	assert.Equal(t, DesiredStateRunning, sm.getDesiredStatus(ws))
}

func TestGetDesiredStatus_ExplicitStopped(t *testing.T) {
	sm := &StateMachine{}
	ws := &workspacev1alpha1.Workspace{
		Spec: workspacev1alpha1.WorkspaceSpec{DesiredStatus: ConditionTypeStopped},
	}
	assert.Equal(t, ConditionTypeStopped, sm.getDesiredStatus(ws))
}

func TestGetDesiredStatus_EmptyDefaultsToRunning(t *testing.T) {
	sm := &StateMachine{}
	ws := &workspacev1alpha1.Workspace{
		Spec: workspacev1alpha1.WorkspaceSpec{DesiredStatus: ""},
	}
	assert.Equal(t, DefaultDesiredStatus, sm.getDesiredStatus(ws))
}

func TestProbeRetrySeconds_WithinLinearPhase(t *testing.T) {
	// failureThreshold=20, ProbeBackoffThreshold=10, so first 10 retries are linear
	result := probeRetrySeconds(2, 5, 20)
	assert.Equal(t, int32(2), result)
}

func TestProbeRetrySeconds_InBackoffPhase(t *testing.T) {
	// failure 11 is the first in the backoff phase (backoffStart = 20-10 = 10)
	result := probeRetrySeconds(2, 11, 20)
	assert.Equal(t, int32(4), result) // 2 * 2^1
}

func TestProbeRetrySeconds_CappedAtMax(t *testing.T) {
	// Deep into backoff — should cap at ProbeBackoffMaxRetrySeconds
	result := probeRetrySeconds(2, 19, 20)
	assert.Equal(t, int32(ProbeBackoffMaxRetrySeconds), result)
}

func TestTimeUntilProbeDeadline_Nil(t *testing.T) {
	ws := &workspacev1alpha1.Workspace{}
	assert.Equal(t, time.Duration(0), timeUntilProbeDeadline(ws))
}

func TestTimeUntilProbeDeadline_InThePast(t *testing.T) {
	past := metav1.NewTime(time.Now().Add(-10 * time.Second))
	ws := &workspacev1alpha1.Workspace{}
	ws.Status.EarliestNextProbeTime = &past
	assert.Equal(t, time.Duration(0), timeUntilProbeDeadline(ws))
}

func TestTimeUntilProbeDeadline_InTheFuture(t *testing.T) {
	future := metav1.NewTime(time.Now().Add(5 * time.Second))
	ws := &workspacev1alpha1.Workspace{}
	ws.Status.EarliestNextProbeTime = &future
	remaining := timeUntilProbeDeadline(ws)
	assert.True(t, remaining > 0 && remaining <= 5*time.Second)
}
