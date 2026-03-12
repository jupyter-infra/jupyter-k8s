/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BearerTokenReviewSpec defines the parameters of the BearerTokenReview
type BearerTokenReviewSpec struct {
	Token string `json:"token"`
}

// BearerTokenReviewUser holds the identity extracted from a verified bearer token
type BearerTokenReviewUser struct {
	Username string              `json:"username"`
	UID      string              `json:"uid,omitempty"`
	Groups   []string            `json:"groups,omitempty"`
	Extra    map[string][]string `json:"extra,omitempty"`
}

// BearerTokenReviewStatus defines the result of the BearerTokenReview
type BearerTokenReviewStatus struct {
	Authenticated bool                  `json:"authenticated"`
	Path          string                `json:"path,omitempty"`
	Domain        string                `json:"domain,omitempty"`
	User          BearerTokenReviewUser `json:"user,omitempty"`
	Error         string                `json:"error,omitempty"`
}

// +kubebuilder:object:root=true

// BearerTokenReview is the schema for BearerTokenReview API
type BearerTokenReview struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              BearerTokenReviewSpec   `json:"spec"`
	Status            BearerTokenReviewStatus `json:"status,omitempty"`
}
