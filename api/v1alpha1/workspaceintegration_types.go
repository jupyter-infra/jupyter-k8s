/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkspaceRef identifies the Workspace that owns a WorkspaceIntegration. It supplies the
// identity (name/namespace) used as template-resolution context when the WorkspaceIntegration
// is baked at admission, and mirrors the ownerReference the controller sets for garbage
// collection.
type WorkspaceRef struct {
	// Name of the owning Workspace.
	Name string `json:"name"`

	// Namespace of the owning Workspace. A WorkspaceIntegration is ALWAYS co-located with its
	// Workspace (the controller creates the child in the workspace's own namespace and ownerRefs
	// are namespace-local), so cross-namespace references are NOT supported. Leave this unset:
	// it defaults to the WorkspaceIntegration's own namespace. If set, it must equal the
	// object's own namespace; a differing value is rejected at admission (enforced by the
	// mutating baker, which resolves against the co-located workspace only).
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// WorkspaceIntegrationSpec defines the desired state of WorkspaceIntegration.
//
// A WorkspaceIntegration is the per-integration INSTANCE of a WorkspaceIntegrationTemplate,
// analogous to how a Workspace is the instance of a WorkspaceTemplate. This mirrors the
// Workspace/WorkspaceTemplate precedent exactly: the instance reuses the template's own field
// types (DeploymentModifications, ShareProcessNamespace, StatusProbe) verbatim, with every
// template expression already substituted to a literal value (no {{ }} remains). There is no
// bespoke "resolved" envelope -- just as Workspace.spec.image carries the resolved value of
// WorkspaceTemplate.spec.defaultImage, these fields carry the resolved values of the template's
// corresponding fields.
//
// TemplateRef + WorkspaceRef are the INPUTS, written by the workspace controller when it creates
// the shell. The resolvable fields are the OUTPUTS, written exclusively by the WorkspaceIntegration
// mutating webhook at this object's admission (CREATE/UPDATE), which resolves TemplateRef against
// the live referenced resources and freezes the literal result. Data scientists have no
// create/update permission on WorkspaceIntegration, so the outputs cannot be user-tampered; the
// webhook recomputes them on every admission, so any submitted value is overwritten before persist.
type WorkspaceIntegrationSpec struct {
	// TemplateRef references the WorkspaceIntegrationTemplate this integration instantiates.
	// The template namespace defaults to this object's namespace when unset. INPUT.
	TemplateRef IntegrationTemplateRef `json:"templateRef"`

	// WorkspaceRef identifies the owning Workspace, supplying the identity used as
	// template-resolution context when resolving the output fields. INPUT.
	WorkspaceRef WorkspaceRef `json:"workspaceRef"`

	// DeploymentModifications is the frozen, fully-resolved pod-template modification set
	// (sidecars, init containers, volumes, primary-container env and volume mounts) with every
	// template expression substituted to a literal value. Same type the WorkspaceIntegrationTemplate
	// uses, so both AccessStrategy and Integration are producers of the same PodModifications shape.
	// The workspace controller appends this to the workspace pod's Deployment. Webhook-owned OUTPUT.
	// +optional
	DeploymentModifications *DeploymentModifications `json:"deploymentModifications,omitempty"`

	// ShareProcessNamespace, when true, enables a shared PID namespace on the workspace pod.
	// Carried verbatim from the template (no resolution needed). Webhook-owned OUTPUT.
	// +optional
	ShareProcessNamespace *bool `json:"shareProcessNamespace,omitempty"`

	// StatusProbe is the integration's status probe, frozen from the template at admission. The
	// operator runs it periodically against the workspace pod and writes the verdict to
	// workspace.status.integrations[]. Freezing it here means the workspace controller never
	// re-reads the WorkspaceIntegrationTemplate at reconcile time, even for liveness.
	// Webhook-owned OUTPUT.
	// +optional
	StatusProbe *IntegrationStatusProbe `json:"statusProbe,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced,shortName=wi
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.spec.templateRef.name`
// +kubebuilder:printcolumn:name="Workspace",type=string,JSONPath=`.spec.workspaceRef.name`
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// WorkspaceIntegration is the Schema for the workspaceintegrations API. It is the frozen,
// per-integration instance of a WorkspaceIntegrationTemplate, created by the workspace
// controller (one per workspace.spec.integrationRefs entry) and baked by a mutating webhook at
// its own admission. It is a system-managed object: data scientists do not author it directly.
type WorkspaceIntegration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of WorkspaceIntegration. The WorkspaceIntegration has no
	// status subresource: it is resolved by the mutating webhook at its own admission and has no
	// reconcile controller, so there is no observed state to report. Per-integration health is
	// surfaced on the parent Workspace's status.integrations[] (see IntegrationStatus), and the
	// child is garbage-collected via its ownerReference. This mirrors the controller-less
	// WorkspaceIntegrationTemplate, which likewise carries no status.
	Spec WorkspaceIntegrationSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// WorkspaceIntegrationList contains a list of WorkspaceIntegration
type WorkspaceIntegrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []WorkspaceIntegration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&WorkspaceIntegration{}, &WorkspaceIntegrationList{})
}
