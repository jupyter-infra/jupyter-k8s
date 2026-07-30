/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// mockIdleChecker is a test double for WorkspaceIdleCheckerInterface. It records
// how many times CheckWorkspaceIdle was called and returns canned results.
type mockIdleChecker struct {
	result   *IdleCheckResult
	err      error
	interval time.Duration
	calls    int
}

func (m *mockIdleChecker) CheckWorkspaceIdle(
	_ context.Context,
	_ *workspacev1alpha1.Workspace,
	_ *corev1.Service,
	_ *workspacev1alpha1.IdleShutdownSpec,
) (*IdleCheckResult, error) {
	m.calls++
	return m.result, m.err
}

func (m *mockIdleChecker) CheckInterval() time.Duration {
	if m.interval == 0 {
		return DefaultIdleCheckInterval
	}
	return m.interval
}

// idleEnabledWorkspace returns a workspace with idle shutdown enabled.
func idleEnabledWorkspace() *workspacev1alpha1.Workspace {
	ws := smWorkspace()
	ws.Spec.IdleShutdown = &workspacev1alpha1.IdleShutdownSpec{
		Enabled:              true,
		IdleTimeoutInMinutes: 30,
	}
	return ws
}

func newStateMachineWithIdleChecker(c client.Client, idle WorkspaceIdleCheckerInterface) *StateMachine {
	statusManager := NewStatusManager(c)
	rm := NewResourceManager(c, c.Scheme(), nil, nil, nil, nil, statusManager)
	return &StateMachine{
		resourceManager:     rm,
		statusManager:       statusManager,
		recorder:            record.NewFakeRecorder(10),
		idleChecker:         idle,
		accessStartupProber: &mockAccessStartupProber{ready: true},
	}
}

func TestHandleIdleShutdownForRunningWorkspace(t *testing.T) {
	scheme := smScheme(t)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: GenerateServiceName(testWorkspaceName), Namespace: testNamespace},
	}

	t.Run("no requeue when idle shutdown is disabled", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		idle := &mockIdleChecker{}
		sm := newStateMachineWithIdleChecker(c, idle)

		// A workspace with no IdleShutdown config.
		result, err := sm.handleIdleShutdownForRunningWorkspace(context.Background(), smWorkspace(), svc)

		require.NoError(t, err)
		assert.Zero(t, result.RequeueAfter)
		assert.Equal(t, 0, idle.calls, "idle checker must not be called when disabled")
	})

	t.Run("no requeue on permanent failure (ShouldRetry=false)", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		idle := &mockIdleChecker{
			result: &IdleCheckResult{IsIdle: false, ShouldRetry: false},
			err:    errors.New("permanent"),
		}
		sm := newStateMachineWithIdleChecker(c, idle)

		result, err := sm.handleIdleShutdownForRunningWorkspace(context.Background(), idleEnabledWorkspace(), svc)

		require.NoError(t, err)
		assert.Zero(t, result.RequeueAfter)
		assert.Equal(t, 1, idle.calls)
	})

	t.Run("requeues on temporary failure (ShouldRetry=true)", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		idle := &mockIdleChecker{
			result:   &IdleCheckResult{IsIdle: false, ShouldRetry: true},
			err:      errors.New("temporary"),
			interval: 45 * time.Second,
		}
		sm := newStateMachineWithIdleChecker(c, idle)

		result, err := sm.handleIdleShutdownForRunningWorkspace(context.Background(), idleEnabledWorkspace(), svc)

		require.NoError(t, err)
		assert.Equal(t, 45*time.Second, result.RequeueAfter)
	})

	t.Run("requeues at check interval when not idle", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		idle := &mockIdleChecker{
			result:   &IdleCheckResult{IsIdle: false, ShouldRetry: false},
			interval: 90 * time.Second,
		}
		sm := newStateMachineWithIdleChecker(c, idle)

		result, err := sm.handleIdleShutdownForRunningWorkspace(context.Background(), idleEnabledWorkspace(), svc)

		require.NoError(t, err)
		assert.Equal(t, 90*time.Second, result.RequeueAfter)
	})

	t.Run("stops the workspace when idle", func(t *testing.T) {
		ws := idleEnabledWorkspace()
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
		idle := &mockIdleChecker{
			result: &IdleCheckResult{IsIdle: true, ShouldRetry: false},
		}
		sm := newStateMachineWithIdleChecker(c, idle)

		result, err := sm.handleIdleShutdownForRunningWorkspace(context.Background(), ws, svc)

		require.NoError(t, err)
		assert.Equal(t, MinimalRequeueDelay, result.RequeueAfter)

		// The workspace should have been flipped to DesiredStatus=Stopped.
		got := &workspacev1alpha1.Workspace{}
		require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(ws), got))
		assert.Equal(t, DesiredStateStopped, got.Spec.DesiredStatus)
	})
}

func TestStopWorkspaceDueToIdle(t *testing.T) {
	scheme := smScheme(t)
	idleConfig := &workspacev1alpha1.IdleShutdownSpec{Enabled: true, IdleTimeoutInMinutes: 15}

	t.Run("flips desired status to Stopped and requeues", func(t *testing.T) {
		ws := idleEnabledWorkspace()
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
		sm := newStateMachineWithIdleChecker(c, &mockIdleChecker{})

		result, err := sm.stopWorkspaceDueToIdle(context.Background(), ws, idleConfig)

		require.NoError(t, err)
		assert.Equal(t, MinimalRequeueDelay, result.RequeueAfter)
		assert.Equal(t, DesiredStateStopped, ws.Spec.DesiredStatus)

		got := &workspacev1alpha1.Workspace{}
		require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(ws), got))
		assert.Equal(t, DesiredStateStopped, got.Spec.DesiredStatus)
	})

	t.Run("returns error when the update fails", func(t *testing.T) {
		ws := idleEnabledWorkspace()
		base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()
		mock := &MockClient{
			Client: base,
			updateFunc: func(context.Context, client.Object, ...client.UpdateOption) error {
				return errInjected
			},
		}
		sm := newStateMachineWithIdleChecker(mock, &mockIdleChecker{})

		_, err := sm.stopWorkspaceDueToIdle(context.Background(), ws, idleConfig)

		require.Error(t, err)
	})
}
