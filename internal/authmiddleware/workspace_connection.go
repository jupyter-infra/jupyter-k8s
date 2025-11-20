// Package authmiddleware provides JWT-based authentication and authorization middleware
// for Jupyter-k8s workspaces, handling user identity and cookie management.
package authmiddleware

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	v1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	"github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WorkspaceInfo holds extracted workspace information
type WorkspaceInfo struct {
	Namespace string
	Name      string
}

// ExtractWorkspaceInfo extracts workspace namespace and name from request
// using the configured routing mode and regex patterns
func (s *Server) ExtractWorkspaceInfo(r *http.Request) (*WorkspaceInfo, error) {
	switch s.config.RoutingMode {
	case RoutingModeSubdomain:
		return s.extractWorkspaceInfoFromSubdomain(r)
	case RoutingModePath:
		return s.extractWorkspaceInfoFromPath(r)
	default:
		return nil, fmt.Errorf("unsupported routing mode: %s", s.config.RoutingMode)
	}
}

// extractWorkspaceInfoFromPath extracts workspace info from URL path
func (s *Server) extractWorkspaceInfoFromPath(r *http.Request) (*WorkspaceInfo, error) {
	path, err := GetForwardedURI(r)
	if err != nil {
		return nil, err
	}

	// Extract namespace using regex
	namespaceRe := regexp.MustCompile(s.config.WorkspaceNamespacePathRegex)
	namespaceMatches := namespaceRe.FindStringSubmatch(path)
	if len(namespaceMatches) != 2 {
		return nil, fmt.Errorf("failed to extract namespace from path: %s", path)
	}

	// Extract workspace name using regex
	nameRe := regexp.MustCompile(s.config.WorkspaceNamePathRegex)
	nameMatches := nameRe.FindStringSubmatch(path)
	if len(nameMatches) != 2 {
		return nil, fmt.Errorf("failed to extract workspace name from path: %s", path)
	}

	return &WorkspaceInfo{
		Namespace: namespaceMatches[1],
		Name:      nameMatches[1],
	}, nil
}

// extractWorkspaceInfoFromSubdomain extracts workspace info from subdomain
func (s *Server) extractWorkspaceInfoFromSubdomain(r *http.Request) (*WorkspaceInfo, error) {
	host, err := GetForwardedHost(r)
	if err != nil {
		return nil, err
	}

	// Extract subdomain part (before first dot)
	subdomain := ExtractSubdomain(host)

	// Extract workspace name using regex
	nameRe := regexp.MustCompile(s.config.WorkspaceNameSubdomainRegex)
	nameMatches := nameRe.FindStringSubmatch(subdomain)
	if len(nameMatches) != 2 {
		return nil, fmt.Errorf("failed to extract workspace name from subdomain: %s", subdomain)
	}

	// Extract namespace using regex
	namespaceRe := regexp.MustCompile(s.config.WorkspaceNamespaceSubdomainRegex)
	namespaceMatches := namespaceRe.FindStringSubmatch(subdomain)
	if len(namespaceMatches) != 2 {
		return nil, fmt.Errorf("failed to extract namespace from subdomain: %s", subdomain)
	}

	// Decode base32 encoded namespace
	namespace, err := workspace.DecodeNamespaceB32(namespaceMatches[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode namespace from base32: %w", err)
	}

	return &WorkspaceInfo{
		Namespace: namespace,
		Name:      nameMatches[1],
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
	uid string,
	extra map[string][]string,
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

	// Add the optional auth context arguments
	if len(extra) > 0 {
		reviewRequest.Spec.Extra = extra
	}
	if uid != "" {
		reviewRequest.Spec.UID = uid
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

// VerifyWorkspaceAccess checks if the user has access to the workspace
// It extracts the workspace info from the request, calls the connection API
// create:ConnectionAccessReview, return the result and WorkspaceInfo.
func (s *Server) VerifyWorkspaceAccess(
	ctx context.Context,
	r *http.Request,
	username string,
	groups []string,
	uid string,
	extra map[string][]string,
) (*v1alpha1.ConnectionAccessReviewStatus, *WorkspaceInfo, error) {
	// Extract workspace info from request
	workspaceInfo, err := s.ExtractWorkspaceInfo(r)
	if err != nil {
		s.logger.Info(fmt.Sprintf("Invalid workspace request: %v", err))
		return nil, nil, err
	}

	// Check if user and/or groups are authorized to create a connection to the Workspace
	accessReviewResult, err := s.createConnectionAccessReview(
		ctx,
		username,
		groups,
		workspaceInfo.Namespace,
		workspaceInfo.Name,
		uid,
		extra,
	)

	if err != nil {
		return nil, workspaceInfo, fmt.Errorf("failed to check workspace permission: %w", err)
	}

	return accessReviewResult, workspaceInfo, nil
}

// VerifyWorkspaceAccessFromJwt retrieves the user auth context as recorded in the JWT
// (consisting in: username, groups, uid, extra), extracts the workspace info from the path,
// call the connection API which makes a create:ConnectionAccessReview call,
// return the result and WorkspaceInfo.
func (s *Server) VerifyWorkspaceAccessFromJwt(
	ctx context.Context,
	r *http.Request,
	claims *jwt.Claims,
) (*v1alpha1.ConnectionAccessReviewStatus, *WorkspaceInfo, error) {
	// Extract workspace info from request
	workspaceInfo, err := s.ExtractWorkspaceInfo(r)
	if err != nil {
		s.logger.Info(fmt.Sprintf("Invalid workspace request: %v", err))
		return nil, nil, err
	}

	username := claims.User
	groups := claims.Groups
	uid := claims.UID
	extra := claims.Extra

	// Check if user and/or groups are authorized to create a connection to the Workspace
	accessReviewResult, err := s.createConnectionAccessReview(
		ctx,
		username,
		groups,
		workspaceInfo.Namespace,
		workspaceInfo.Name,
		uid,
		extra,
	)

	if err != nil {
		return nil, workspaceInfo, fmt.Errorf("failed to check workspace permission: %w", err)
	}

	return accessReviewResult, workspaceInfo, nil

}
