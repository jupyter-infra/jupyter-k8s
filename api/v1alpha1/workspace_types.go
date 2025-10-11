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

// VolumeSpec defines a volume to mount from an existing PVC
type VolumeSpec struct {
	// Name is the name of the volume
	Name string `json:"name"`

	// PersistentVolumeClaimName is the name of the existing PVC to mount
	PersistentVolumeClaimName string `json:"persistentVolumeClaimName"`

	// MountPath is the path where the volume should be mounted
	MountPath string `json:"mountPath"`
}

// ContainerConfig defines container command and args configuration
type ContainerConfig struct {
	// Command specifies the container command
	Command []string `json:"command,omitempty"`

	// Args specifies the container arguments
	Args []string `json:"args,omitempty"`
}

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

// AccessStrategyRef defines a reference to a WorkspaceAccessStrategy
type AccessStrategyRef struct {
	// Name of the WorkspaceAccessStrategy
	Name string `json:"name"`

	// Namespace where the WorkspaceAccessStrategy is located
	// +optional
	Namespace string `json:"namespace,omitempty"`
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

	// Resources specifies the resource requirements
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage specifies the storage configuration
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="storage is immutable"
	Storage *StorageSpec `json:"storage,omitempty"`

	// Volumes specifies additional volumes to mount from existing PVCs
	Volumes []VolumeSpec `json:"volumes,omitempty"`

	// ContainerConfig specifies container command and args configuration
	ContainerConfig *ContainerConfig `json:"containerConfig,omitempty"`

	// NodeSelector specifies node selection constraints for the workspace pod
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// AccessStrategy specifies the WorkspaceAccessStrategy to use
	// +optional
	AccessStrategy *AccessStrategyRef `json:"accessStrategy,omitempty"`

	// TemplateRef references a WorkspaceTemplate to use as base configuration
	// When set, template provides defaults and spec fields (Image, Resources, Storage.Size) act as overrides
	// IMMUTABLE: Cannot be changed after workspace creation
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="templateRef is immutable"
	// +optional
	TemplateRef *string `json:"templateRef,omitempty"`
}

// AccessResourceStatus defines the status of a resource created from a template
type AccessResourceStatus struct {
	// Kind of the Kubernetes resource
	Kind string `json:"kind"`

	// APIVersion of the Kubernetes resource
	APIVersion string `json:"apiVersion"`

	// Name of the resource
	Name string `json:"name"`

	// Namespace of the resource
	Namespace string `json:"namespace"`
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

	// AccessURL is the URL at which the workspace can be accessed
	// +optional
	AccessURL string `json:"accessURL,omitempty"`

	// AccessResourceSelector is a label selector that can be used to find all resources
	// created from the workspace's AccessStrategy templates
	// +optional
	AccessResourceSelector string `json:"accessResourceSelector,omitempty"`

	// AccessResources provides status details of individual resources created from
	// the workspace's AccessStrategy templates
	// +optional
	AccessResources []AccessResourceStatus `json:"accessResources,omitempty"`

	// Conditions represent the current state of the Workspace resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional and ready to use
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	// - "Valid": the workspace configuration passes all validation checks (template, quota, etc.)
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
