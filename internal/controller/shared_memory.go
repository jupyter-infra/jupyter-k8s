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

// sharedMemoryEnabled reports whether the pod gets the operator's /dev/shm volume: only when the
// workspace asks for it with sharedMemory set, by itself or copied from its template's
// defaultSharedMemory, not disabled, and none of its own volumes already mounts /dev/shm. A volume
// carrying a reserved name does not count, since the builder drops it.
func sharedMemoryEnabled(workspace *workspacev1alpha1.Workspace) bool {
	if !workspace.Spec.SharedMemory.IsEnabled() {
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

// sharedMemorySizeLimit returns the /dev/shm volume's sizeLimit: the container's memory limit, since the
// volume's contents count against it, or the memory request when the container has no limit. With
// neither the result is nil and no volume is mounted, because a tmpfs without a size would be bounded
// only by the node.
func sharedMemorySizeLimit(resources corev1.ResourceRequirements) *resource.Quantity {
	var size *resource.Quantity
	if limit, hasLimit := resources.Limits[corev1.ResourceMemory]; hasLimit {
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

// sharedMemoryMounted reports whether the pod gets the operator's /dev/shm volume: enabled for the
// workspace and with a size that can be derived from it.
func sharedMemoryMounted(workspace *workspacev1alpha1.Workspace, resources corev1.ResourceRequirements) bool {
	return sharedMemoryEnabled(workspace) && sharedMemorySizeLimit(resources) != nil
}

// sharedMemoryVolume builds the memory-backed emptyDir volume mounted at /dev/shm.
func sharedMemoryVolume(resources corev1.ResourceRequirements) corev1.Volume {
	return corev1.Volume{
		Name: volumeNameWorkspaceSharedMemory,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium:    corev1.StorageMediumMemory,
				SizeLimit: sharedMemorySizeLimit(resources),
			},
		},
	}
}

// dropShadowedSharedMemory removes the operator's /dev/shm volume and mount when an access strategy or
// integration template has mounted another volume at /dev/shm in the primary container. Two mounts at
// one path make the Deployment invalid, and the overlay's choice wins the way a workspace volume does.
func dropShadowedSharedMemory(podSpec *corev1.PodSpec) {
	primary := findPrimaryContainer(podSpec)
	if primary == nil {
		return
	}
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
