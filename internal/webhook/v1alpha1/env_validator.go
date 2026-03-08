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

// validateEnvRequirements checks workspace env vars against template's EnvRequirements
func validateEnvRequirements(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	if len(template.Spec.EnvRequirements) == 0 {
		return nil
	}

	// Build lookup of workspace env vars by name
	envMap := make(map[string]string, len(workspace.Spec.Env))
	for _, e := range workspace.Spec.Env {
		envMap[e.Name] = e.Value
	}

	var violations []TemplateViolation

	for _, req := range template.Spec.EnvRequirements {
		value, exists := envMap[req.Name]

		// Check required
		if req.Required != nil && *req.Required && !exists {
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeEnvRequired,
				Field:   fmt.Sprintf("spec.env[%s]", req.Name),
				Message: fmt.Sprintf("Environment variable '%s' is required by template", req.Name),
			})
			continue
		}

		// Check regex (only if env var exists and regex is set)
		if exists && req.Regex != "" {
			matched, err := regexp.MatchString(req.Regex, value)
			if err != nil {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeEnvRegexMismatch,
					Field:   fmt.Sprintf("spec.env[%s]", req.Name),
					Message: fmt.Sprintf("Environment variable '%s' has invalid regex in template: %s", req.Name, err.Error()),
				})
				continue
			}
			if !matched {
				violations = append(violations, TemplateViolation{
					Type:    ViolationTypeEnvRegexMismatch,
					Field:   fmt.Sprintf("spec.env[%s]", req.Name),
					Message: fmt.Sprintf("Environment variable '%s' value does not match required pattern", req.Name),
					Allowed: req.Regex,
					Actual:  value,
				})
			}
		}
	}

	return violations
}
