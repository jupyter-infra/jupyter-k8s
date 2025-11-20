/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
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
