/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// apiInternalErr returns a non-NotFound API error, useful for driving the
// error branches of Get-based helpers.
func apiInternalErr() error {
	return apierrors.NewInternalError(errInjected)
}

// smScheme returns a scheme with the types the state machine tests need.
func smScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(s))
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, corev1.AddToScheme(s))
	return s
}

// smWorkspace builds a minimal workspace for state machine tests.
func smWorkspace() *workspacev1alpha1.Workspace {
	return &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testWorkspaceName,
			Namespace: testNamespace,
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			Image:         testImage,
			DesiredStatus: DesiredStateRunning,
		},
	}
}

// newStateMachineWithClient wires a StateMachine around the given client with real
// managers. The idle checker is left nil; tests that exercise idle paths use
// newStateMachineWithIdleChecker instead.
func newStateMachineWithClient(c client.Client, scheme *runtime.Scheme) *StateMachine {
	statusManager := NewStatusManager(c)
	rm := NewResourceManager(
		c,
		scheme,
		NewDeploymentBuilder(scheme, WorkspaceControllerOptions{}, c),
		NewServiceBuilder(scheme),
		NewPVCBuilder(scheme),
		NewAccessResourcesBuilder(),
		statusManager,
	)
	return &StateMachine{
		resourceManager:     rm,
		statusManager:       statusManager,
		recorder:            record.NewFakeRecorder(10),
		accessStartupProber: &mockAccessStartupProber{ready: true},
	}
}

func TestNewStateMachine(t *testing.T) {
	scheme := smScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	statusManager := NewStatusManager(c)
	rm := NewResourceManager(c, scheme, nil, nil, nil, nil, statusManager)
	idleChecker := NewWorkspaceIdleChecker(c, time.Minute)
	prober := &mockAccessStartupProber{}

	sm := NewStateMachine(rm, statusManager, record.NewFakeRecorder(1), idleChecker, prober)

	require.NotNil(t, sm)
	assert.Same(t, rm, sm.resourceManager)
	assert.Same(t, statusManager, sm.statusManager)
	assert.Equal(t, idleChecker, sm.idleChecker)
}

func TestGetAccessStrategyForWorkspace(t *testing.T) {
	scheme := smScheme(t)

	t.Run("returns nil when the workspace has no access strategy", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		sm := newStateMachineWithClient(c, scheme)

		got, err := sm.GetAccessStrategyForWorkspace(context.Background(), smWorkspace())

		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("returns the referenced access strategy", func(t *testing.T) {
		strategy := &workspacev1alpha1.WorkspaceAccessStrategy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      testStrategyName,
				Namespace: accessStrategyNamespaceConst,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(strategy).Build()
		sm := newStateMachineWithClient(c, scheme)

		ws := smWorkspace()
		ws.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
			Name:      testStrategyName,
			Namespace: accessStrategyNamespaceConst,
		}

		got, err := sm.GetAccessStrategyForWorkspace(context.Background(), ws)

		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, testStrategyName, got.Name)
	})

	t.Run("returns error when the referenced access strategy is missing", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		sm := newStateMachineWithClient(c, scheme)

		ws := smWorkspace()
		ws.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
			Name:      testStrategyName,
			Namespace: accessStrategyNamespaceConst,
		}

		_, err := sm.GetAccessStrategyForWorkspace(context.Background(), ws)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestReconcileDesiredState_UnknownStatus(t *testing.T) {
	scheme := smScheme(t)
	ws := smWorkspace()
	ws.Spec.DesiredStatus = "Bogus"

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&workspacev1alpha1.Workspace{}).
		WithObjects(ws).
		Build()
	sm := newStateMachineWithClient(c, scheme)

	result, err := sm.ReconcileDesiredState(context.Background(), ws, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown desired status")
	assert.Equal(t, LongRequeueDelay, result.RequeueAfter)
}

