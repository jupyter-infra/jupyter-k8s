// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
	authorizationv1 "k8s.io/api/authorization/v1"
)

// handleConnectionAccessReview handles requests to the connectionaccessreview resource
func (s *ExtensionServer) handleConnectionAccessReview(w http.ResponseWriter, r *http.Request) {
	logger := GetLoggerFromContext(r.Context())

	if r.Method != "POST" {
		WriteError(w, http.StatusBadRequest, "ConnectionAccessReview must use POST method")
		return
	}

	logger.Info("Handling ConnectionAccessReview", "method", r.Method, "path", r.URL.Path)

	// Parse path to extract namespace
	namespace, err := GetNamespaceFromPath(r.URL.Path)

	if err != nil {
		logger.Error(err, "Failed to retrieve the namespace")
		WriteError(w, http.StatusBadRequest, "ConnectionAccessReview must be namespaced")
		return
	}

	// Read and parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error(err, "Failed to read request body")
		WriteError(w, http.StatusBadRequest, "Failed to read request body")
		return
	}

	var accessReview connectionv1alpha1.ConnectionAccessReview
	if err := json.Unmarshal(body, &accessReview); err != nil {
		logger.Error(err, "Failed to unmarshal ConnectionAccessReview")
		WriteError(w, http.StatusBadRequest, "Invalid ConnectionAccessReview format")
		return
	}

	// Set namespace in the object metadata
	accessReview.Namespace = namespace

	// Ensure workspaceName is provided in the spec
	if accessReview.Spec.WorkspaceName == "" {
		logger.Error(
			fmt.Errorf("missing workspace name"),
			"WorkspaceName not provided in ConnectionAccessReview spec",
		)
		WriteError(w, http.StatusBadRequest, "WorkspaceName is required in the spec")
		return
	}

	// Check if the user has permission to access the workspace
	workspaceName := accessReview.Spec.WorkspaceName
	username := accessReview.Spec.User
	groups := accessReview.Spec.Groups

	// Convert map[string][]string to map[string]authorizationv1.ExtraValue
	extra := make(map[string]authorizationv1.ExtraValue)
	for k, v := range accessReview.Spec.Extra {
		extra[k] = authorizationv1.ExtraValue(v)
	}

	result, err := s.CheckWorkspaceConnectionPermission(namespace, workspaceName, username, groups, extra, &logger)
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "Failed to verify access permission")
		return
	}

	accessReview.Status = connectionv1alpha1.ConnectionAccessReviewStatus{
		Allowed:  result.Allowed,
		NotFound: result.NotFound,
		Reason:   result.Reason,
	}

	logger.Info(
		"ConnectionAccessReview result",
		"user", username,
		"groups", strings.Join(groups, ","),
		"workspace", workspaceName,
		"allowed", accessReview.Status.Allowed,
		"reason", accessReview.Status.Reason,
	)

	// Return the response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(accessReview); err != nil {
		logger.Error(err, "Failed to encode response")
		WriteError(w, http.StatusInternalServerError, "Failed to encode response")
		return
	}
}
