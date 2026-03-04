/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// validateDefaultLabels checks if workspace labels match template's DefaultLabels
func validateDefaultLabels(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) *TemplateViolation {
	if template.Spec.DefaultLabels == nil || len(template.Spec.DefaultLabels) == 0 {
		return nil
	}

	if workspace.Labels == nil {
		workspace.Labels = make(map[string]string)
	}

	// Check each default label
	for key, expectedValue := range template.Spec.DefaultLabels {
		if actualValue, exists := workspace.Labels[key]; exists {
			if actualValue != expectedValue {
				return &TemplateViolation{
					Type:    ViolationTypeDefaultLabelMismatch,
					Field:   fmt.Sprintf("metadata.labels[%s]", key),
					Message: fmt.Sprintf("Label '%s' value must match template default", key),
					Allowed: expectedValue,
					Actual:  actualValue,
				}
			}
		}
	}

	return nil
}
