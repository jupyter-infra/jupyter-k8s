/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/api/equality"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	webhookconst "github.com/jupyter-infra/jupyter-k8s/internal/webhook"
)

// effectiveOwnershipType treats an empty ownership type as Public, the value the sharing defaulter
// gives it.
func effectiveOwnershipType(ownershipType string) string {
	if ownershipType == "" {
		return webhookconst.OwnershipTypePublic
	}
	return ownershipType
}

// effectiveAccessType treats an empty access type as the effective ownership type, the value the
// sharing defaulter gives it.
func effectiveAccessType(accessType, ownershipType string) string {
	if accessType == "" {
		return effectiveOwnershipType(ownershipType)
	}
	return accessType
}

// validatePinnedSharingTypes holds a workspace's accessType and ownershipType to the template's defaults
// when the template sets allowCustomAccessType or allowCustomOwnershipType to false.
func validatePinnedSharingTypes(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	var violations []TemplateViolation
	if template.Spec.AccessTypeLocked() {
		if v := pinnedSharingTypeViolation(ViolationTypeAccessTypeNotAllowed, "spec.accessType",
			effectiveAccessType(workspace.Spec.AccessType, workspace.Spec.OwnershipType),
			effectiveAccessType(template.Spec.DefaultAccessType, template.Spec.DefaultOwnershipType),
			template.Name); v != nil {
			violations = append(violations, *v)
		}
	}
	if template.Spec.OwnershipTypeLocked() {
		if v := pinnedSharingTypeViolation(ViolationTypeOwnershipTypeNotAllowed, "spec.ownershipType",
			effectiveOwnershipType(workspace.Spec.OwnershipType), effectiveOwnershipType(template.Spec.DefaultOwnershipType),
			template.Name); v != nil {
			violations = append(violations, *v)
		}
	}
	return violations
}

// pinnedSharingTypeViolation compares effective values, so an empty one means what the defaulter makes of it.
func pinnedSharingTypeViolation(violationType, field, actual, pinned, templateName string) *TemplateViolation {
	if actual == pinned {
		return nil
	}
	return &TemplateViolation{
		Type:    violationType,
		Field:   field,
		Message: fmt.Sprintf("%s: pinned to %s by template %q", field, pinned, templateName),
		Allowed: pinned,
		Actual:  actual,
	}
}

// validatePinnedSecurityContexts holds a workspace's podSecurityContext and containerSecurityContext to
// the template's defaults when the template's podSecurityContextOverrides or
// containerSecurityContextOverrides lock them. A workspace that sets nothing passes only when the
// template has no default either; otherwise the defaulter gives it the default.
func validatePinnedSecurityContexts(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	var violations []TemplateViolation
	if template.Spec.PodSecurityContextOverrides.Locked() &&
		!equality.Semantic.DeepEqual(workspace.Spec.PodSecurityContext, template.Spec.DefaultPodSecurityContext) {
		violations = append(violations, securityContextViolation("spec.podSecurityContext",
			template.Spec.DefaultPodSecurityContext != nil, template.Name))
	}
	if template.Spec.ContainerSecurityContextOverrides.Locked() &&
		!equality.Semantic.DeepEqual(workspace.Spec.ContainerSecurityContext, template.Spec.DefaultContainerSecurityContext) {
		violations = append(violations, securityContextViolation("spec.containerSecurityContext",
			template.Spec.DefaultContainerSecurityContext != nil, template.Name))
	}
	return violations
}

func securityContextViolation(field string, templateHasDefault bool, templateName string) TemplateViolation {
	allowed := "unset, the template has no default"
	if templateHasDefault {
		allowed = "the template's default"
	}
	return TemplateViolation{
		Type:    ViolationTypeSecurityContextOverrideNotAllowed,
		Field:   field,
		Message: fmt.Sprintf("%s: pinned to %s by template %q", field, allowed, templateName),
		Allowed: allowed,
		Actual:  "a custom security context",
	}
}
