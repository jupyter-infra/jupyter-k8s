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
// workspace disables it or one of its own volumes already mounts /dev/shm. A volume carrying the
// reserved name does not count, since the builder drops it.
func sharedMemoryEnabled(workspace *workspacev1alpha1.Workspace) bool {
	if sm := workspace.Spec.SharedMemory; sm != nil && sm.Enabled != nil && !*sm.Enabled {
		return false
	}
	for _, vol := range workspace.Spec.Volumes {
		if vol.Name == volumeNameWorkspaceSharedMemory {
			continue
		}
		if vol.MountPath == sharedMemoryMountPath {
			return false
		}
	}
	return true
}

// sharedMemorySizeLimit returns the /dev/shm volume's sizeLimit: the container's memory limit, or the
// workspace's sharedMemory.sizeLimit when that is smaller. A container without a memory limit uses its
// memory request; with neither the result is nil and the kubelet applies the pod or node bound.
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
