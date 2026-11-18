/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PosixIdentitySpec is the immutable record of one allocated uid. The uid is the object's name,
// uid-<uid>, so the API server's name uniqueness makes two records of one uid impossible.
type PosixIdentitySpec struct {
	// Username is the Kubernetes username the uid belongs to, as in the workspace's created-by annotation.
	// The user's primary gid equals the uid (user private group).
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=1024
	Username string `json:"username"`

	// UsernameHash is the lookup key of Username: the first 32 hex characters of its SHA-256 digest,
	// exposed to templates as {{ .User.UsernameHash }}.
	// +kubebuilder:validation:Pattern=`^[0-9a-f]{32}$`
	UsernameHash string `json:"usernameHash"`

	// UnixName is the name of the user's passwd entry: what whoami prints for the uid inside the
	// workspace, and what ls -l shows as the owner of the user's files. It exists because a Kubernetes
	// username such as "github:JaneDoe" cannot be a passwd name (':' separates passwd fields).
	//
	// The operator never writes passwd itself: templates read the name as {{ .User.UnixName }}, e.g. an
	// access strategy passing it to the image in an env var, and the image writes the passwd entry for
	// its uid (for instance with nss_wrapper).
	//
	// The operator derives it from Username at allocation ("github:JaneDoe" becomes "github-janedoe");
	// an admin pre-seeding an object may set the name the user already has elsewhere, e.g. in a
	// directory. It is stored, so a change to the derivation rule never renames existing users.
	// +kubebuilder:validation:Pattern=`^[a-z0-9_-]{1,32}$`
	UnixName string `json:"unixName"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Username",type="string",JSONPath=".spec.username"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// PosixIdentity records the uid allocated to one user. It is an immutable ledger entry written by the
// operator: one object per uid, named uid-<uid>, never garbage-collected since files on shared storage
// keep carrying the uid after the user leaves.
type PosixIdentity struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty,omitzero"`

	// spec is immutable once created
	// +required
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="spec is immutable"
	Spec PosixIdentitySpec `json:"spec"`
}

// +kubebuilder:object:root=true

// PosixIdentityList contains a list of PosixIdentity
type PosixIdentityList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PosixIdentity `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PosixIdentity{}, &PosixIdentityList{})
}