func TestReconcileDesiredStoppedStatus_DeleteErrors(t *testing.T) {
	scheme := smScheme(t)

	newStoppedWorkspace := func() *workspacev1alpha1.Workspace {
		ws := smWorkspace()
		ws.Spec.DesiredStatus = DesiredStateStopped
		return ws
	}

	t.Run("returns error when deployment deletion get fails", func(t *testing.T) {
		ws := newStoppedWorkspace()
		base := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&workspacev1alpha1.Workspace{}).
			WithObjects(ws).
			Build()
		mock := &MockClient{
			Client: base,
			getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*appsv1.Deployment); ok {
					return apiInternalErr()
				}
				return base.Get(ctx, key, obj, opts...)
			},
		}
		sm := newStateMachineWithClient(mock, scheme)

		_, err := sm.ReconcileDesiredState(context.Background(), ws, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get deployment")
	})

	t.Run("returns error when service deletion get fails", func(t *testing.T) {
		ws := newStoppedWorkspace()
		base := fake.NewClientBuilder().
			WithScheme(scheme).
			WithStatusSubresource(&workspacev1alpha1.Workspace{}).
			WithObjects(ws).
			Build()
		mock := &MockClient{
			Client: base,
			getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*corev1.Service); ok {
					return apiInternalErr()
				}
				return base.Get(ctx, key, obj, opts...)
			},
		}
		sm := newStateMachineWithClient(mock, scheme)

		_, err := sm.ReconcileDesiredState(context.Background(), ws, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get service")
	})
}

func TestReconcileDeletion_NoFinalizer(t *testing.T) {
	scheme := smScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	sm := newStateMachineWithClient(c, scheme)

	result, err := sm.ReconcileDeletion(context.Background(), smWorkspace())

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
}

func TestReconcileDeletion_RemovesFinalizerWhenCleaned(t *testing.T) {
	scheme := smScheme(t)
	ws := smWorkspace()
	ws.Finalizers = []string{WorkspaceFinalizerName}

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&workspacev1alpha1.Workspace{}).
		WithObjects(ws).
		Build()
	sm := newStateMachineWithClient(c, scheme)

	// No deployment/service/PVC exist, so CleanupAllResources reports fully deleted.
	result, err := sm.ReconcileDeletion(context.Background(), ws)

	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)

	// Finalizer should be gone from the persisted object.
	got := &workspacev1alpha1.Workspace{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(ws), got))
	assert.NotContains(t, got.Finalizers, WorkspaceFinalizerName)
}

func TestReconcileDeletion_CleanupError(t *testing.T) {
	scheme := smScheme(t)
	ws := smWorkspace()
	ws.Finalizers = []string{WorkspaceFinalizerName}
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateDeploymentName(ws.Name),
			Namespace: ws.Namespace,
		},
	}
	base := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&workspacev1alpha1.Workspace{}).
		WithObjects(ws, dep).
		Build()
	// Delete fails, so CleanupAllResources propagates an error.
	mock := &MockClient{
		Client: base,
		deleteFunc: func(context.Context, client.Object, ...client.DeleteOption) error {
			return errInjected
		},
	}
	sm := newStateMachineWithClient(mock, scheme)

	_, err := sm.ReconcileDeletion(context.Background(), ws)

	require.Error(t, err)
}

func TestReconcileDeletion_FinalizerUpdateError(t *testing.T) {
	scheme := smScheme(t)
	ws := smWorkspace()
	ws.Finalizers = []string{WorkspaceFinalizerName}
	base := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&workspacev1alpha1.Workspace{}).
		WithObjects(ws).
		Build()
	// No resources exist, so cleanup succeeds and we reach the finalizer removal
	// Update, which we force to fail.
	mock := &MockClient{
		Client: base,
		updateFunc: func(context.Context, client.Object, ...client.UpdateOption) error {
			return errInjected
		},
	}
	sm := newStateMachineWithClient(mock, scheme)

	_, err := sm.ReconcileDeletion(context.Background(), ws)

	require.Error(t, err)
}

func TestReconcileDeletion_RequeuesWhenResourcesRemain(t *testing.T) {
	scheme := smScheme(t)
	ws := smWorkspace()
	ws.Finalizers = []string{WorkspaceFinalizerName}

	// A deployment with a finalizer survives the Delete call (it gets a deletion
	// timestamp instead of being removed), so CleanupAllResources reports
	// not-all-deleted and the reconcile requeues.
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:       GenerateDeploymentName(ws.Name),
			Namespace:  ws.Namespace,
			Finalizers: []string{"keep-around/test"},
		},
	}
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&workspacev1alpha1.Workspace{}).
		WithObjects(ws, dep).
		Build()
	sm := newStateMachineWithClient(c, scheme)

	result, err := sm.ReconcileDeletion(context.Background(), ws)

	require.NoError(t, err)
	assert.Equal(t, PollRequeueDelay, result.RequeueAfter)

	// Finalizer must still be present since cleanup is not complete.
	got := &workspacev1alpha1.Workspace{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(ws), got))
	assert.Contains(t, got.Finalizers, WorkspaceFinalizerName)
}

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
