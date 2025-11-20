/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConnectionAccessReviewSpec defines the parameters of the ConnectionAccessReview
type ConnectionAccessReviewSpec struct {
	WorkspaceName string              `json:"workspaceName"`
	Groups        []string            `json:"groups"`
	UID           string              `json:"uid,omitempty"`
	User          string              `json:"user"`
	Extra         map[string][]string `json:"extra,omitempty"`
}

// ConnectionAccessReviewStatus defines the observed state of the ConnectionAccessReview
type ConnectionAccessReviewStatus struct {
	Allowed  bool   `json:"allowed"`
	NotFound bool   `json:"notFound"`
	Reason   string `json:"reason"`
}

// +kubebuilder:object:root=true

// ConnectionAccessReview is the schema for ConnectionAccessReview API
type ConnectionAccessReview struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ConnectionAccessReviewSpec   `json:"spec"`
	Status            ConnectionAccessReviewStatus `json:"status,omitempty"`
}
