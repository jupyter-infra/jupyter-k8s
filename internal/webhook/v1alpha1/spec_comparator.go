/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"reflect"

	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// specChanged detects if any spec field changed between old and new workspace
// This is used to determine if full template validation is needed on update
func specChanged(oldSpec, newSpec *workspacev1alpha1.WorkspaceSpec) bool {
	// DisplayName changed
	if oldSpec.DisplayName != newSpec.DisplayName {
		return true
	}

	// Image changed
	if oldSpec.Image != newSpec.Image {
		return true
	}

	// DesiredStatus changed
	if oldSpec.DesiredStatus != newSpec.DesiredStatus {
		return true
	}

	// OwnershipType changed
	if oldSpec.OwnershipType != newSpec.OwnershipType {
		return true
	}

	// AccessType changed
	if oldSpec.AccessType != newSpec.AccessType {
		return true
	}

	// Resources changed
	if !resourcesEqual(oldSpec.Resources, newSpec.Resources) {
		return true
	}

	// Storage changed
	if !storageEqual(oldSpec.Storage, newSpec.Storage) {
		return true
	}

	// Volumes changed
	if !volumesEqual(oldSpec.Volumes, newSpec.Volumes) {
		return true
	}

	// ContainerConfig changed
	if !containerConfigEqual(oldSpec.ContainerConfig, newSpec.ContainerConfig) {
		return true
	}

	// NodeSelector changed
	if !nodeSelectorEqual(oldSpec.NodeSelector, newSpec.NodeSelector) {
		return true
	}

	// Affinity changed
	if !affinityEqual(oldSpec.Affinity, newSpec.Affinity) {
		return true
	}

	// Tolerations changed
	if !tolerationsEqual(oldSpec.Tolerations, newSpec.Tolerations) {
		return true
	}

	// Lifecycle changed
	if !lifecycleEqual(oldSpec.Lifecycle, newSpec.Lifecycle) {
		return true
	}

	// AccessStrategy changed
	if !accessStrategyEqual(oldSpec.AccessStrategy, newSpec.AccessStrategy) {
		return true
	}

	// TemplateRef changed
	if !templateRefEqual(oldSpec.TemplateRef, newSpec.TemplateRef) {
		return true
	}

	// IdleShutdown changed
	if !idleShutdownEqual(oldSpec.IdleShutdown, newSpec.IdleShutdown) {
		return true
	}

	// AppType changed
	if oldSpec.AppType != newSpec.AppType {
		return true
	}

	// ServiceAccountName changed
	if oldSpec.ServiceAccountName != newSpec.ServiceAccountName {
		return true
	}

	return false
}

// templateRefEqual compares two TemplateRef for equality
func templateRefEqual(old, new *workspacev1alpha1.TemplateRef) bool {
	if old == nil && new == nil {
		return true
	}
	if old == nil || new == nil {
		return false
	}
	return old.Name == new.Name && old.Namespace == new.Namespace
}

// volumesEqual compares two slices of VolumeSpec for equality
func volumesEqual(old, new []workspacev1alpha1.VolumeSpec) bool {
	if len(old) != len(new) {
		return false
	}
	for i := range old {
		if old[i].Name != new[i].Name ||
			old[i].PersistentVolumeClaimName != new[i].PersistentVolumeClaimName ||
			old[i].MountPath != new[i].MountPath {
			return false
		}
	}
	return true
}

// containerConfigEqual compares two ContainerConfig for equality
func containerConfigEqual(old, new *workspacev1alpha1.ContainerConfig) bool {
	if old == nil && new == nil {
		return true
	}
	if old == nil || new == nil {
		return false
	}
	return stringSlicesEqual(old.Command, new.Command) && stringSlicesEqual(old.Args, new.Args)
}

// stringSlicesEqual compares two string slices for equality
func stringSlicesEqual(old, new []string) bool {
	if len(old) != len(new) {
		return false
	}
	for i := range old {
		if old[i] != new[i] {
			return false
		}
	}
	return true
}

