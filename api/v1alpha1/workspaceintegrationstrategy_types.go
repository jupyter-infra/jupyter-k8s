/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceLookup specifies how to find a Kubernetes resource at reconcile time
type ResourceLookup struct {
	// APIVersion of the target resource (e.g., "ray.io/v1")
	APIVersion string `json:"apiVersion"`

	// Kind of the target resource (e.g., "RayCluster")
	Kind string `json:"kind"`

	// Name supports template expressions: {{ .Workspace.Name }}, {{ .Parameters.X }}
	Name string `json:"name"`

	// Namespace supports strategy expressions; defaults to the workspace's namespace if omitted
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// WorkspaceIntegrationStrategySpec defines the desired state of WorkspaceIntegrationStrategy
type WorkspaceIntegrationStrategySpec struct {
	// DisplayName is a human-readable name for this integration strategy
	// +kubebuilder:validation:MaxLength=253
	DisplayName string `json:"displayName"`

	// ResourceLookup defines the resource to fetch at reconcile time
	// +optional
	ResourceLookup *ResourceLookup `json:"resourceLookup,omitempty"`

	// DeploymentModifications defines modifications to apply to workspace deployments
	// with template expressions in string fields
	// +optional
	DeploymentModifications *DeploymentModifications `json:"deploymentModifications,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=wis
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`

// WorkspaceIntegrationStrategy is the Schema for the workspaceintegrationstrategies API.
// It defines a declarative, template-driven strategy for adding runtime capabilities
// (sidecars, volumes, env vars) to workspace pods with dynamic resource lookup and
// template expression resolution.
type WorkspaceIntegrationStrategy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of WorkspaceIntegrationStrategy
	Spec WorkspaceIntegrationStrategySpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// WorkspaceIntegrationStrategyList contains a list of WorkspaceIntegrationStrategy
type WorkspaceIntegrationStrategyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkspaceIntegrationStrategy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkspaceIntegrationStrategy{}, &WorkspaceIntegrationStrategyList{})
}
