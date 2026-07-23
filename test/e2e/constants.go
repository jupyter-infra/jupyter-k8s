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
	ConditionTypeDeleting    = "Deleting"
)

// WorkspaceLabelName is the label key used to identify workspace resources
const WorkspaceLabelName = "workspace.jupyter.org/workspace-name"

// WebhookCertificateName is the name of the webhook certificate
const WebhookCertificateName = "jupyter-k8s-serving-cert"

// Common string literal values used across e2e tests. Declared as constants to
// satisfy goconst (repeated string literals) and to keep usage consistent.
const (
	// valueTrue is the string representation of a boolean true value
	valueTrue = "true"
	// flagSet is the helm/kubectl "--set" flag
	flagSet = "--set"
	// verbGet is the kubectl "get" verb
	verbGet = "get"
	// verbCreate is the kubectl "create" verb
	verbCreate = "create"
	// phaseBound is the PVC "Bound" phase value
	phaseBound = "Bound"
	// accessModeRWO is the "ReadWriteOnce" PVC access mode value
	accessModeRWO = "ReadWriteOnce"
	// volumeNameTeamData is the shared team-data volume name
	volumeNameTeamData = "team-data"
	// pvcNameSharedTeamData is the shared-team-data PVC name
	pvcNameSharedTeamData = "shared-team-data"
	// systemMastersGroup is the Kubernetes super-user group used to impersonate an admin/exempt caller
	systemMastersGroup = "system:masters"
)

// JSONPath expressions used to query Kubernetes resources in e2e tests.
const (
	jsonPathPVCStorage    = "{.spec.resources.requests.storage}"
	jsonPathAccessMode    = "{.spec.accessModes[0]}"
	jsonPathOwnerRefName  = "{.metadata.ownerReferences[0].name}"
	jsonPathStatusPhase   = "{.status.phase}"
	jsonPathVolumeName    = "{.spec.volumes[0].name}"
	jsonPathVolumePVCName = "{.spec.volumes[0].persistentVolumeClaimName}"
)

// Description strings used in table-driven storage/template verification tests.
const (
	descVerifyingPVCSize       = "verifying pvc size"
	descVerifyingPVCAccessMode = "verifying pvc access mode"
	descVerifyingOwnerRef      = "verifying owner reference"
	descVerifyingBindingStatus = "verifying binding status"
)

// GetWorkspaceCrds returns an array with the workspace CRD names
func GetWorkspaceCrds() []string {
	return []string{
		"workspaces.workspace.jupyter.org",
		"workspacetemplates.workspace.jupyter.org",
		"workspaceaccessstrategies.workspace.jupyter.org",
	}
}