// nodeSelectorEqual compares two node selectors for equality
func nodeSelectorEqual(old, new map[string]string) bool {
	if len(old) != len(new) {
		return false
	}
	for key, oldVal := range old {
		if newVal, exists := new[key]; !exists || newVal != oldVal {
			return false
		}
	}
	return true
}

// affinityEqual compares two Affinity objects for equality using deep equality
func affinityEqual(old, new *corev1.Affinity) bool {
	return reflect.DeepEqual(old, new)
}

// tolerationsEqual compares two slices of Toleration for equality using deep equality
func tolerationsEqual(old, new []corev1.Toleration) bool {
	return reflect.DeepEqual(old, new)
}

// lifecycleEqual compares two Lifecycle objects for equality using deep equality
func lifecycleEqual(old, new *corev1.Lifecycle) bool {
	return reflect.DeepEqual(old, new)
}

// accessStrategyEqual compares two AccessStrategyRef for equality
func accessStrategyEqual(old, new *workspacev1alpha1.AccessStrategyRef) bool {
	if old == nil && new == nil {
		return true
	}
	if old == nil || new == nil {
		return false
	}
	return old.Name == new.Name && old.Namespace == new.Namespace
}

// idleShutdownEqual compares two IdleShutdownSpec for equality using deep equality
// This covers Enabled, TimeoutMinutes, and Detection (which includes HTTPGet)
func idleShutdownEqual(old, new *workspacev1alpha1.IdleShutdownSpec) bool {
	return reflect.DeepEqual(old, new)
}

// onlyDesiredStatusChanged checks if DesiredStatus is the ONLY spec field that changed
// Used to determine if stopping a workspace should bypass validation
// Returns true only when DesiredStatus changed and all other fields remain unchanged
func onlyDesiredStatusChanged(oldSpec, newSpec *workspacev1alpha1.WorkspaceSpec) bool {
	// DesiredStatus must have changed
	if oldSpec.DesiredStatus == newSpec.DesiredStatus {
		return false
	}

	// All other fields must be unchanged
	if oldSpec.DisplayName != newSpec.DisplayName {
		return false
	}

	if oldSpec.Image != newSpec.Image {
		return false
	}

	if oldSpec.OwnershipType != newSpec.OwnershipType {
		return false
	}

	if oldSpec.AccessType != newSpec.AccessType {
		return false
	}

	if !resourcesEqual(oldSpec.Resources, newSpec.Resources) {
		return false
	}

	if !storageEqual(oldSpec.Storage, newSpec.Storage) {
		return false
	}

	if !volumesEqual(oldSpec.Volumes, newSpec.Volumes) {
		return false
	}

	if !containerConfigEqual(oldSpec.ContainerConfig, newSpec.ContainerConfig) {
		return false
	}

	if !nodeSelectorEqual(oldSpec.NodeSelector, newSpec.NodeSelector) {
		return false
	}

	if !affinityEqual(oldSpec.Affinity, newSpec.Affinity) {
		return false
	}

	if !tolerationsEqual(oldSpec.Tolerations, newSpec.Tolerations) {
		return false
	}

	if !lifecycleEqual(oldSpec.Lifecycle, newSpec.Lifecycle) {
		return false
	}

	if !accessStrategyEqual(oldSpec.AccessStrategy, newSpec.AccessStrategy) {
		return false
	}

	if !templateRefEqual(oldSpec.TemplateRef, newSpec.TemplateRef) {
		return false
	}

	if !idleShutdownEqual(oldSpec.IdleShutdown, newSpec.IdleShutdown) {
		return false
	}

	if oldSpec.AppType != newSpec.AppType {
		return false
	}

	if oldSpec.ServiceAccountName != newSpec.ServiceAccountName {
		return false
	}

	return true
}
