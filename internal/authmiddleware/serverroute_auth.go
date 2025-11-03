package authmiddleware

import (
	"encoding/json"
	"net/http"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/jwt"
)

// handleAuth handles authentication requests
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get headers from request
	user := r.Header.Get(HeaderAuthRequestUser)
	preferredUsername := r.Header.Get(HeaderAuthRequestPreferredUsername)
	groups := r.Header.Get(HeaderAuthRequestGroups)
	fullPath := r.Header.Get(HeaderForwardedURI)
	host := r.Header.Get(HeaderForwardedHost)

	// Extract base app path for JWT authorization
	appPath := ExtractAppPath(fullPath, s.config.PathRegexPattern)
	s.logger.Debug("Extracted app path for authorization", "full_path", fullPath, "app_path", appPath)

	// Validate required headers
	if user == "" {
		http.Error(w, "Missing "+HeaderAuthRequestUser+" header", http.StatusBadRequest)
		return
	}

	// Allow Groups header to be empty (user may not belong to any group)

	if fullPath == "" {
		http.Error(w, "Missing "+HeaderForwardedURI+" header", http.StatusBadRequest)
		return
	}

	if host == "" {
		http.Error(w, "Missing "+HeaderForwardedHost+" header", http.StatusBadRequest)
		return
	}

	// Determine k8s username
	k8sUID := user
	k8sUsername := GetOidcUsername(s.config, preferredUsername)
	k8sGroups := GetOidcGroups(s.config, splitGroups(groups))

	// Check workspace access permission
	// and the Kubernetes REST client is available
	if s.restClient == nil {
		s.logger.Error("cannot authorize, REST client not set")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if s.restClient != nil {
		// Verify workspace access using the new extracted function
		connectionAccessReviewResult, workspaceInfo, err := s.VerifyWorkspaceAccess(
			r.Context(),
			appPath,
			k8sUsername,
			k8sGroups,
			k8sUID,
			nil, // we cannot retrieve user.Info.GetExtra() in oauth flow
		)

		if err != nil {
			s.logger.Error("Failed to verify workspace access", "error", err, "path", appPath)
			http.Error(w, "Failed to verify workspace access", http.StatusInternalServerError)
			return
		}

		allowed := connectionAccessReviewResult.Allowed
		notFound := connectionAccessReviewResult.NotFound

		if !allowed || notFound {
			s.logger.Info("Workspace connection refused",
				"username", k8sUsername,
				"workspace", workspaceInfo.Name,
				"namespace", workspaceInfo.Namespace,
				"workspaceNotFound", connectionAccessReviewResult.NotFound,
				"reason", connectionAccessReviewResult.Reason,
			)

			// Workspace was not found maps to Access denied error.
			// If the workspace does not exist, we cannot know whether the user would have access
			// given such decision depends on a/ the Workspace.Spec.AccessType, b/ whether the user
			// is the owner of the Workspace, c/ when we support peer-to-peer sharing, whether the
			// owner shared their workspace to the user or one of the group they belong to.

			// Note: if the Workspace is in Stopped state, we could trigger an automatic restart
			// when a user makes an authorized connection request. However, we should use a different
			// route of the authmiddleware and use a redirect (possibly '/restart')
			http.Error(w, "Access denied: you are not authorized to connect to this workspace", http.StatusForbidden)
			return
		}

		s.logger.Info("Connection to the workspace granted",
			"username", k8sUsername,
			"workspace", workspaceInfo.Name,
			"namespace", workspaceInfo.Namespace,
			"reason", connectionAccessReviewResult.Reason,
		)
	}

	// Generate JWT token with app path and domain for authorization scope
	token, err := s.jwtManager.GenerateToken(k8sUsername, k8sGroups, k8sUID, nil, appPath, host, jwt.TokenTypeSession)
	if err != nil {
		s.logger.Error("Failed to generate token", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set cookie using appPath
	// passing full_path would fallback to `app_path` anyway
	s.cookieManager.SetCookie(w, token, appPath)

	// Create empty response
	response := map[string]string{}

	// Log successful connection
	s.logger.Info("Connection successful",
		"user", user,
		"username", k8sUsername,
		"path", appPath,
		"groups", k8sGroups)

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode JSON response", "error", err)
	}
}
