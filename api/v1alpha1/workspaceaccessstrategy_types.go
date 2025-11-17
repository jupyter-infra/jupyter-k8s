/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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

	// ControllerConfig contains settings used by the controller for strategy-specific operations
	// +optional
	ControllerConfig map[string]string `json:"controllerConfig,omitempty"`

	// DeploymentModifications defines modifications to apply to workspace deployments
	// +optional
	DeploymentModifications *DeploymentModifications `json:"deploymentModifications,omitempty"`
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
