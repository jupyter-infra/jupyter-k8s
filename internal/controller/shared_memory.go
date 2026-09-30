/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// sharedMemoryEnabled reports whether the pod gets the operator's /dev/shm volume: on unless the
// workspace disables it or already mounts one of its own volumes at /dev/shm.
func sharedMemoryEnabled(workspace *workspacev1alpha1.Workspace) bool {
	if sm := workspace.Spec.SharedMemory; sm != nil && sm.Enabled != nil && !*sm.Enabled {
		return false
	}
	for _, vol := range workspace.Spec.Volumes {
		if vol.MountPath == sharedMemoryMountPath {
			return false
		}
	}
	return true
}

// sharedMemorySizeLimit returns the /dev/shm volume's sizeLimit: the container's memory limit,
// lowered to the workspace's sharedMemory.sizeLimit when that is smaller. Without a limit the memory
// request stands in; without either the kubelet caps the volume at the pod or node level.
func sharedMemorySizeLimit(workspace *workspacev1alpha1.Workspace, resources corev1.ResourceRequirements) *resource.Quantity {
	var size *resource.Quantity
	if limit, ok := resources.Limits[corev1.ResourceMemory]; ok {
		size = &limit
	} else if request, ok := resources.Requests[corev1.ResourceMemory]; ok {
		size = &request
	}
	if sm := workspace.Spec.SharedMemory; sm != nil && sm.SizeLimit != nil && sm.SizeLimit.Sign() > 0 {
		if size == nil || sm.SizeLimit.Cmp(*size) < 0 {
			size = sm.SizeLimit
		}
	}
	if size == nil {
		return nil
	}
	copied := size.DeepCopy()
	return &copied
}

// sharedMemoryVolume builds the memory-backed emptyDir volume mounted at /dev/shm.
func sharedMemoryVolume(workspace *workspacev1alpha1.Workspace, resources corev1.ResourceRequirements) corev1.Volume {
	return corev1.Volume{
		Name: volumeNameWorkspaceSharedMemory,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium:    corev1.StorageMediumMemory,
				SizeLimit: sharedMemorySizeLimit(workspace, resources),
			},
		},
	}
}
