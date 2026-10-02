/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

const (
	testSchedulingMessage = "0/3 nodes are available: 3 Insufficient nvidia.com/gpu."
	testStalledMessage    = "ReplicaSet \"jupyter-test-workspace-abc\" has timed out progressing."
)

func TestResourceManager_IsDeploymentProgressDeadlineExceeded(t *testing.T) {
	progressing := func(status corev1.ConditionStatus, reason, message string) *appsv1.Deployment {
		return &appsv1.Deployment{Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{{
			Type: appsv1.DeploymentProgressing, Status: status, Reason: reason, Message: message,
		}}}}
	}

	tests := []struct {
		name            string
		deployment      *appsv1.Deployment
		expectedStalled bool
		expectedMessage string
	}{
		{name: "nil deployment", deployment: nil},
		{name: "no conditions", deployment: &appsv1.Deployment{}},
		{
			name: "only an Available condition",
			deployment: &appsv1.Deployment{Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{{
				Type: appsv1.DeploymentAvailable, Status: corev1.ConditionFalse, Reason: "MinimumReplicasUnavailable",
			}}}},
		},
		{
			name:       "rollout in progress",
			deployment: progressing(corev1.ConditionTrue, "ReplicaSetUpdated", "ReplicaSet is progressing."),
		},
		{
			name:       "rollout complete",
			deployment: progressing(corev1.ConditionTrue, "NewReplicaSetAvailable", "ReplicaSet has successfully progressed."),
		},
		{
			name:       "Progressing=False with another reason",
			deployment: progressing(corev1.ConditionFalse, "SomethingElse", "not a deadline"),
		},
		{
			name:            "deadline exceeded",
			deployment:      progressing(corev1.ConditionFalse, deploymentTimedOutReason, testStalledMessage),
			expectedStalled: true,
			expectedMessage: testStalledMessage,
		},
	}

	rm := &ResourceManager{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stalled, message := rm.IsDeploymentProgressDeadlineExceeded(tt.deployment)
			assert.Equal(t, tt.expectedStalled, stalled)
			assert.Equal(t, tt.expectedMessage, message)
		})
	}
}

func TestPodStallMessage(t *testing.T) {
	scheduled := corev1.PodCondition{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}
	unschedulable := func(message string) corev1.PodCondition {
		return corev1.PodCondition{
			Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable, Message: message,
		}
	}
	waiting := func(name, reason, message string) corev1.ContainerStatus {
		return corev1.ContainerStatus{Name: name, State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{Reason: reason, Message: message},
		}}
	}
	running := corev1.ContainerStatus{Name: "workspace", State: corev1.ContainerState{
		Running: &corev1.ContainerStateRunning{},
	}}

	tests := []struct {
		name     string
		status   corev1.PodStatus
		expected string
	}{
		{name: "no status", status: corev1.PodStatus{}},
		{
			name:     "unschedulable pod reports the scheduler's verdict",
			status:   corev1.PodStatus{Conditions: []corev1.PodCondition{unschedulable(testSchedulingMessage)}},
			expected: testSchedulingMessage,
		},
		{
			name:   "unschedulable pod without a message reports nothing",
			status: corev1.PodStatus{Conditions: []corev1.PodCondition{unschedulable("")}},
		},
		{
			name: "scheduled pod reports the waiting container's reason and message",
			status: corev1.PodStatus{
				Conditions:        []corev1.PodCondition{scheduled},
				ContainerStatuses: []corev1.ContainerStatus{waiting("workspace", "ImagePullBackOff", `Back-off pulling image "example.com/missing:1"`)},
			},
			expected: `ImagePullBackOff: Back-off pulling image "example.com/missing:1"`,
		},
		{
			name: "waiting container without a message reports the reason alone",
			status: corev1.PodStatus{
				Conditions:        []corev1.PodCondition{scheduled},
				ContainerStatuses: []corev1.ContainerStatus{waiting("workspace", "ContainerCreating", "")},
			},
			expected: "ContainerCreating",
		},
		{
			name: "a waiting init container is reported before the main container",
			status: corev1.PodStatus{
				Conditions:            []corev1.PodCondition{scheduled},
				InitContainerStatuses: []corev1.ContainerStatus{waiting("init", "CrashLoopBackOff", "back-off 5m0s restarting failed container=init")},
				ContainerStatuses:     []corev1.ContainerStatus{waiting("workspace", "PodInitializing", "")},
			},
			expected: "CrashLoopBackOff: back-off 5m0s restarting failed container=init",
		},
		{
			name: "the first waiting container wins when a sidecar is already running",
			status: corev1.PodStatus{
				Conditions: []corev1.PodCondition{scheduled},
				ContainerStatuses: []corev1.ContainerStatus{
					running,
					waiting("sidecar", "CreateContainerConfigError", `secret "token" not found`),
				},
			},
			expected: `CreateContainerConfigError: secret "token" not found`,
		},
		{
			name: "the scheduler's verdict wins over container states",
			status: corev1.PodStatus{
				Conditions:        []corev1.PodCondition{unschedulable(testSchedulingMessage)},
				ContainerStatuses: []corev1.ContainerStatus{waiting("workspace", "ContainerCreating", "")},
			},
			expected: testSchedulingMessage,
		},
		{
			name: "a waiting state without a reason is skipped",
			status: corev1.PodStatus{
				Conditions:        []corev1.PodCondition{scheduled},
				ContainerStatuses: []corev1.ContainerStatus{waiting("workspace", "", "")},
			},
		},
		{
			name: "a running pod reports nothing",
			status: corev1.PodStatus{
				Conditions:        []corev1.PodCondition{scheduled},
				ContainerStatuses: []corev1.ContainerStatus{running},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, podStallMessage(&corev1.Pod{Status: tt.status}))
		})
	}
}

