/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
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
	// Name is a unique identifier for this volume within the pod (maps to pod.spec.volumes[].name)
	Name string `json:"name"`

	// PersistentVolumeClaimName is the name of the existing PVC to mount
	PersistentVolumeClaimName string `json:"persistentVolumeClaimName"`

	// MountPath is the path where the volume should be mounted (Unix-style path, e.g. /data)
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
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="storage class name is immutable"
	StorageClassName *string `json:"storageClassName,omitempty"`

	// Size specifies the size of the persistent volume
	// Supports standard Kubernetes resource quantities (e.g., "10Gi", "500Mi", "1Ti")
	// Integer values without units are interpreted as bytes
	Size resource.Quantity `json:"size,omitempty"`

	// MountPath specifies where to mount the persistent volume in the container
	// Default is /home/jovyan (jovyan is the standard user in Jupyter images)
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

// TemplateRef defines a reference to a WorkspaceTemplate
type TemplateRef struct {
	// Name of the WorkspaceTemplate
	Name string `json:"name"`

	// Namespace where the WorkspaceTemplate is located
	// When omitted, defaults to the workspace's namespace
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// IdleShutdownSpec defines idle shutdown configuration
type IdleShutdownSpec struct {
	// Enabled indicates if idle shutdown is enabled
	Enabled bool `json:"enabled"`

	// IdleTimeoutInMinutes specifies idle timeout in minutes
	// +kubebuilder:validation:Minimum=1
	IdleTimeoutInMinutes int `json:"idleTimeoutInMinutes"`

	// Detection specifies how to detect idle state
	Detection IdleDetectionSpec `json:"detection"`
}

// IdleDetectionSpec defines idle detection methods
type IdleDetectionSpec struct {
	// HTTPGet specifies the HTTP request to perform for idle detection
	// +optional
	HTTPGet *IdleHTTPGetAction `json:"httpGet,omitempty"`
}

// IdleHTTPGetAction extends corev1.HTTPGetAction with transport and response parsing options.
type IdleHTTPGetAction struct {
	corev1.HTTPGetAction `json:",inline"`

	// Transport selects how the operator reaches the endpoint.
	// "podExec" executes curl inside the workspace container (legacy).
	// "network" makes a direct HTTP call from the operator to the workspace Service's ClusterIP.
	// +kubebuilder:validation:Enum=podExec;network
	// +kubebuilder:default=podExec
	// +optional
	Transport string `json:"transport,omitempty"`

	// LastActivityTimestamp describes how to extract and parse the last-activity
	// timestamp from the JSON response body.
	// +optional
	LastActivityTimestamp *IdleLastActivityTimestampSpec `json:"lastActivityTimestamp,omitempty"`
}

// IdleLastActivityTimestampSpec configures extraction and parsing of a last-activity
// timestamp value from an idle-detection HTTP response body.
type IdleLastActivityTimestampSpec struct {
	// ResponseBodyPath is a dot-separated path to the timestamp value in the
	// JSON response body (e.g. "last_activity" or "status.lastActive").
	// Default: "lastActiveTimestamp"
	// +optional
	ResponseBodyPath string `json:"responseBodyPath,omitempty"`

	// Format specifies how to parse the extracted value.
	// "RFC3339" expects an RFC 3339 timestamp string.
	// "unix" expects epoch seconds (numeric or string).
	// +kubebuilder:validation:Enum=RFC3339;unix
	// +kubebuilder:default=RFC3339
	// +optional
	Format string `json:"format,omitempty"`
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

	// OwnershipType specifies who can modify the workspace.
	// Public means anyone with RBAC permissions can update/delete the workspace.
	// OwnerOnly means only the creator can update/delete the workspace.
	// +kubebuilder:validation:Enum=Public;OwnerOnly
	// +optional
	OwnershipType string `json:"ownershipType,omitempty"`

	// AccessType specifies who can connect to the workspace.
	// Public means anyone with RBAC permissions can connect to workspace.
	// OwnerOnly means only the creator can connect to the workspace.
	// +kubebuilder:validation:Enum=Public;OwnerOnly
	// +optional
	AccessType string `json:"accessType,omitempty"`

	// Resources specifies the resource requirements
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Storage specifies the storage configuration
	Storage *StorageSpec `json:"storage,omitempty"`

	// Volumes specifies additional volumes to mount from existing PersistantVolumeClaims
	// +kubebuilder:validation:XValidation:rule="!self.exists(v, v.name == 'workspace-storage')",message="volume name 'workspace-storage' is reserved"
	Volumes []VolumeSpec `json:"volumes,omitempty"`

	// ContainerConfig specifies container command and args configuration
	ContainerConfig *ContainerConfig `json:"containerConfig,omitempty"`

	// Env specifies environment variables for the workspace container
	// When a template is used, template's BaseEnv vars are merged (workspace vars take precedence by name)
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// NodeSelector specifies node selection constraints for the workspace pod
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Affinity specifies node affinity and anti-affinity rules for the workspace pod
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations specifies tolerations for the workspace pod to schedule on nodes with matching taints
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Lifecycle specifies actions that the management system should take
	// in response to container lifecycle events (for instance, lifecycle hooks)
	Lifecycle *corev1.Lifecycle `json:"lifecycle,omitempty"`

	// ReadinessProbe specifies the readiness probe for the main workspace container.
	// +optional
	ReadinessProbe *corev1.Probe `json:"readinessProbe,omitempty"`

	// AccessStrategy specifies the WorkspaceAccessStrategy to use
	// +optional
	AccessStrategy *AccessStrategyRef `json:"accessStrategy,omitempty"`

	// TemplateRef references a WorkspaceTemplate to use as base configuration
	// When set, template provides defaults and workspace spec fields act as overrides
	// +optional
	TemplateRef *TemplateRef `json:"templateRef,omitempty"`

	// IdleShutdown specifies idle shutdown configuration
	// +optional
	IdleShutdown *IdleShutdownSpec `json:"idleShutdown,omitempty"`

	// AppType specifies the application type for this workspace
	// +optional
	AppType string `json:"appType,omitempty"`

	// ServiceAccountName specifies the name of the ServiceAccount to use for the workspace pod
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`

	// PodSecurityContext specifies pod-level security context
	// Overrides template defaults when specified
	// +optional
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`

	// ContainerSecurityContext specifies container-level security context for the main workspace container
	// Takes precedence over PodSecurityContext for the main container
	// Overrides template defaults when specified
	// +optional
	ContainerSecurityContext *corev1.SecurityContext `json:"containerSecurityContext,omitempty"`

	// InitContainers specifies init containers to run before the workspace container starts
	// When a template is used, template's DefaultInitContainers are applied if workspace has none
	// Requires AllowCustomInitContainers=true on the template to specify custom init containers
	// +kubebuilder:validation:MaxItems=10
	// +optional
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// IntegrationTemplateRefs attaches one or more WorkspaceIntegrationTemplates that inject runtime
	// capabilities (sidecars, volumes, env vars) into the workspace pod with template-based
	// dynamic resolution. Each entry is the user's REQUEST only -- a reference to a
	// WorkspaceIntegrationTemplate plus parameters -- never the resolved sidecars.
	//
	// The workspace controller resolves each integration against its referenced resources only when
	// the input token -- hash(templateRef + parameters) -- changes. On an
	// unchanged token (external drift in a referenced resource, an idle reconcile), the controller
	// rebuilds the pod template from the frozen values recorded in status.resolvedIntegrations
	// instead of re-reading the referenced resource, so drift never rolls the running pod. No
	// WorkspaceIntegration child object is created.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=1
	IntegrationTemplateRefs []IntegrationTemplateRef `json:"integrationTemplateRefs,omitempty"`
}

// IntegrationParameter is a single user-provided input for template expression resolution.
type IntegrationParameter struct {
	// Name of the parameter, referenced in template expressions as {{ .Parameters.<Name> }}
	Name string `json:"name"`

	// Value of the parameter (substituted into template expression fields at resolution time)
	// +optional
	Value string `json:"value,omitempty"`
}

// IntegrationTemplateRef defines a reference to a WorkspaceIntegrationTemplate.
type IntegrationTemplateRef struct {
	// Name of the WorkspaceIntegrationTemplate
	Name string `json:"name"`

	// Namespace where the WorkspaceIntegrationTemplate is located
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Parameters provided by the user for template expression resolution.
	// Each parameter is referenced in template expressions as {{ .Parameters.<Name> }}.
	// Names must be unique.
	// +optional
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MaxItems=10
	Parameters []IntegrationParameter `json:"parameters,omitempty"`
}

// ParametersMap flattens the extensible name/value parameter list into a map keyed
// by name for {{ .Parameters.<name> }} expression access.
func (r *IntegrationTemplateRef) ParametersMap() map[string]string {
	m := make(map[string]string, len(r.Parameters))
	for _, p := range r.Parameters {
		m[p.Name] = p.Value
	}
	return m
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

	// ApplicationBasePath is the resolved routing prefix for the workspace application.
	// Set during access-resources reconciliation; used by idle detection to construct
	// the full endpoint path.
	// +optional
	ApplicationBasePath string `json:"applicationBasePath,omitempty"`

	// AccessResourceSelector is a label selector that can be used to find all resources
	// created from the workspace's AccessStrategy templates
	// +optional
	AccessResourceSelector string `json:"accessResourceSelector,omitempty"`

	// AccessResources provides status details of individual resources created from
	// the workspace's AccessStrategy templates
	// +optional
	AccessResources []AccessResourceStatus `json:"accessResources,omitempty"`

	// ObservedAccessStrategyVersion is a token capturing the identity and
	// version of the AccessStrategy last evaluated during workspace
	// reconciliation. The controller resets probe state when this value changes.
	// +optional
	ObservedAccessStrategyVersion string `json:"observedAccessStrategyVersion,omitempty"`

	// AccessStartupProbeSucceeded indicates whether the access startup probe
	// has passed. Set to true when the probe succeeds; reset to false when
	// the workspace stops.
	// +optional
	AccessStartupProbeSucceeded bool `json:"accessStartupProbeSucceeded,omitempty"`

	// AccessStartupProbeFailures tracks the number of consecutive failed access
	// startup probe attempts. Set by the controller during the probing phase;
	// cleared (nil) on success or when the workspace stops.
	// +optional
	AccessStartupProbeFailures *int32 `json:"accessStartupProbeFailures,omitempty"`

	// EarliestNextProbeTime is the earliest wall-clock time at which the next
	// access startup probe may fire. Set by the controller after each probe
	// attempt to enforce spacing; survives watch-triggered re-reconciliations.
	// +optional
	EarliestNextProbeTime *metav1.Time `json:"earliestNextProbeTime,omitempty"`

	// Conditions represent the current state of the Workspace resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional and ready to use
	// - "Progressing": the resource is being created, updated, or stopped
	// - "Degraded": the resource failed to reach or maintain its desired state
	// - "Stopped": the workspace has been stopped and resources scaled down
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +patchStrategy=merge
	// +patchMergeKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// IntegrationStatuses reports the operator-observed readiness of each integration applied to the
	// workspace, as observed by the periodic status probe. One entry per integration that defines
	// a statusProbe. Absence of an entry means "not yet probed"; a present entry always reflects
	// an actual observation (Ready + Reason). Named after the k8s pod.status.containerStatuses
	// convention (<thing>Statuses []<Thing>Status), matching the IntegrationStatus element type.
	// +optional
	// +listType=map
	// +listMapKey=name
	IntegrationStatuses []IntegrationStatus `json:"integrationStatuses,omitempty"`

	// ResolvedIntegrations records the frozen output of the last successful integration resolution,
	// one entry per attached integration. It is the operator's private freeze store: on an unchanged
	// input token the controller rebuilds the pod template from these values WITHOUT re-reading the
	// referenced resource, so external drift never re-resolves and never rolls the running pod. Each
	// entry stores only the resolved template substitutions (a small string map), never the fully
	// rendered pod spec -- the pod SHAPE still comes from the live WorkspaceIntegrationTemplate; only
	// the resolved VALUES are frozen here. Written by the controller via the status subresource; it
	// is never user-authored.
	// +optional
	// +listType=map
	// +listMapKey=name
	ResolvedIntegrations []ResolvedIntegration `json:"resolvedIntegrations,omitempty"`
}

// ResolvedIntegration is the frozen resolution record for a single attached integration. It is the
// source of truth the controller replays from on unchanged-token reconciles.
type ResolvedIntegration struct {
	// Name is the WorkspaceIntegrationTemplate name this record resolves (matches the
	// spec.integrationTemplateRefs[].name that produced it).
	Name string `json:"name"`

	// ParametersHash is a hash of the integration ref's identity and user-supplied parameters
	// (templateRef namespace+name + the sorted parameter map) captured when these values were
	// resolved. A change means the user switched clusters or edited a parameter.
	ParametersHash string `json:"parametersHash"`

	// ObservedIntegrationTemplateVersion is "<template.UID>.<template.Generation>" captured when these
	// values were resolved. A change means the admin edited (or replaced) the referenced
	// WorkspaceIntegrationTemplate. Mirrors status.observedAccessStrategyVersion.
	ObservedIntegrationTemplateVersion string `json:"observedIntegrationTemplateVersion"`

	// The controller replays the frozen Values (below) as long as BOTH ParametersHash and
	// ObservedIntegrationTemplateVersion still match the live inputs; if either differs it re-resolves
	// against the referenced resource and refreezes. Re-resolution re-renders the pod template, so it
	// rolls the pod only when the rendered output actually changes.

	// Values is the frozen resolution map: each key is a "<resourceRefID>|<jsonPath>" capture key
	// and each value is the literal string the {{ resource ... }} expression resolved to at capture
	// time. On replay, the resolver serves these instead of reading the referenced resource. Storing
	// only these substitutions (not the rendered pod spec) keeps the status payload small.
	// +optional
	Values map[string]string `json:"values,omitempty"`
}

// IntegrationStatus reports the operator-observed readiness of a single integration. It follows the
// KRO instance-status precedent (https://kro.run/docs/concepts/instances#understanding-status):
// a name, a coarse state, and standard Kubernetes conditions carrying the machine-readable detail.
type IntegrationStatus struct {
	// Name is the name of the WorkspaceIntegrationTemplate this status reports on.
	Name string `json:"name"`

	// State is a coarse, human-facing rollup of the conditions below: "Ready" when the integration's
	// last probe succeeded, "Degraded" when it failed. Empty means "not yet probed".
	// +optional
	State string `json:"state,omitempty"`

	// Conditions carry the detailed, machine-readable observation. The operator sets a single
	// "Ready" condition whose Reason is one of Ready/ProbeFailed/PodNotFound/ProbeError and whose
	// Message holds any human-readable detail (e.g. the probe's stderr on failure).
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Available",type="string",JSONPath=".status.conditions[?(@.type==\"Available\")].status"
// +kubebuilder:printcolumn:name="Progressing",type="string",JSONPath=".status.conditions[?(@.type==\"Progressing\")].status"
// +kubebuilder:printcolumn:name="Degraded",type="string",JSONPath=".status.conditions[?(@.type==\"Degraded\")].status"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="CreatedBy",type="string",JSONPath=`.metadata.annotations['workspace\.jupyter\.org/created-by']`,priority=1
// +kubebuilder:printcolumn:name="AccessType",type="string",JSONPath=".spec.accessType",priority=1

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
