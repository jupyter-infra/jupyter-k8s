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
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// StorageSpec defines the storage configuration for Workspace
type StorageSpec struct {
	// StorageClassName specifies the storage class to use for persistent storage
	// +optional
	StorageClassName *string `json:"storageClassName,omitempty"`

	// Size specifies the size of the persistent volume
	// Supports standard Kubernetes resource quantities (e.g., "10Gi", "500Mi", "1Ti")
	// Integer values without units are interpreted as bytes
	// +kubebuilder:default="10Gi"
	Size resource.Quantity `json:"size,omitempty"`

	// MountPath specifies where to mount the persistent volume in the container
	// Default is /home/jovyan (jovyan is the standard user in Jupyter images)
	// +kubebuilder:default="/home/jovyan"
	MountPath string `json:"mountPath,omitempty"`
}

// WorkspaceSpec defines the desired state of Workspace
type WorkspaceSpec struct {
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

	// Storage specifies the storage configuration
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="storage is immutable"
	Storage *StorageSpec `json:"storage,omitempty"`

	// TemplateRef references a WorkspaceTemplate to use as base configuration
	// Note: Changing templateRef after creation is not recommended as it may lead to
	// configuration inconsistencies. Future versions may enforce immutability via webhook.
	// +optional
	TemplateRef *string `json:"templateRef,omitempty"`

	// TemplateOverrides allows overriding specific template values
	// +optional
	TemplateOverrides *TemplateOverrides `json:"templateOverrides,omitempty"`
}

// TemplateOverrides allows selective overrides of template settings
type TemplateOverrides struct {
	// Image overrides the template's default image
	// +optional
	Image *string `json:"image,omitempty"`

	// Resources overrides the template's default resources
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// StorageSize overrides the template's default storage size
	// +optional
	StorageSize *string `json:"storageSize,omitempty"`

	// IdleShutdownConfig overrides the template's idle shutdown configuration
	// +optional
	IdleShutdownConfig *IdleShutdownConfig `json:"idleShutdownConfig,omitempty"`
}

// WorkspaceStatus defines the observed state of Workspace.
type WorkspaceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// DeploymentName is the name of the deployment managing the Workspace pods
	// +optional
	DeploymentName string `json:"deploymentName,omitempty"`

	// ServiceName is the name of the service exposing the Workspace
	// +optional
	ServiceName string `json:"serviceName,omitempty"`

	// Conditions represent the current state of the Workspace resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional and ready to use
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	// - "TemplateValid": the template reference and overrides are valid (if using template)
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

// Workspace is the Schema for the workspaces API
type Workspace struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec defines the desired state of Workspace
	// +required
	Spec WorkspaceSpec `json:"spec"`

	// status defines the observed state of Workspace
	// +optional
	Status WorkspaceStatus `json:"status,omitempty,omitzero"`
}

// +kubebuilder:object:root=true

// WorkspaceList contains a list of Workspace
type WorkspaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Workspace `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Workspace{}, &WorkspaceList{})
}
