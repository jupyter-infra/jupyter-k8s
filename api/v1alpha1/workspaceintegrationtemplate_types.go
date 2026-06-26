/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceRef identifies a Kubernetes resource to fetch at resolution time and gives it a
// stable handle (ID) for use in template expressions: {{ resource "<id>" "<jsonpath>" }}.
type ResourceRef struct {
	// ID is the handle used to reference this resource in template expressions:
	// {{ resource "<id>" "<jsonpath>" }}. Must be unique within resourceRefs.
	// +kubebuilder:validation:Pattern=`^[a-z][a-zA-Z0-9-]*$`
	// +kubebuilder:validation:MaxLength=63
	ID string `json:"id"`

	// APIVersion of the target resource (e.g., "ray.io/v1")
	APIVersion string `json:"apiVersion"`

	// Kind of the target resource (e.g., "RayCluster")
	Kind string `json:"kind"`

	// Name supports template expressions: {{ .Workspace.Name }}, {{ .Parameters.X }}
	Name string `json:"name"`

	// Namespace supports template expressions; defaults to the workspace's namespace if omitted
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// WorkspaceIntegrationTemplateSpec defines the desired state of WorkspaceIntegrationTemplate
type WorkspaceIntegrationTemplateSpec struct {
	// DisplayName is a human-readable name for this integration template
	// +kubebuilder:validation:MaxLength=253
	DisplayName string `json:"displayName"`

	// ResourceRefs defines the resources to fetch at resolution time. Each entry is
	// addressed in template expressions by its id: {{ resource "<id>" "<jsonpath>" }}.
	// +optional
	// +listType=map
	// +listMapKey=id
	// +kubebuilder:validation:MaxItems=1
	ResourceRefs []ResourceRef `json:"resourceRefs,omitempty"`

	// ShareProcessNamespace enables a shared PID namespace across all containers in the
	// workspace pod, so the workspace container can see and signal processes running in
	// injected sidecars (e.g. the Ray sidecar). Mirrors corev1.PodSpec.ShareProcessNamespace.
	//
	// Admin-controlled (this field only exists on the integration template, which admins
	// install). Containers sharing a PID namespace can read each other's /proc (filesystem
	// and environment); injected containers should run as non-root.
	// +optional
	ShareProcessNamespace *bool `json:"shareProcessNamespace,omitempty"`

	// DeploymentModifications defines modifications to apply to workspace deployments
	// with template expressions in string fields
	// +optional
	DeploymentModifications *DeploymentModifications `json:"deploymentModifications,omitempty"`

	// StatusProbe is an optional, admin-authored probe the operator runs periodically
	// to verify the integration is reachable from the workspace pod's actual runtime
	// context. There is one probe per integration (not per resourceRef): it yields a
	// single verdict surfaced in workspace.status.integrations[].
	//
	// It is named statusProbe (not readinessProbe) because it is report-only: it writes
	// the verdict into status and never gates pod endpoint registration or restarts the
	// pod. Modeled on accessStrategy.spec.accessStartupProbe, but re-evaluated on the
	// operator poll cadence rather than as a one-shot gate.
	// +optional
	StatusProbe *IntegrationStatusProbe `json:"statusProbe,omitempty"`
}

// IntegrationStatusProbe defines how the operator checks an integration and records the
// verdict in workspace.status.integrations[]. It is report-only (not gating), which is why
// it is a statusProbe rather than a readinessProbe.
//
// The handler fields mirror the corev1.Probe handler union: exactly one transport is set.
// We deliberately do NOT embed corev1.Probe — it carries kubelet-only semantics
// (successThreshold, terminationGracePeriodSeconds) that don't apply to an operator-run,
// non-gating probe. This follows the AccessStartupProbe precedent, which hand-rolls its
// own wrapper around an optional handler for the same reason.
//
// Only Exec is implemented today (the operator execs the command in the workspace
// container, mirroring the idle detector). HTTPGet/TCPSocket/GRPC are reserved as additive
// optional siblings — adding one later is non-breaking.
type IntegrationStatusProbe struct {
	// Exec runs a command inside the workspace container; exit code 0 means ready. The
	// command always runs in the workspace container (the operator fixes this; it is not
	// author-selectable) so it sees the pod's real network and auth context, catching
	// data-plane failures a control-plane status check would miss (e.g. a Ray GCS reconnect
	// loop while .status still reads ready). This is the only transport implemented today.
	// +optional
	Exec *corev1.ExecAction `json:"exec,omitempty"`

	// How often (in seconds) the operator performs the probe. Default: operator poll cadence.
	// +optional
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`

	// Number of seconds after which a single probe attempt times out. Default: 5. Minimum: 1.
	// +optional
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=wit
// +kubebuilder:printcolumn:name="Display Name",type=string,JSONPath=`.spec.displayName`

// WorkspaceIntegrationTemplate is the Schema for the workspaceintegrationtemplates API.
// It defines a declarative, template-driven integration for adding runtime capabilities
// (sidecars, volumes, env vars) to workspace pods with dynamic resource lookup and
// template expression resolution. A Workspace may attach several of these via
// spec.integrationRefs.
type WorkspaceIntegrationTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of WorkspaceIntegrationTemplate
	Spec WorkspaceIntegrationTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// WorkspaceIntegrationTemplateList contains a list of WorkspaceIntegrationTemplate
type WorkspaceIntegrationTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkspaceIntegrationTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkspaceIntegrationTemplate{}, &WorkspaceIntegrationTemplateList{})
}
