/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

func TestWaitingContainerMessage(t *testing.T) {
	scheduled := corev1.PodCondition{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}
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
			assert.Equal(t, tt.expected, waitingContainerMessage(&corev1.Pod{Status: tt.status}))
		})
	}
}
