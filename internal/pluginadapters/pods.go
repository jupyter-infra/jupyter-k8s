/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginadapters

import (
	"context"

	corev1 "k8s.io/api/core/v1"
)

// PodExecInterface defines the interface for executing commands in pods.
type PodExecInterface interface {
	ExecInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (string, error)
}

// IsContainerRunning checks if a container is in running state.
func IsContainerRunning(pod *corev1.Pod, containerName string) bool {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == containerName {
			return cs.State.Running != nil
		}
	}
	return false
}

// GetContainerRestartCount returns the restart count for a named container.
func GetContainerRestartCount(pod *corev1.Pod, containerName string) int32 {
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Name == containerName {
			return cs.RestartCount
		}
	}
	return 0
}
