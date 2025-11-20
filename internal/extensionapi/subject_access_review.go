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

// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlog "github.com/go-logr/logr"

	connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
)

// RBACPermissionResult contains the result of an RBAC permission check
type RBACPermissionResult struct {
	Allowed bool
	Reason  string
}

// CheckRBACPermission checks if a user has the specified RBAC permissions
// using the Kubernetes SubjectAccessReview API
func (s *ExtensionServer) CheckRBACPermission(
	namespace string,
	username string,
	groups []string,
	uid string,
	extra map[string]authorizationv1.ExtraValue,
	logger *rlog.Logger,
) (*RBACPermissionResult, error) {
	sarClient := s.sarClient

	// Create a SubjectAccessReview to check RBAC permissions
	sarRequest := &authorizationv1.SubjectAccessReview{
		Spec: authorizationv1.SubjectAccessReviewSpec{
			ResourceAttributes: &authorizationv1.ResourceAttributes{
				Namespace: namespace,
				Verb:      "create",
				Group:     connectionv1alpha1.SchemeGroupVersion.Group,
				Resource:  "workspaceconnections",
			},
			User:   username,
			Groups: groups,
			UID:    uid,
			Extra:  extra,
		},
	}

	// Create the SubjectAccessReview
	sarResult, err := sarClient.Create(
		context.Background(),
		sarRequest,
		v1.CreateOptions{})

	if err != nil {
		logger.Error(err, "Failed to create SubjectAccessReview")
		return nil, fmt.Errorf("failed to create SubjectAccessReview: %w", err)
	}

	logger.Info("SubjectAccessReview result",
		"username", username,
		"allowed", sarResult.Status.Allowed,
		"reason", sarResult.Status.Reason)

	return &RBACPermissionResult{
		Allowed: sarResult.Status.Allowed,
		Reason:  sarResult.Status.Reason,
	}, nil
}
