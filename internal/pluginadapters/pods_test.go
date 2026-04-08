/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginadapters

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestPod(statuses []corev1.ContainerStatus) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod"},
		Status:     corev1.PodStatus{ContainerStatuses: statuses},
	}
}

func TestIsContainerRunning_Running(t *testing.T) {
	pod := newTestPod([]corev1.ContainerStatus{
		{Name: "main", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
	})
	assert.True(t, IsContainerRunning(pod, "main"))
}

func TestIsContainerRunning_NotRunning(t *testing.T) {
	pod := newTestPod([]corev1.ContainerStatus{
		{Name: "main", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}},
	})
	assert.False(t, IsContainerRunning(pod, "main"))
}

func TestIsContainerRunning_NotFound(t *testing.T) {
	pod := newTestPod([]corev1.ContainerStatus{})
	assert.False(t, IsContainerRunning(pod, "missing"))
}

func TestGetContainerRestartCount_Found(t *testing.T) {
	pod := newTestPod([]corev1.ContainerStatus{
		{Name: "sidecar", RestartCount: 3},
	})
	assert.Equal(t, int32(3), GetContainerRestartCount(pod, "sidecar"))
}

func TestGetContainerRestartCount_Zero(t *testing.T) {
	pod := newTestPod([]corev1.ContainerStatus{
		{Name: "main", RestartCount: 0},
	})
	assert.Equal(t, int32(0), GetContainerRestartCount(pod, "main"))
}

func TestGetContainerRestartCount_NotFound(t *testing.T) {
	pod := newTestPod([]corev1.ContainerStatus{})
	assert.Equal(t, int32(0), GetContainerRestartCount(pod, "missing"))
}

func TestIsContainerRunning_MultipleContainers(t *testing.T) {
	pod := newTestPod([]corev1.ContainerStatus{
		{Name: "sidecar", State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{}}},
		{Name: "main", State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}}},
	})
	assert.False(t, IsContainerRunning(pod, "sidecar"))
	assert.True(t, IsContainerRunning(pod, "main"))
}
