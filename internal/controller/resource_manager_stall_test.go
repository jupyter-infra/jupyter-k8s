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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const testStalledMessage = "ReplicaSet \"jupyter-test-workspace-abc\" has timed out progressing."

func TestResourceManager_IsDeploymentProgressDeadlineExceeded(t *testing.T) {
	deadline := time.Date(2026, 10, 8, 12, 10, 0, 0, time.UTC)
	progressing := func(status corev1.ConditionStatus, reason, message string) *appsv1.Deployment {
		return &appsv1.Deployment{Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{{
			Type: appsv1.DeploymentProgressing, Status: status, Reason: reason, Message: message,
			LastTransitionTime: metav1.NewTime(deadline),
		}}}}
	}

	tests := []struct {
		name            string
		deployment      *appsv1.Deployment
		expectedStalled bool
		expectedMessage string
		expectedSince   time.Time
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
			expectedSince:   deadline,
		},
	}

	rm := &ResourceManager{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stalled, message, since := rm.IsDeploymentProgressDeadlineExceeded(tt.deployment)
			assert.Equal(t, tt.expectedStalled, stalled)
			assert.Equal(t, tt.expectedMessage, message)
			assert.True(t, tt.expectedSince.Equal(since), "since %v", since)
		})
	}
}

func TestResourceManager_DeploymentFailedCreate(t *testing.T) {
	const quotaMessage = `pods "ws-abc" is forbidden: exceeded quota: gpu, requested: requests.nvidia.com/gpu=1, ` +
		`used: requests.nvidia.com/gpu=4, limited: requests.nvidia.com/gpu=4`
	replicaFailure := func(status corev1.ConditionStatus, reason, message string) *appsv1.Deployment {
		return &appsv1.Deployment{Status: appsv1.DeploymentStatus{Conditions: []appsv1.DeploymentCondition{
			{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "ReplicaSetUpdated"},
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
		{name: "no conditions", deployment: &appsv1.Deployment{}},
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
