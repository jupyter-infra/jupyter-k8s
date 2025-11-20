/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlog "github.com/go-logr/logr"

	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
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
