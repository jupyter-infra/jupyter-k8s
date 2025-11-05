/*
MIT License

Copyright (c) 2025 jupyter-ai-contrib

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
*/

package controller

import (
	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// constraintsChanged checks if any constraint fields changed between old and new templates
// Constraint fields are those that affect workspace validation (resource bounds, allowed images, etc.)
func constraintsChanged(oldTemplate, newTemplate *workspacev1alpha1.WorkspaceTemplate) bool {
	oldSpec := &oldTemplate.Spec
	newSpec := &newTemplate.Spec

	// Check AllowedImages changes
	if !stringSlicesEqual(oldSpec.AllowedImages, newSpec.AllowedImages) {
		return true
	}

	// Check AllowCustomImages changes
	if (oldSpec.AllowCustomImages == nil) != (newSpec.AllowCustomImages == nil) {
		return true
	}
	if oldSpec.AllowCustomImages != nil && newSpec.AllowCustomImages != nil {
		if *oldSpec.AllowCustomImages != *newSpec.AllowCustomImages {
			return true
		}
	}

	// Check ResourceBounds changes
	if resourceBoundsChanged(oldSpec.ResourceBounds, newSpec.ResourceBounds) {
		return true
	}

	// Check PrimaryStorage.MaxSize changes
	if maxStorageSizeChanged(oldSpec.PrimaryStorage, newSpec.PrimaryStorage) {
		return true
	}

	// Check IdleShutdownOverrides.Allow changes
	if idleShutdownAllowOverrideChanged(oldSpec.IdleShutdownOverrides, newSpec.IdleShutdownOverrides) {
		return true
	}

	// Check IdleShutdownOverrides timeout bounds changes
	if idleShutdownTimeoutBoundsChanged(oldSpec.IdleShutdownOverrides, newSpec.IdleShutdownOverrides) {
		return true
	}

	return false
}

// resourceBoundsChanged checks if resource bounds changed
func resourceBoundsChanged(oldBounds, newBounds *workspacev1alpha1.ResourceBounds) bool {
	// If one is nil and the other isn't, they're different
	if (oldBounds == nil) != (newBounds == nil) {
		return true
	}

	// Both nil means no change
	if oldBounds == nil {
		return false
	}

	// Check CPU bounds
	if resourceRangeChanged(oldBounds.CPU, newBounds.CPU) {
		return true
	}

	// Check Memory bounds
	if resourceRangeChanged(oldBounds.Memory, newBounds.Memory) {
		return true
	}

	// Check GPU bounds
	if resourceRangeChanged(oldBounds.GPU, newBounds.GPU) {
		return true
	}

	return false
}

// resourceRangeChanged checks if a resource range changed
func resourceRangeChanged(oldRange, newRange *workspacev1alpha1.ResourceRange) bool {
	// If one is nil and the other isn't, they're different
	if (oldRange == nil) != (newRange == nil) {
		return true
	}

	// Both nil means no change
	if oldRange == nil {
		return false
	}

	// Compare Min and Max
	return !oldRange.Min.Equal(newRange.Min) || !oldRange.Max.Equal(newRange.Max)
}

// maxStorageSizeChanged checks if max storage size changed
func maxStorageSizeChanged(oldStorage, newStorage *workspacev1alpha1.StorageConfig) bool {
	// If one is nil and the other isn't, they're different
	if (oldStorage == nil) != (newStorage == nil) {
		return true
	}

	// Both nil means no change
	if oldStorage == nil {
		return false
	}

	// Check MaxSize changes
	if (oldStorage.MaxSize == nil) != (newStorage.MaxSize == nil) {
		return true
	}

	if oldStorage.MaxSize != nil && !oldStorage.MaxSize.Equal(*newStorage.MaxSize) {
		return true
	}

	// Check MinSize changes (also affects validation)
	if (oldStorage.MinSize == nil) != (newStorage.MinSize == nil) {
		return true
	}

	if oldStorage.MinSize != nil && !oldStorage.MinSize.Equal(*newStorage.MinSize) {
		return true
	}

	return false
}

// idleShutdownAllowOverrideChanged checks if Allow setting changed
func idleShutdownAllowOverrideChanged(oldOverrides, newOverrides *workspacev1alpha1.IdleShutdownOverridePolicy) bool {
	// If one is nil and the other isn't, they're different
	if (oldOverrides == nil) != (newOverrides == nil) {
		return true
	}

	// Both nil means no change
	if oldOverrides == nil {
		return false
	}

	// Check Allow changes
	if (oldOverrides.Allow == nil) != (newOverrides.Allow == nil) {
		return true
	}

	if oldOverrides.Allow != nil && *oldOverrides.Allow != *newOverrides.Allow {
		return true
	}

	return false
}

// idleShutdownTimeoutBoundsChanged checks if timeout bounds changed
func idleShutdownTimeoutBoundsChanged(oldOverrides, newOverrides *workspacev1alpha1.IdleShutdownOverridePolicy) bool {
	// If one is nil and the other isn't, they're different
	if (oldOverrides == nil) != (newOverrides == nil) {
		return true
	}

	// Both nil means no change
	if oldOverrides == nil {
		return false
	}

	// Check MinTimeoutMinutes
	if (oldOverrides.MinTimeoutMinutes == nil) != (newOverrides.MinTimeoutMinutes == nil) {
		return true
	}
	if oldOverrides.MinTimeoutMinutes != nil && *oldOverrides.MinTimeoutMinutes != *newOverrides.MinTimeoutMinutes {
		return true
	}

	// Check MaxTimeoutMinutes
	if (oldOverrides.MaxTimeoutMinutes == nil) != (newOverrides.MaxTimeoutMinutes == nil) {
		return true
	}
	if oldOverrides.MaxTimeoutMinutes != nil && *oldOverrides.MaxTimeoutMinutes != *newOverrides.MaxTimeoutMinutes {
		return true
	}

	return false
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
