/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

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

// Namespace scope label constants
const (
	// TemplateScopeNamespaceLabel is the Namespace label key that controls template scope enforcement
	TemplateScopeNamespaceLabel = "workspace.jupyter.org/template-namespace-scope"
)

// TemplateScopeStrategy defines the template scope enforcement strategy for a namespace
type TemplateScopeStrategy string

const (
	// TemplateScopeNamespaced restricts workspaces to only use templates from their own namespace
	TemplateScopeNamespaced TemplateScopeStrategy = "Namespaced"

	// TemplateScopeCluster allows workspaces to use templates from any namespace (default)
	TemplateScopeCluster TemplateScopeStrategy = "Cluster"
)
