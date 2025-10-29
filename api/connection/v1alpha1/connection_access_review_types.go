/*
Copyright 2024 The Jupyter-k8s Authors.

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
