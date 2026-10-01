/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"path"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// isReservedVolumeName reports whether a volume name belongs to the operator's own volumes, which a
// workspace may not declare: the primary storage and the /dev/shm volume.
func isReservedVolumeName(name string) bool {
	return name == volumeNameWorkspaceStorage || name == volumeNameWorkspaceSharedMemory
}

// MountsSharedMemoryPath reports whether a mount path is /dev/shm, ignoring a trailing slash or a
// doubled separator, which Kubernetes would otherwise treat as a distinct path. The admission webhook
// uses the same test so policy and pod build agree on what counts as /dev/shm.
func MountsSharedMemoryPath(mountPath string) bool {
	return path.Clean(mountPath) == SharedMemoryMountPath
}

// sharedMemoryEnabled reports whether the pod gets the operator's /dev/shm volume: on unless the
// workspace disables it or one of its own volumes already mounts /dev/shm. A volume carrying a
// reserved name does not count, since the builder drops it.
func sharedMemoryEnabled(workspace *workspacev1alpha1.Workspace) bool {
	if sm := workspace.Spec.SharedMemory; sm != nil && sm.Enabled != nil && !*sm.Enabled {
		return false
	}
	for _, vol := range workspace.Spec.Volumes {
		if isReservedVolumeName(vol.Name) {
			continue
		}
		if MountsSharedMemoryPath(vol.MountPath) {
			return false
		}
	}
	return true
}

// sharedMemorySizeLimit returns the /dev/shm volume's sizeLimit. An explicit sharedMemory.sizeLimit is
// used as given, lowered to the container's memory limit when it exceeds it, since the volume's contents
// count against that limit. Without one, the size is the memory limit, or the memory request when the
// container has no limit; with neither the result is nil and the kubelet applies the pod or node bound.
func sharedMemorySizeLimit(workspace *workspacev1alpha1.Workspace, resources corev1.ResourceRequirements) *resource.Quantity {
	limit, hasLimit := resources.Limits[corev1.ResourceMemory]

	var size *resource.Quantity
	if sm := workspace.Spec.SharedMemory; sm != nil && sm.SizeLimit != nil && sm.SizeLimit.Sign() > 0 {
		size = sm.SizeLimit
		if hasLimit && limit.Cmp(*size) < 0 {
			size = &limit
		}
	} else if hasLimit {
		size = &limit
	} else if request, hasRequest := resources.Requests[corev1.ResourceMemory]; hasRequest {
		size = &request
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

// dropShadowedSharedMemory removes the operator's /dev/shm volume and mount when an access strategy or
// integration template has mounted another volume at /dev/shm in the primary container. Two mounts at
// one path make the Deployment invalid, and the overlay's choice wins the way a workspace volume does.
func dropShadowedSharedMemory(podSpec *corev1.PodSpec) {
	if len(podSpec.Containers) == 0 {
		return
	}
	primary := &podSpec.Containers[0]
	shadowed := false
	for _, mount := range primary.VolumeMounts {
		if mount.Name != volumeNameWorkspaceSharedMemory && MountsSharedMemoryPath(mount.MountPath) {
			shadowed = true
			break
		}
	}
	if !shadowed {
		return
	}
	mounts := make([]corev1.VolumeMount, 0, len(primary.VolumeMounts))
	for _, mount := range primary.VolumeMounts {
		if mount.Name != volumeNameWorkspaceSharedMemory {
			mounts = append(mounts, mount)
		}
	}
	primary.VolumeMounts = mounts
	volumes := make([]corev1.Volume, 0, len(podSpec.Volumes))
	for _, volume := range podSpec.Volumes {
		if volume.Name != volumeNameWorkspaceSharedMemory {
			volumes = append(volumes, volume)
		}
	}
	podSpec.Volumes = volumes
}
