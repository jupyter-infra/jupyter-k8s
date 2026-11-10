/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
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
//
// It handles templates whose security context defaults hold integers only. A default holding an
// identity template has no single value to compare against: the caller resolves it for the
// workspace's owner and namespace and calls validatePinnedSecurityContextsAgainst instead, so this
// function reports no violation for such a template.
func validatePinnedSecurityContexts(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate) []TemplateViolation {
	pod, podErr := template.Spec.DefaultPodSecurityContext.Resolve(workspacev1alpha1.StaticID)
	container, containerErr := template.Spec.DefaultContainerSecurityContext.Resolve(workspacev1alpha1.StaticID)
	if podErr != nil || containerErr != nil {
		// A template default holding identity templates is resolved and checked by the caller.
		return nil
	}
	return validatePinnedSecurityContextsAgainst(workspace, template, pod, container)
}

// validatePinnedSecurityContextsAgainst is validatePinnedSecurityContexts against the template's
// defaults already resolved for the workspace (identity templates rendered for its owner and
// namespace), so a locked context compares with the values the defaulter gave the workspace.
func validatePinnedSecurityContextsAgainst(
	workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate,
	pod *corev1.PodSecurityContext, container *corev1.SecurityContext,
) []TemplateViolation {
	var violations []TemplateViolation
	if template.Spec.PodSecurityContextOverrides.Locked() &&
		!equality.Semantic.DeepEqual(workspace.Spec.PodSecurityContext, pod) {
		violations = append(violations, securityContextViolation("spec.podSecurityContext",
			template.Spec.DefaultPodSecurityContext != nil, template.Name))
	}
	if template.Spec.ContainerSecurityContextOverrides.Locked() &&
		!equality.Semantic.DeepEqual(workspace.Spec.ContainerSecurityContext, container) {
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
