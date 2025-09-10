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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// JupyterServerSpec defines the desired state of JupyterServer
type JupyterServerSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// Display Name of the server
	DisplayName string `json:"displayName"`

	// Image specifies the container image to use
	Image string `json:"image,omitempty"`

	// DesiredStatus specifies the desired operational status
	// +kubebuilder:validation:Enum=Running;Stopped
	DesiredStatus string `json:"desiredStatus,omitempty"`

	// ServiceAccountName specifies the ServiceAccount used by the pod
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// Resources specifies the resource requirements
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

// JupyterServerStatus defines the observed state of JupyterServer.
type JupyterServerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// DeploymentName is the name of the deployment managing the JupyterServer pods
	// +optional
	DeploymentName string `json:"deploymentName,omitempty"`

	// ServiceName is the name of the service exposing the JupyterServer
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// Conditions represent the current state of the JupyterServer resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional and ready to use
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Available",type="string",JSONPath=".status.conditions[?(@.type==\"Available\")].status"
// +kubebuilder:printcolumn:name="Progressing",type="string",JSONPath=".status.conditions[?(@.type==\"Progressing\")].status"
// +kubebuilder:printcolumn:name="Degraded",type="string",JSONPath=".status.conditions[?(@.type==\"Degraded\")].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// JupyterServer is the Schema for the jupyterservers API
type JupyterServer struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of JupyterServer
	// +required
	Spec JupyterServerSpec `json:"spec"`

	// status defines the observed state of JupyterServer
	// +optional
	Status JupyterServerStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// JupyterServerList contains a list of JupyterServer
type JupyterServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []JupyterServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&JupyterServer{}, &JupyterServerList{})
}
