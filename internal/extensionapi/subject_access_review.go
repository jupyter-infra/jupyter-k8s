// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"context"
	"fmt"

	authorizationv1 "k8s.io/api/authorization/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	rlog "github.com/go-logr/logr"
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
				Group:     "workspace.jupyter.org",
				Resource:  "workspaces/connection",
			},
			User:   username,
			Groups: groups,
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
