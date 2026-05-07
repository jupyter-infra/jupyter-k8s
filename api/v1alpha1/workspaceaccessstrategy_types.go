/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AccessResourceTemplate defines a template for creating Kubernetes resources
type AccessResourceTemplate struct {
	// Kind of the Kubernetes resource to create
	Kind string `json:"kind"`

	// ApiVersion of the Kubernetes resource
	ApiVersion string `json:"apiVersion"`

	// NamePrefix is a prefix for the resource name
	// The name will be constructed as {NamePrefix}-{workspace.metadata.name}
	NamePrefix string `json:"namePrefix"`

	// Template is a YAML template string for the resource
	// Template variables include Workspace, AccessStrategy and Service objects
	Template string `json:"template"`
}

// AccessEnvTemplate defines a template for environment variables
type AccessEnvTemplate struct {
	// Name of the environment variable
	Name string `json:"name"`

	// ValueTemplate is a template string for the value
	// Can use variables from the Workspace or AccessStrategy objects
	// but not the Service object
	ValueTemplate string `json:"valueTemplate"`
}

// DeploymentModifications defines modifications to apply to deployment spec
type DeploymentModifications struct {
	// PodModifications describes modifications to apply to the pod template
	// +optional
	PodModifications *PodModifications `json:"podModifications,omitempty"`
}

// PodModifications defines pod-level modifications
type PodModifications struct {
	// AdditionalContainers to add to the pod (sidecars)
	// +optional
	AdditionalContainers []corev1.Container `json:"additionalContainers,omitempty"`

	// Volumes to add to the pod
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// InitContainers to add to the pod
	// +optional
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// PrimaryContainerModifications to apply to the primary container
	// +optional
	PrimaryContainerModifications *PrimaryContainerModifications `json:"primaryContainerModifications,omitempty"`
}

// PrimaryContainerModifications defines modifications for the primary container
type PrimaryContainerModifications struct {
	// VolumeMounts to add to the primary container
	// +optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// MergeEnv defines environment variables to be added to the main container
	// These will be merged with any existing env vars in the Workspace's container
	// +optional
	MergeEnv []AccessEnvTemplate `json:"mergeEnv,omitempty"`
}

// AccessStartupProbe defines how the controller verifies that access resources
// are serving traffic before marking the workspace as Available. Modeled after
// corev1.startupProbe — a one-shot gate that passes on the first successful
// response and is never checked again.
type AccessStartupProbe struct {
	// HTTPGet specifies an HTTP GET to perform against the access path.
	// +optional
	HTTPGet *AccessHTTPGetProbe `json:"httpGet,omitempty"`

	// Number of seconds after access resources are created before probes are initiated.
	// Default: 0.
	// +optional
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`

	// How often (in seconds) to perform the probe. Default: 2. Minimum: 1.
	// +optional
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`

	// Number of seconds after which the probe times out. Default: 5. Minimum: 1.
	// +optional
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`

	// Minimum consecutive failures before giving up and marking the workspace as Degraded.
	// Once degraded, the workspace must be stopped and restarted to retry the probe.
	// Default: 30. Minimum: 1.
	// +optional
	FailureThreshold int32 `json:"failureThreshold,omitempty"`
}

// AccessHTTPGetProbe defines the HTTP GET action for access startup probing.
type AccessHTTPGetProbe struct {
	// URLTemplate is a Go text/template resolving to the URL to probe.
	// Available variables: .Workspace, .AccessStrategy, .Service
	// (same as accessURLTemplate and accessResourceTemplates).
	URLTemplate string `json:"urlTemplate"`

	// AdditionalSuccessStatusCodes extends the default success range (200–399)
	// with extra HTTP status codes that indicate the route is live.
	// Example: [401] for bearer-token auth flows where the auth middleware
	// returns 401 on unauthenticated requests.
	// +optional
	AdditionalSuccessStatusCodes []int `json:"additionalSuccessStatusCodes,omitempty"`
}

// WorkspaceAccessStrategySpec defines the desired state of WorkspaceAccessStrategy
type WorkspaceAccessStrategySpec struct {
	// DisplayName is a human-readable name for this access strategy
	DisplayName string `json:"displayName"`

	// AccessResourceTemplates defines templates for resources created in the routes namespace
	AccessResourceTemplates []AccessResourceTemplate `json:"accessResourceTemplates"`

	// AccessURLTemplate is a template string for constructing the workspace access URL
	// Template variables include .Workspace and .AccessStrategy objects
	// If not provided, the AccessURL will not be set in the workspace status
	// Example: "https://example.com/workspace-path/"
	// +optional
	AccessURLTemplate string `json:"accessURLTemplate,omitempty"`

	// BearerAuthURLTemplate is a template string for constructing the bearer auth URL
	// Template variables include .Workspace and .AccessStrategy objects
	// Used by the extension API to generate initial authentication URLs
	// +optional
	BearerAuthURLTemplate string `json:"bearerAuthURLTemplate,omitempty"`

	// CreateConnectionHandler specifies the default handler for connection creation (e.g., "k8s-native").
	// Used as fallback when CreateConnectionHandlerMap does not contain the requested connection type.
	// +optional
	CreateConnectionHandler string `json:"createConnectionHandler,omitempty"`

	// CreateConnectionHandlerMap maps connection types to handler references in "plugin:action" format.
	// Example: {"vscode-remote": "aws:createSession"}
	// Falls back to CreateConnectionHandler if the requested connection type is not in this map.
	// +optional
	CreateConnectionHandlerMap map[string]string `json:"createConnectionHandlerMap,omitempty"`

	// PodEventsHandler specifies the handler for pod lifecycle events in "plugin:action" format.
	// Example: "aws:ssm-remote-access"
	// +optional
	PodEventsHandler string `json:"podEventsHandler,omitempty"`

	// CreateConnectionContext contains configuration for the connection handler
	// +optional
	CreateConnectionContext map[string]string `json:"createConnectionContext,omitempty"`

	// PodEventsContext contains configuration for the pod events handler
	// +optional
	PodEventsContext map[string]string `json:"podEventsContext,omitempty"`

	// DeploymentModifications defines modifications to apply to workspace deployments
	// +optional
	DeploymentModifications *DeploymentModifications `json:"deploymentModifications,omitempty"`

	// AccessStartupProbe defines how the controller verifies that access resources are
	// serving traffic. If not set, access resources are considered ready as soon as they
	// exist in the API server.
	// +optional
	AccessStartupProbe *AccessStartupProbe `json:"accessStartupProbe,omitempty"`
}

// WorkspaceAccessStrategyStatus defines the observed state of WorkspaceAccessStrategy
type WorkspaceAccessStrategyStatus struct {
	// Conditions represent the latest available observations of the resource's state
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// WorkspaceAccessStrategy is the Schema for the workspaceaccessstrategies API
type WorkspaceAccessStrategy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of WorkspaceAccessStrategy
	Spec WorkspaceAccessStrategySpec `json:"spec"`

	// Status defines the observed state of WorkspaceAccessStrategy
	// +optional
	Status WorkspaceAccessStrategyStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// WorkspaceAccessStrategyList contains a list of WorkspaceAccessStrategy
type WorkspaceAccessStrategyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkspaceAccessStrategy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkspaceAccessStrategy{}, &WorkspaceAccessStrategyList{})
}
