/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// AccessStrategyValidator handles access strategy namespace validation for webhooks
type AccessStrategyValidator struct {
	sharedNamespace string
}

// NewAccessStrategyValidator creates a new AccessStrategyValidator
func NewAccessStrategyValidator(sharedNamespace string) *AccessStrategyValidator {
	return &AccessStrategyValidator{
		sharedNamespace: sharedNamespace,
	}
}

// validateNamespaceScope checks that an access strategy reference targets an allowed namespace.
// Both workspaces and templates may only reference access strategies from the referrer's own
// namespace or the configured shared namespace. referrerKind ("workspace" / "template") only
// shapes the error message; the rules are identical.
func (v *AccessStrategyValidator) validateNamespaceScope(asNamespace, referrerNamespace, referrerKind string) error {
	if asNamespace == "" || asNamespace == referrerNamespace {
		return nil
	}

	if v.sharedNamespace == "" {
		return fmt.Errorf(
			"accessStrategy.namespace %q is not allowed: access strategies must be in the %s namespace %q",
			asNamespace, referrerKind, referrerNamespace,
		)
	}

	if asNamespace == v.sharedNamespace {
		return nil
	}

	return fmt.Errorf(
		"accessStrategy.namespace %q is not allowed: access strategies must be in the %s namespace %q or the shared namespace %q",
		asNamespace, referrerKind, referrerNamespace, v.sharedNamespace,
	)
}

// validateAccessStrategyNamespace checks that accessStrategy.namespace targets an allowed namespace.
// Workspaces can only reference access strategies from their own namespace or the shared namespace.
func (v *AccessStrategyValidator) validateAccessStrategyNamespace(workspace *workspacev1alpha1.Workspace) error {
	if workspace.Spec.AccessStrategy == nil {
		return nil
	}
	return v.validateNamespaceScope(workspace.Spec.AccessStrategy.Namespace, workspace.Namespace, "workspace")
}

// validateTemplateAccessStrategyNamespace checks that a template's defaultAccessStrategy.namespace
// targets an allowed namespace. Templates can only reference access strategies from their own
// namespace or the shared namespace — the same rule workspaces are subject to. Enforcing it here
// prevents admins from creating templates that would make any referencing workspace un-admittable.
func (v *AccessStrategyValidator) validateTemplateAccessStrategyNamespace(template *workspacev1alpha1.WorkspaceTemplate) error {
	if template.Spec.DefaultAccessStrategy == nil {
		return nil
	}
	return v.validateNamespaceScope(template.Spec.DefaultAccessStrategy.Namespace, template.Namespace, "template")
}

// ValidateCreateWorkspace validates access strategy namespace on workspace creation
func (v *AccessStrategyValidator) ValidateCreateWorkspace(workspace *workspacev1alpha1.Workspace) error {
	return v.validateAccessStrategyNamespace(workspace)
}

// ValidateUpdateWorkspace validates access strategy namespace on workspace update.
// No special-casing needed — validateAccessStrategyNamespace already handles nil accessStrategy,
// which covers the "removed" case. The admission webhook is the single enforcement point.
func (v *AccessStrategyValidator) ValidateUpdateWorkspace(_, newWorkspace *workspacev1alpha1.Workspace) error {
	return v.validateAccessStrategyNamespace(newWorkspace)
}

// ValidateCreateTemplate validates the access strategy namespace on template creation.
func (v *AccessStrategyValidator) ValidateCreateTemplate(template *workspacev1alpha1.WorkspaceTemplate) error {
	return v.validateTemplateAccessStrategyNamespace(template)
}

// ValidateUpdateTemplate validates the access strategy namespace on template update.
// Like the workspace case, nil defaultAccessStrategy is handled (covers the "removed" case),
// so removing the reference is always allowed.
func (v *AccessStrategyValidator) ValidateUpdateTemplate(_, newTemplate *workspacev1alpha1.WorkspaceTemplate) error {
	return v.validateTemplateAccessStrategyNamespace(newTemplate)
}
