package authmiddleware

import (
	"encoding/json"
	"net/http"
)

// handleAuth handles authentication requests
func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get headers from request
	user := r.Header.Get(HeaderAuthRequestUser)
	groups := r.Header.Get(HeaderAuthRequestGroups)
	fullPath := r.Header.Get(HeaderForwardedURI)
	host := r.Header.Get(HeaderForwardedHost)
	proto := r.Header.Get(HeaderForwardedProto)

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

	// Generate JWT token with app path and domain for authorization scope
	token, err := s.jwtManager.GenerateToken(user, splitGroups(groups), appPath, host)
	if err != nil {
		s.logger.Error("Failed to generate token", "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set cookie using appPath
	// passing full_path would fallback to `app_path` anyway
	s.cookieManager.SetCookie(w, token, appPath)

	// Generate CSRF token for the form
	csrfToken := s.cookieManager.GenerateCSRFToken(r)

	// Create response with CSRF token
	response := map[string]string{
		"user":       user,
		"groups":     groups,
		"path":       fullPath,
		"app_path":   appPath,
		"csrf_token": csrfToken,
	}

	// Determine redirect URL if provided in query
	redirectURL := r.URL.Query().Get("redirect")
	if redirectURL != "" {
		// Validate redirect URL (should be relative or same host)
		if !isValidRedirectURL(redirectURL, host) {
			http.Error(w, "Invalid redirect URL", http.StatusBadRequest)
			return
		}

		// If redirect URL doesn't have protocol, add it
		if !hasProtocol(redirectURL) && proto != "" {
			redirectURL = proto + "://" + host + redirectURL
		}

		response["redirect"] = redirectURL
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		s.logger.Error("Failed to encode JSON response", "error", err)
	}
}
