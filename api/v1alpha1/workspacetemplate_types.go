/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
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

	// DefaultPodSecurityContext specifies default pod-level security context
	// +optional
	DefaultPodSecurityContext *corev1.PodSecurityContext `json:"defaultPodSecurityContext,omitempty"`

	// AppType specifies the application type for workspaces using this template
	// +optional
	AppType string `json:"appType,omitempty"`
}

// ResourceBounds defines minimum and maximum resource limits for any resource type.
// Uses Kubernetes ResourceName as keys to support vendor-agnostic resource specifications.
type ResourceBounds struct {
	// Resources defines min/max bounds for any resource type.
	// Map keys use Kubernetes resource names following these conventions:
	//
	// Standard resources (no vendor prefix):
	//   - cpu: CPU cores (e.g., "100m", "2")
	//   - memory: RAM (e.g., "128Mi", "4Gi")
	//
	// Extended resources (vendor-prefixed):
	//   - nvidia.com/gpu: NVIDIA GPUs
	//   - amd.com/gpu: AMD GPUs
	//   - intel.com/gpu: Intel GPUs
	//   - nvidia.com/mig-1g.5gb: NVIDIA MIG profile (1 GPU instance, 5GB)
	//   - nvidia.com/mig-2g.10gb: NVIDIA MIG profile (2 GPU instances, 10GB)
	//
	// Custom accelerators follow the pattern: vendor.example/resource-name
	// +optional
	Resources map[corev1.ResourceName]ResourceRange `json:"resources,omitempty"`
}

// ResourceRange defines min and max for a resource
// NOTE: CEL validation for min <= max is not possible due to resource.Quantity type limitations
// Validation is enforced at runtime in the template resolver
type ResourceRange struct {
	// Min is the minimum allowed value
	// +kubebuilder:validation:Required
	Min resource.Quantity `json:"min"`

	// Max is the maximum allowed value
	// +kubebuilder:validation:Required
	Max resource.Quantity `json:"max"`
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

	// MinIdleTimeoutInMinutes is the minimum allowed timeout
	// +optional
	MinIdleTimeoutInMinutes *int `json:"minIdleTimeoutInMinutes,omitempty"`

	// MaxIdleTimeoutInMinutes is the maximum allowed timeout
	// +optional
	MaxIdleTimeoutInMinutes *int `json:"maxIdleTimeoutInMinutes,omitempty"`
}

// WorkspaceTemplateStatus defines the observed state of WorkspaceTemplate
// Follows Kubernetes API conventions for status reporting
type WorkspaceTemplateStatus struct {
	// ObservedGeneration reflects the generation of the most recently observed WorkspaceTemplate spec.
	// This field is used by controllers to determine if they need to reconcile the template.
	// When metadata.generation != status.observedGeneration, the controller has not yet processed the latest spec.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Display Name",type="string",JSONPath=".spec.displayName"
// +kubebuilder:printcolumn:name="Default Image",type="string",JSONPath=".spec.defaultImage"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// WorkspaceTemplate is the Schema for the workspacetemplates API
// Templates define reusable, secure-by-default configurations for workspaces.
// Template spec can be updated; existing workspaces keep their configuration (lazy application).
type WorkspaceTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WorkspaceTemplateSpec   `json:"spec,omitempty"`
	Status WorkspaceTemplateStatus `json:"status,omitempty"`
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
