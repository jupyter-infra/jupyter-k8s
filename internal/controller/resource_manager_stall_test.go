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
