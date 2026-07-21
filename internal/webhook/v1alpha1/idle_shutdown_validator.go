/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"
	"reflect"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// validateIdleShutdownOverrides enforces the template's idleShutdownOverrides policy.
//
// The policy has two orthogonal knobs:
//
//   - Allow governs the STRUCTURE. When Allow=false, the workspace may not deviate from the
//     template's defaultIdleShutdown: every field must match the default EXCEPT
//     idleTimeoutInMinutes. When Allow is unset or true (the kubebuilder default), the
//     workspace may freely choose its idle shutdown config, including disabling it.
//
//   - MinIdleTimeoutInMinutes/MaxIdleTimeoutInMinutes bound the timeout VALUE. Declared bounds
//     are enforced whenever the workspace has idle shutdown enabled, regardless of Allow — so
//     an admin can permit overrides yet still cap the timeout.
//
// Missing-bound fallback differs by intent:
//
//   - Allow=false (locked window): an unset bound falls back to the default timeout, pinning
//     the unspecified side to the default ("the only wiggle room is what I explicitly granted").
//   - Allow=true/unset (permissive): an unset bound means unbounded on that side.
//
// The structural lock requires a defaultIdleShutdown to compare against; without one there is
// no baseline to match, so only the timeout bounds apply.
func validateIdleShutdownOverrides(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	policy := template.Spec.IdleShutdownOverrides
	if policy == nil {
		return nil
	}

	allowOverride := policy.Allow == nil || *policy.Allow
	def := template.Spec.DefaultIdleShutdown
	ws := workspace.Spec.IdleShutdown

	var violations []TemplateViolation

	// Structural lock: when overrides are not allowed, the workspace must match the template
	// default on every field except the timeout. Requires a default baseline to compare against.
	if !allowOverride && def != nil {
		if ws == nil {
			// Overrides are not allowed but the workspace carries no idle shutdown config,
			// which drops the template's policy entirely.
			return []TemplateViolation{{
				Type:    ViolationTypeIdleShutdownOverrideNotAllowed,
				Field:   "spec.idleShutdown",
				Message: fmt.Sprintf("Template '%s' does not allow overriding idle shutdown; idleShutdown must match the template default", template.Name),
				Allowed: "match template defaultIdleShutdown",
				Actual:  "unset",
			}}
		}

		// Compare copies with the timeout zeroed so a permitted timeout difference doesn't trip
		// the equality check.
		wsCopy := ws.DeepCopy()
		defCopy := def.DeepCopy()
		wsCopy.IdleTimeoutInMinutes = 0
		defCopy.IdleTimeoutInMinutes = 0
		if !reflect.DeepEqual(wsCopy, defCopy) {
			violations = append(violations, TemplateViolation{
				Type:    ViolationTypeIdleShutdownOverrideNotAllowed,
				Field:   "spec.idleShutdown",
				Message: fmt.Sprintf("Template '%s' does not allow overriding idle shutdown; only idleTimeoutInMinutes may differ from the template default", template.Name),
				Allowed: "match template defaultIdleShutdown (except idleTimeoutInMinutes)",
				Actual:  "differs from template default",
			})
		}
	}

	// Timeout bounds apply whenever the workspace has idle shutdown enabled: a disabled or absent
	// idle shutdown has no active timeout to bound.
	if ws != nil && ws.Enabled {
		if violation := validateIdleTimeoutBounds(ws.IdleTimeoutInMinutes, policy, def, allowOverride, template.Name); violation != nil {
			violations = append(violations, *violation)
		}
	}

	return violations
}

// validateIdleTimeoutBounds checks the workspace idle timeout against the policy's declared
// bounds, applying the missing-bound fallback appropriate to the Allow setting. Returns nil when
// the timeout is within bounds (or no bound applies to the relevant side).
func validateIdleTimeoutBounds(
	timeout int,
	policy *workspacev1alpha1.IdleShutdownOverridePolicy,
	def *workspacev1alpha1.IdleShutdownSpec,
	allowOverride bool,
	templateName string,
) *TemplateViolation {
	minTimeout, hasMin := 0, false
	if policy.MinIdleTimeoutInMinutes != nil {
		minTimeout, hasMin = *policy.MinIdleTimeoutInMinutes, true
	}
	maxTimeout, hasMax := 0, false
	if policy.MaxIdleTimeoutInMinutes != nil {
		maxTimeout, hasMax = *policy.MaxIdleTimeoutInMinutes, true
	}

	// When overrides are locked, an unset bound falls back to the default timeout, pinning the
	// unspecified side to the default. When overrides are allowed, an unset bound stays unbounded.
	if !allowOverride && def != nil {
		if !hasMin {
			minTimeout, hasMin = def.IdleTimeoutInMinutes, true
		}
		if !hasMax {
			maxTimeout, hasMax = def.IdleTimeoutInMinutes, true
		}
	}

	belowMin := hasMin && timeout < minTimeout
	aboveMax := hasMax && timeout > maxTimeout
	if !belowMin && !aboveMax {
		return nil
	}

	minStr := "unbounded"
	if hasMin {
		minStr = fmt.Sprintf("%d", minTimeout)
	}
	maxStr := "unbounded"
	if hasMax {
		maxStr = fmt.Sprintf("%d", maxTimeout)
	}

	return &TemplateViolation{
		Type:    ViolationTypeIdleShutdownTimeoutOutOfBounds,
		Field:   "spec.idleShutdown.idleTimeoutInMinutes",
		Message: fmt.Sprintf("Idle timeout %d minutes is outside the range [%s, %s] allowed by template '%s'", timeout, minStr, maxStr, templateName),
		Allowed: fmt.Sprintf("min: %s, max: %s", minStr, maxStr),
		Actual:  fmt.Sprintf("%d", timeout),
	}
}