func TestResourceManager_WorkspacePodStallMessage(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{
		Name: testWorkspaceName, Namespace: testNamespaceName,
	}}
	unschedulablePod := func(name string, labels map[string]string, message string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespaceName, Labels: labels},
			Status: corev1.PodStatus{Phase: corev1.PodPending, Conditions: []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionFalse, Reason: corev1.PodReasonUnschedulable, Message: message,
			}}},
		}
	}
	now := metav1.Now()
	terminatingPod := unschedulablePod("terminating", GenerateLabels(testWorkspaceName), "stale verdict")
	terminatingPod.DeletionTimestamp = &now
	terminatingPod.Finalizers = []string{"test.jupyter.org/keep"}

	tests := []struct {
		name     string
		objects  []client.Object
		listErr  error
		expected string
	}{
		{name: "no pods"},
		{
			name:     "the workspace pod's verdict",
			objects:  []client.Object{unschedulablePod("pending", GenerateLabels(testWorkspaceName), testSchedulingMessage)},
			expected: testSchedulingMessage,
		},
		{
			name:    "another workspace's pod is not consulted",
			objects: []client.Object{unschedulablePod("other", GenerateLabels("other-workspace"), testSchedulingMessage)},
		},
		{
			name:    "a terminating pod is skipped",
			objects: []client.Object{terminatingPod},
		},
		{
			name: "the pending pod is preferred over the terminating one",
			objects: []client.Object{
				terminatingPod,
				unschedulablePod("pending", GenerateLabels(testWorkspaceName), testSchedulingMessage),
			},
			expected: testSchedulingMessage,
		},
		{
			name:    "a list failure reports nothing",
			objects: []client.Object{unschedulablePod("pending", GenerateLabels(testWorkspaceName), testSchedulingMessage)},
			listErr: errInjected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := crudScheme(t)
			var c client.Client = fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()
			if tt.listErr != nil {
				c = &MockClient{Client: c, listFunc: func(context.Context, client.ObjectList, ...client.ListOption) error {
					return tt.listErr
				}}
			}
			rm := newResourceManagerForCRUD(c, scheme)
			require.NotNil(t, rm)
			assert.Equal(t, tt.expected, rm.WorkspacePodStallMessage(context.Background(), workspace))
		})
	}
}
