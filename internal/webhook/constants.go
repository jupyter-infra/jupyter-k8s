// Package webhook provides constants and utilities for webhook validation.
package webhook

// Access type constants
const (
	OwnershipTypeOwnerOnly = "OwnerOnly"
	OwnershipTypePublic    = "Public"
)

// Admin group constants
// Defined by Kubernetes: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#user-facing-roles
const (
	DefaultAdminGroup = "system:masters"
)

// Template label constants
const (
	DefaultClusterTemplateLabel = "workspace.jupyter.org/default-cluster-template"
	DefaultServiceAccountLabel  = "workspace.jupyter.org/default-service-account"
)
