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
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// validateStorageSize checks if storage size is within template bounds
func validateStorageSize(size resource.Quantity, template *workspacev1alpha1.WorkspaceTemplate) *TemplateViolation {
	config := template.Spec.PrimaryStorage
	if config == nil {
		return nil
	}

	if config.MinSize != nil && size.Cmp(*config.MinSize) < 0 {
		return &TemplateViolation{
			Type:    ViolationTypeStorageExceeded,
			Field:   "spec.storage.size",
			Message: fmt.Sprintf("Storage size %s is below minimum %s required by template '%s'", size.String(), config.MinSize.String(), template.Name),
			Allowed: fmt.Sprintf("min: %s", config.MinSize.String()),
			Actual:  size.String(),
		}
	}

	if config.MaxSize != nil && size.Cmp(*config.MaxSize) > 0 {
		return &TemplateViolation{
			Type:    ViolationTypeStorageExceeded,
			Field:   "spec.storage.size",
			Message: fmt.Sprintf("Storage size %s exceeds maximum %s allowed by template '%s'", size.String(), config.MaxSize.String(), template.Name),
			Allowed: fmt.Sprintf("max: %s", config.MaxSize.String()),
			Actual:  size.String(),
		}
	}

	return nil
}

// storageEqual compares two StorageSpec for equality
func storageEqual(old, new *workspacev1alpha1.StorageSpec) bool {
	if old == nil && new == nil {
		return true
	}
	if old == nil || new == nil {
		return false
	}

	return old.Size.Equal(new.Size) && old.MountPath == new.MountPath
}
