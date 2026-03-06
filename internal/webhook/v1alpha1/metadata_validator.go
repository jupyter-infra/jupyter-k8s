/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"
	"regexp"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// validateLabelRequirements checks workspace labels against template's LabelRequirements
func validateLabelRequirements(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	if len(template.Spec.LabelRequirements) == 0 {
		return nil
	}

	var violations []TemplateViolation

	for _, req := range template.Spec.LabelRequirements {
		value, exists := workspace.Labels[req.Key]

		// Check required
		if req.Required != nil && *req.Required && !exists {
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeLabelRequired,
				Field:   fmt.Sprintf("metadata.labels[%s]", req.Key),
				Message: fmt.Sprintf("Label '%s' is required by template", req.Key),
			})
			continue
		}

		// Check regex (only if label exists and regex is set)
		if exists && req.Regex != "" {
			matched, err := regexp.MatchString(req.Regex, value)
			if err != nil {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeLabelRegexMismatch,
					Field:   fmt.Sprintf("metadata.labels[%s]", req.Key),
					Message: fmt.Sprintf("Label '%s' has invalid regex in template: %s", req.Key, err.Error()),
				})
				continue
			}
			if !matched {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeLabelRegexMismatch,
					Field:   fmt.Sprintf("metadata.labels[%s]", req.Key),
					Message: fmt.Sprintf("Label '%s' value does not match required pattern", req.Key),
					Allowed: req.Regex,
					Actual:  value,
				})
			}
		}
	}

	return violations
}
