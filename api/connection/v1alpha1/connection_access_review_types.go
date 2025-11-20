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
