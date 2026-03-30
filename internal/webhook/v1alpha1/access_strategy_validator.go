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

// validateAccessStrategyNamespace checks that accessStrategy.namespace targets an allowed namespace.
// Workspaces can only reference access strategies from their own namespace or the shared namespace.
func (v *AccessStrategyValidator) validateAccessStrategyNamespace(workspace *workspacev1alpha1.Workspace) error {
	if workspace.Spec.AccessStrategy == nil {
		return nil
	}

	asNamespace := workspace.Spec.AccessStrategy.Namespace
	workspaceNamespace := workspace.Namespace

	if asNamespace == "" || asNamespace == workspaceNamespace {
		return nil
	}

	if v.sharedNamespace == "" {
		return fmt.Errorf(
			"accessStrategy.namespace %q is not allowed: access strategies must be in the workspace namespace %q",
			asNamespace, workspaceNamespace,
		)
	}

	if asNamespace == v.sharedNamespace {
		return nil
	}

	return fmt.Errorf(
		"accessStrategy.namespace %q is not allowed: access strategies must be in the workspace namespace %q or the shared namespace %q",
		asNamespace, workspaceNamespace, v.sharedNamespace,
	)
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
