package authmiddleware

import (
	"encoding/json"
	"net/http"

	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
)

// handleAuth handles authentication requests
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get headers from request
	fullPath := r.Header.Get(HeaderForwardedURI)
	host := r.Header.Get(HeaderForwardedHost)
	authHeader := r.Header.Get(HeaderAuthorization)

	// Get headers for verification with OIDC claims
	headerUID := r.Header.Get(HeaderAuthRequestUser)
	headerPreferredUsername := GetOidcUsername(s.config, r.Header.Get(HeaderAuthRequestPreferredUsername))
	headerGroups := GetOidcGroups(s.config, splitGroups(r.Header.Get(HeaderAuthRequestGroups)))

	// Extract base app path for JWT authorization
	appPath := ExtractAppPath(fullPath, s.config.PathRegexPattern)
	s.logger.Debug("Extracted app path for authorization", "full_path", fullPath, "app_path", appPath)

	// Validate required headers
	if fullPath == "" {
		http.Error(w, "Missing "+HeaderForwardedURI+" header", http.StatusBadRequest)
		return
	}

	if host == "" {
		http.Error(w, "Missing "+HeaderForwardedHost+" header", http.StatusBadRequest)
		return
	}

	// Authorization is required
	if authHeader == "" {
		s.logger.Error("Missing Authorization header")
		http.Error(w, "Missing Authorization header", http.StatusUnauthorized)
		return
	}

	// OIDCVerifier should always be initialized when /auth is enabled
	if s.oidcVerifier == nil {
		s.logger.Error("OIDC verifier is not initialized")
		http.Error(w, "Internal server error: OIDC verifier not initialized", http.StatusInternalServerError)
		return
	}

	// Extract token from Authorization header
	token, err := ExtractBearerToken(authHeader)
	if err != nil {
		s.logger.Error("Failed to extract bearer token", "error", err)
		http.Error(w, "Invalid Authorization header", http.StatusBadRequest)
		return
	}

	// Verify the token with the OIDC provider
	oidcClaims, isVerifyTokenFault, err := s.oidcVerifier.VerifyToken(r.Context(), token, s.logger)
	if err != nil {
		if isVerifyTokenFault {
			// Server-side error (e.g., OIDC provider unavailable)
			s.logger.Error("OIDC provider connection error", "error", err)
			http.Error(w, "Internal server error: OIDC provider not available", http.StatusInternalServerError)
			return
		}

		// Otherwise the token is invalid, reject with 400
		s.logger.Error("OIDC token validation error", "error", err)
		http.Error(w, "Invalid or expired OIDC token", http.StatusForbidden)
		return
	}

	// Set the user identity variables from the OIDC token
	k8sUID := oidcClaims.Subject
	k8sUsername := GetOIDCUsernameFromToken(s.config, oidcClaims)
	k8sGroups := GetOIDCGroupsFromToken(s.config, oidcClaims)

	// Verify preferred username in header if available
	if headerPreferredUsername != "" && k8sUsername != headerPreferredUsername {
		s.logger.Error("Preferred username mismatch between token and headers",
			"token preferred username", k8sUsername,
			"header preferred username", headerPreferredUsername)
		http.Error(w, "Username mismatch between token and headers", http.StatusUnauthorized)
		return
	}

	// Verify UID in header if available
	if headerUID != "" && k8sUID != headerUID {
		s.logger.Error("UID mismatch between token and headers",
			"token UID", k8sUID,
			"header UID", headerUID)
		http.Error(w, "UID verification failed", http.StatusUnauthorized)
		return
	}

	// Verify groups in header if available
	if len(headerGroups) > 0 {
		ok, missingGroups := EnsureSubsetOf(headerGroups, k8sGroups)
		if !ok {
			s.logger.Error("Groups mismatch between token and headers", "missing groups in token", missingGroups)
			http.Error(w, "Groups verification failed", http.StatusUnauthorized)
			return
		}
	}

	// Check workspace access permission
	// and the Kubernetes REST client is available
	if s.restClient == nil {
		s.logger.Error("cannot authorize, REST client not set")
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Verify workspace access using the new extracted function
	connectionAccessReviewResult, workspaceInfo, err := s.VerifyWorkspaceAccess(
		r.Context(),
		r,
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

	// Generate JWT token with app path and domain for authorization scope
	jwtToken, err := s.jwtManager.GenerateToken(k8sUsername, k8sGroups, k8sUID, nil, appPath, host, jwt.TokenTypeSession)
	if err != nil {
		s.logger.Error("Failed to generate token", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set cookie using appPath and same domain as JWT token
	s.cookieManager.SetCookie(w, jwtToken, appPath, host)

	// Create empty response
	response := map[string]string{}

	// Log successful connection
	s.logger.Info("Connection successful",
		"user", k8sUID,
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
