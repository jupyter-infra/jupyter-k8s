/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"net/http"
	"net/url"

	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
)

// handleBearerAuth handles bearer token authentication requests
// Takes short lived JWT tokens from URL parameter and exchanges them for 6-hour session cookies
func (s *Server) handleBearerAuth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get the original forwarded URI which contains the token
	forwardedURI := r.Header.Get(HeaderForwardedURI)
	if forwardedURI == "" {
		s.logger.Error("Missing forwarded URI header")
		http.Error(w, "Missing "+HeaderForwardedURI+" header", http.StatusBadRequest)
		return
	}

	// Parse the forwarded URI to extract query parameters
	parsedURL, err := url.Parse(forwardedURI)
	if err != nil {
		s.logger.Error("Invalid forwarded URI", "uri", forwardedURI, "error", err)
		http.Error(w, "Invalid forwarded URI", http.StatusBadRequest)
		return
	}

	// Extract token from forwarded URI query parameters
	token := parsedURL.Query().Get("token")
	if token == "" {
		s.logger.Error("Missing token parameter",
			"forwarded_uri", forwardedURI,
			"query_params", parsedURL.Query())
		http.Error(w, "Missing token parameter", http.StatusBadRequest)
		return
	}

	// Get headers for path/host extraction
	fullPath := forwardedURI
	host := r.Header.Get(HeaderForwardedHost)

	// Validate required headers
	if host == "" {
		s.logger.Error("Missing forwarded host header")
		http.Error(w, "Missing "+HeaderForwardedHost+" header", http.StatusBadRequest)
		return
	}

	// Extract base app path for validation
	appPath := ExtractAppPath(fullPath, s.config.PathRegexPattern)
	s.logger.Debug("Extracted app path for validation", "full_path", fullPath, "app_path", appPath)

	var user, uid string
	var groups []string
	var extra map[string][]string

	if s.config.KMSKeyId != "" {
		// Local KMS validation — existing behavior
		claims, err := s.jwtManager.ValidateToken(token)
		if err != nil {
			s.logger.Error("Invalid token", "error", err)
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if claims.TokenType != jwt.TokenTypeBootstrap {
			s.logger.Error("Invalid token type for bearer auth", "expected", jwt.TokenTypeBootstrap, "actual", claims.TokenType)
			http.Error(w, "Invalid token type", http.StatusUnauthorized)
			return
		}

		if claims.Path != appPath {
			s.logger.Error("Token path mismatch", "token_path", claims.Path, "request_path", appPath)
			http.Error(w, "Token path mismatch", http.StatusForbidden)
			return
		}

		user = claims.Subject
		uid = claims.UID
		groups = claims.Groups
		extra = claims.Extra
	} else {
		// Delegate to BearerTokenReview API — token is opaque
		wsInfo, err := s.ExtractWorkspaceInfo(r)
		if err != nil {
			s.logger.Error("Failed to extract workspace info", "error", err)
			http.Error(w, "Failed to extract workspace info", http.StatusBadRequest)
			return
		}

		reviewStatus, err := s.createBearerTokenReview(r.Context(), token, wsInfo.Namespace)
		if err != nil {
			s.logger.Error("BearerTokenReview failed", "error", err)
			http.Error(w, "Token verification failed", http.StatusInternalServerError)
			return
		}

		if !reviewStatus.Authenticated {
			s.logger.Error("Bearer token not authenticated", "error", reviewStatus.Error)
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		if reviewStatus.Path != appPath {
			s.logger.Error("Token path mismatch", "token_path", reviewStatus.Path, "request_path", appPath)
			http.Error(w, "Token path mismatch", http.StatusForbidden)
			return
		}

		user = reviewStatus.User.Username
		uid = reviewStatus.User.UID
		groups = reviewStatus.User.Groups
		extra = reviewStatus.User.Extra
	}

	// Generate new long-term session token
	sessionToken, err := s.jwtManager.GenerateToken(
		user, groups, uid, extra, appPath, host, jwt.TokenTypeSession)
	if err != nil {
		s.logger.Error("Failed to generate session token", "error", err, "user", user)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set session cookie using appPath and same domain as JWT token
	s.cookieManager.SetCookie(w, sessionToken, appPath, host)

	// Log successful token exchange
	s.logger.Info("Token exchange successful",
		"user", user,
		"path", appPath,
		"host", host)

	w.WriteHeader(http.StatusOK)
}
