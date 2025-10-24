// Package authmiddleware provides JWT-based authentication and authorization middleware
// for Jupyter-k8s workspaces, handling user identity, cookie management, and CSRF protection.
package authmiddleware

import (
	"context"
	"fmt"
	"regexp"

	v1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkspaceInfo holds extracted workspace information
type WorkspaceInfo struct {
	Namespace string
	Name      string
}

// ExtractWorkspaceInfo extracts workspace namespace and name from a path
// using the configured regex patterns
func (s *Server) ExtractWorkspaceInfo(path string) (*WorkspaceInfo, error) {
	if path == "" {
		return nil, fmt.Errorf("empty path")
	}

	// Extract namespace using the namespace regex pattern
	namespaceRe := regexp.MustCompile(s.config.WorkspaceNamespacePathRegex)
	namespaceMatches := namespaceRe.FindStringSubmatch(path)
	if len(namespaceMatches) != 2 {
		return nil, fmt.Errorf("failed to extract namespace from path: %s", path)
	}
	namespace := namespaceMatches[1]

	// Extract name using the name regex pattern
	nameRe := regexp.MustCompile(s.config.WorkspaceNamePathRegex)
	nameMatches := nameRe.FindStringSubmatch(path)
	if len(nameMatches) != 2 {
		return nil, fmt.Errorf("failed to extract workspace name from path: %s", path)
	}
	name := nameMatches[1]

	return &WorkspaceInfo{
		Namespace: namespace,
		Name:      name,
	}, nil
}

// createConnectionAccessReview calls create:ConnectionAccessReview API to verify if the username
// and/or groups have permission to connect to that particular Workspace in that particular Namespace.
// This API maps to the extension API of the controller, and performs both an RBAC and webhook admission
// control for the workspace/connection.
func (s *Server) createConnectionAccessReview(
	ctx context.Context,
	username string,
	groups []string,
	namespace string,
	workspaceName string,
) (*v1alpha1.ConnectionAccessReviewStatus, error) {
	if s.restClient == nil {
		return nil, fmt.Errorf("kubernetes REST client not initialized")
	}

	// Create a ConnectionAccessReview request
	reviewRequest := &v1alpha1.ConnectionAccessReview{
		ObjectMeta: v1.ObjectMeta{
			Namespace: namespace,
		},
		Spec: v1alpha1.ConnectionAccessReviewSpec{
			WorkspaceName: workspaceName,
			User:          username,
			Groups:        groups,
		},
	}

	// The ConnectionAccessReview requires using a REST client rather than a standard k8s client
	// because it needs to perform a Create:ConnectionAccessReview and read the response, which
	// k8sClient.Create() method not allow. There is no `Get:ConnectionAccessReview`.
	// Furthermore:
	// 1. As of Q4 2025, we cannot define custom subresources with kubebuilder
	// 2. Controller-runtime has limited support for custom subresources
	url := fmt.Sprintf("/apis/%s/namespaces/%s/connectionaccessreview",
		v1alpha1.SchemeGroupVersion.String(), namespace)

	// Create a REST request to create:ConnectionAccessReview to the extension server
	var result v1alpha1.ConnectionAccessReview
	err := s.restClient.Post().
		AbsPath(url).
		Body(reviewRequest).
		Do(ctx).
		Into(&result)

	if err != nil {
		s.logger.Error("create ConnectionAccessReview failed",
			"username", username,
			"workspace", workspaceName,
			"namespace", namespace,
			"error", err.Error())

		return nil, fmt.Errorf("failed to create ConnectionAccessReview: %w", err)
	}
	s.logger.Info("ConnectionAccessReview completed",
		"username", username,
		"workspace", workspaceName,
		"namespace", namespace,
		"allowed", result.Status.Allowed,
		"notFound", result.Status.NotFound,
		"reason", result.Status.Reason)

	return &result.Status, nil
}

// VerifyWorkspaceAccess checks if the user has access to the workspace path
// It extracts the workspace info from the path, call the connection API
// create:ConnectionAccessReview, return a boolean for access, the result
// of the ConnectionAccessReview and WorkspaceInfo.
func (s *Server) VerifyWorkspaceAccess(
	ctx context.Context,
	path string,
	username string,
	groups []string,
) (*v1alpha1.ConnectionAccessReviewStatus, *WorkspaceInfo, error) {
	// Extract workspace info from path
	workspaceInfo, err := s.ExtractWorkspaceInfo(path)
	if err != nil {
		s.logger.Info(fmt.Sprintf("Invalid workspace path: %s", path))
		return nil, nil, err
	}

	// Check if user and/or groups are authorized to create a connection to the Workspace
	accessReviewResult, err := s.createConnectionAccessReview(
		ctx,
		username,
		groups,
		workspaceInfo.Namespace,
		workspaceInfo.Name,
	)

	if err != nil {
		return nil, workspaceInfo, fmt.Errorf("failed to check workspace permission: %w", err)
	}

	return accessReviewResult, workspaceInfo, nil
}
