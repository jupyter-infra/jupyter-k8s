//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package e2e contains end-to-end tests for the Jupyter K8s operator.
// These tests use a real Kubernetes cluster and verify actual behavior.
package e2e

// ServiceAccountName is the name of the service account used by the controller
const ServiceAccountName = "jupyter-k8s-controller-manager"

// MetricsServiceName is the name of the service account used by the metrics service
const MetricsServiceName = "jupyter-k8s-controller-manager-metrics-service"

// MetricsRoleBindingName is the name of the role binding to the metrics service accoun t
const MetricsRoleBindingName = "jupyter-k8s-metrics-binding"

// OperatorNamespace for controller and system components
const OperatorNamespace = "jupyter-k8s-system"

// SharedNamespace for template and access strategies
const SharedNamespace = "jupyter-k8s-shared"

// ConditionTrue represents the "True" string value for Kubernetes condition status
const ConditionTrue = "True"

// ConditionFalse represents the "False" string value for Kubernetes condition status
const ConditionFalse = "False"

// Workspace condition type names
const (
	ConditionTypeProgressing = "Progressing"
	ConditionTypeDegraded    = "Degraded"
	ConditionTypeAvailable   = "Available"
	ConditionTypeStopped     = "Stopped"
)

// WorkspaceLabelName is the label key used to identify workspace resources
const WorkspaceLabelName = "workspace.jupyter.org/workspace-name"

// WebhookCertificateName is the name of the webhook certificate
const WebhookCertificateName = "jupyter-k8s-serving-cert"

// GetWorkspaceCrds returns an array with the workspace CRD names
func GetWorkspaceCrds() []string {
	return []string{
		"workspaces.workspace.jupyter.org",
		"workspacetemplates.workspace.jupyter.org",
		"workspaceaccessstrategies.workspace.jupyter.org",
	}
}
