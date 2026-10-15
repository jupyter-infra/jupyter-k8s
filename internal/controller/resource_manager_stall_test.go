/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const testStalledMessage = "ReplicaSet \"jupyter-test-workspace-abc\" has timed out progressing."

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
			deployment: progressing(corev1.ConditionTrue, replicaSetUpdatedReason, "ReplicaSet is progressing."),
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

func TestDeploymentProgressDeadline(t *testing.T) {
	shorter := int32(300)
	assert.Equal(t, defaultDeploymentProgressDeadline, deploymentProgressDeadline(nil))
	assert.Equal(t, defaultDeploymentProgressDeadline, deploymentProgressDeadline(&appsv1.Deployment{}))
	assert.Equal(t, 5*time.Minute, deploymentProgressDeadline(&appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{ProgressDeadlineSeconds: &shorter},
	}))
}

func TestResourceManager_DeploymentFailedCreate(t *testing.T) {
	const quotaMessage = `pods "ws-abc" is forbidden: exceeded quota: gpu, requested: requests.nvidia.com/gpu=1, ` +
		`used: requests.nvidia.com/gpu=4, limited: requests.nvidia.com/gpu=4`
	replicaFailure := func(status corev1.ConditionStatus, reason, message string) *appsv1.Deployment {
		return &appsv1.Deployment{Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{
			{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: replicaSetUpdatedReason},
			{Type: appsv1.DeploymentReplicaFailure, Status: status, Reason: reason, Message: message},
		}}}
	}

	tests := []struct {
		name            string
		deployment      *appsv1.Deployment
		expectedFailed  bool
		expectedMessage string
	}{
		{name: "nil deployment"},
		{name: "a deployment without conditions", deployment: &appsv1.Deployment{}},
		{name: "progressing without a replica failure", deployment: &appsv1.Deployment{Status: appsv1.DeploymentStatus{
			Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue}}}}},
		{name: "a pod the ReplicaSet controller cannot create",
			deployment:     replicaFailure(corev1.ConditionTrue, replicaSetReasonFailedCreate, quotaMessage),
			expectedFailed: true, expectedMessage: quotaMessage},
		{name: "a pod it cannot delete is not a start failure",
			deployment: replicaFailure(corev1.ConditionTrue, "FailedDelete", "forbidden")},
		{name: "a cleared replica failure",
			deployment: replicaFailure(corev1.ConditionFalse, replicaSetReasonFailedCreate, quotaMessage)},
	}

	rm := &ResourceManager{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, failed := rm.DeploymentFailedCreate(tt.deployment)
			assert.Equal(t, tt.expectedFailed, failed)
			assert.Equal(t, tt.expectedMessage, message)
		})
	}
}
