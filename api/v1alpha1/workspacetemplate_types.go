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

// WorkspaceTemplateSpec defines the desired state of WorkspaceTemplate
type WorkspaceTemplateSpec struct {
	// DisplayName is the human-readable name of this template
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=100
	DisplayName string `json:"displayName"`

	// Description provides additional information about this template
	// +kubebuilder:validation:MaxLength=500
	// +optional
	Description string `json:"description,omitempty"`

	// DefaultImage is the default container image for workspaces using this template
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=500
	DefaultImage string `json:"defaultImage"`

	// AllowedImages is a list of container images that can be used with this template
	// If empty, only DefaultImage is allowed (secure by default)
	// If populated, workspace can override image with any from this list
	// +kubebuilder:validation:MaxItems=50
	// +optional
	AllowedImages []string `json:"allowedImages,omitempty"`

	// AllowCustomImages allows workspaces to use any container image, bypassing the AllowedImages restriction
	// When true, workspaces can specify any image regardless of the AllowedImages list
	// +kubebuilder:default=false
	// +optional
	AllowCustomImages *bool `json:"allowCustomImages,omitempty"`

	// DefaultResources specifies the default resource requirements
	// +optional
	DefaultResources *corev1.ResourceRequirements `json:"defaultResources,omitempty"`

	// ResourceBounds defines the min/max boundaries for resource overrides
	// +optional
	ResourceBounds *ResourceBounds `json:"resourceBounds,omitempty"`

	// PrimaryStorage defines storage configuration
	// +optional
	PrimaryStorage *StorageConfig `json:"primaryStorage,omitempty"`

	// EnvironmentVariables defines default environment variables
	// +optional
	EnvironmentVariables []corev1.EnvVar `json:"environmentVariables,omitempty"`

	// DefaultContainerConfig specifies default container command and args configuration
	// +optional
	DefaultContainerConfig *ContainerConfig `json:"defaultContainerConfig,omitempty"`

	// AllowSecondaryStorages controls whether workspaces using this template
	// can mount additional storage volumes beyond the primary storage
	// +kubebuilder:default=true
	// +optional
	AllowSecondaryStorages *bool `json:"allowSecondaryStorages,omitempty"`

	// DefaultNodeSelector specifies default node selection constraints
	// +optional
	DefaultNodeSelector map[string]string `json:"defaultNodeSelector,omitempty"`

	// DefaultAffinity specifies default node affinity and anti-affinity rules
	// +optional
	DefaultAffinity *corev1.Affinity `json:"defaultAffinity,omitempty"`

	// DefaultTolerations specifies default tolerations for scheduling on nodes with taints
	// +optional
	DefaultTolerations []corev1.Toleration `json:"defaultTolerations,omitempty"`

	// DefaultOwnershipType specifies default ownershipType for workspaces using this template
	// OwnershipType controls which users may edit/delete the workspace
	// +kubebuilder:validation:Enum=Public;OwnerOnly
	// +kubebuilder:default="Public"
	// +optional
	DefaultOwnershipType string `json:"defaultOwnershipType,omitempty"`

	// DefaultIdleShutdown provides default idle shutdown configuration
	// Includes timeout, detection endpoint, and enable/disable
	// +optional
	DefaultIdleShutdown *IdleShutdownSpec `json:"defaultIdleShutdown,omitempty"`

	// IdleShutdownOverrides controls override behavior and bounds
	// +optional
	IdleShutdownOverrides *IdleShutdownOverridePolicy `json:"idleShutdownOverrides,omitempty"`
	// DefaultAccessType specifies the default accessType for workspaces using this template
	// AccessType controls which users may create connections to the workspace.
	// +kubebuilder:validation:Enum=Public;OwnerOnly
	// +kubebuilder:default="Public"
	// +optional
	DefaultAccessType string `json:"defaultAccessType,omitempty"`

	// DefaultAccessStrategy specifies the default access strategy for workspaces using this template
	// +optional
	DefaultAccessStrategy *AccessStrategyRef `json:"defaultAccessStrategy,omitempty"`

	// DefaultLifecycle specifies default lifecycle hooks for workspaces using this template
	// +optional
	DefaultLifecycle *corev1.Lifecycle `json:"defaultLifecycle,omitempty"`

	// AppType specifies the application type for workspaces using this template
	// +optional
	AppType string `json:"appType,omitempty"`
}

// StorageConfig defines storage settings
// NOTE: CEL validation for minSize <= maxSize is not possible due to resource.Quantity type limitations
// Validation is enforced at runtime in the template resolver
type StorageConfig struct {
	// DefaultSize is the default storage size
	// +kubebuilder:default="10Gi"
	// +optional
	DefaultSize resource.Quantity `json:"defaultSize,omitempty"`

	// MinSize is the minimum allowed storage size
	// +optional
	MinSize *resource.Quantity `json:"minSize,omitempty"`

	// MaxSize is the maximum allowed storage size
	// +optional
	MaxSize *resource.Quantity `json:"maxSize,omitempty"`

	// DefaultStorageClassName is the default storage class name
	// +optional
	DefaultStorageClassName *string `json:"defaultStorageClassName,omitempty"`

	// DefaultMountPath is the default mount path for the storage
	// +kubebuilder:default="/home/jovyan"
	// +optional
	DefaultMountPath string `json:"defaultMountPath,omitempty"`
}

// IdleShutdownOverridePolicy defines idle shutdown override constraints
type IdleShutdownOverridePolicy struct {
	// Allow controls whether workspaces can override idle shutdown
	// +kubebuilder:default=true
	// +optional
	Allow *bool `json:"allow,omitempty"`

	// MinTimeoutMinutes is the minimum allowed timeout
	// +optional
	MinTimeoutMinutes *int `json:"minTimeoutMinutes,omitempty"`

	// MaxTimeoutMinutes is the maximum allowed timeout
	// +optional
	MaxTimeoutMinutes *int `json:"maxTimeoutMinutes,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Display Name",type="string",JSONPath=".spec.displayName"
// +kubebuilder:printcolumn:name="Default Image",type="string",JSONPath=".spec.defaultImage"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// WorkspaceTemplate is the Schema for the workspacetemplates API
// Templates define reusable, secure-by-default configurations for workspaces.
// The spec is immutable after creation - to update a template, create a new version.
type WorkspaceTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec WorkspaceTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// WorkspaceTemplateList contains a list of WorkspaceTemplate
type WorkspaceTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkspaceTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkspaceTemplate{}, &WorkspaceTemplateList{})
}
