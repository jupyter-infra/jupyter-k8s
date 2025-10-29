package authmiddleware

import (
	"net/http"
	"strings"
)

// handleVerify handles token verification requests
func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	// Get requested path from header
	requestPath := r.Header.Get(HeaderForwardedURI)
	requestDomain := r.Header.Get(HeaderForwardedHost)

	// Validate required headers
	if requestPath == "" {
		s.logger.Info("Missing " + HeaderForwardedURI + " header")
		http.Error(w, "Missing "+HeaderForwardedURI+" header", http.StatusBadRequest)
		return
	}

	if requestDomain == "" {
		s.logger.Info("Missing " + HeaderForwardedHost + " header")
		http.Error(w, "Missing "+HeaderForwardedHost+" header", http.StatusBadRequest)
		return
	}

	// Get path-specific cookie by hashing full path, retrieve embedded JWT
	token, err := s.cookieManager.GetCookie(r, requestPath)
	if err != nil {
		s.logger.Info("No auth cookie found", "error", err, "path", requestPath)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Validate token
	claims, err := s.jwtManager.ValidateToken(token)
	if err != nil {
		s.logger.Info("Invalid token", "error", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Validate token type - verify should only accept session tokens
	if claims.TokenType != TokenTypeSession {
		s.logger.Info("Invalid token type for verify", "expected", TokenTypeSession, "actual", claims.TokenType)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify token path matches requested path or is a parent path
	if claims.Path != "" && requestPath != "" {
		if !strings.HasPrefix(requestPath, claims.Path) {
			s.logger.Warn("Path mismatch", "token_path", claims.Path, "request_path", requestPath)
			http.Error(w, "Path not authorized", http.StatusForbidden)
			return
		}
	}

	// Verify token domain matches request domain
	if claims.Domain != requestDomain {
		s.logger.Warn("Domain mismatch", "error", err, "token_domain", claims.Domain, "request_domain", requestDomain)
		http.Error(w, "Domain not authorized", http.StatusForbidden)
		return
	}

	// Check if token needs to be refreshed
	if s.jwtManager.ShouldRefreshToken(claims) {
		s.logger.Debug("Refreshing token", "user", claims.User, "path", claims.Path)

		// Verify that the user still has access to the specific Workspace
		accessReviewResult, workspaceInfo, accessErr := s.VerifyWorkspaceAccessFromJwt(r.Context(), requestPath, claims)

		// UNHAPPY CASE 1: we can't check authZ for some reason, stop attempting to refresh
		if accessErr != nil {
			s.logger.Warn("Failed to retrieve the accessReview for cookie refresh", "error", err)
			newToken, err := s.jwtManager.UpdateSkipRefreshToken(claims)
			if err != nil {
				s.logger.Warn("Failed to update token to skip", "error", err)
			} else {
				// Set refreshed cookie with the same path as the original token
				s.cookieManager.SetCookie(w, newToken, claims.Path)
				s.logger.Info("Token refreshed successfully", "user", claims.User, "path", claims.Path)
			}
			// UNHAPPY CASE 2: user is no longer allowed, return 403
		} else if !accessReviewResult.Allowed {
			s.logger.Info(
				"JWT renewal denied: ConnectionAccessReview.Allowed is false",
				"workspace",
				workspaceInfo.Name,
				"workspaceNamespace",
				workspaceInfo.Namespace,
				"reason",
				accessReviewResult.Reason)
			s.cookieManager.ClearCookie(w, claims.Path)
			http.Error(w, "Access denied: you are no longer authorized to access this workspace", http.StatusForbidden)
			return
			// HAPPY CASE: user is allowed, refresh their cookie
		} else {
			// Refresh token
			newToken, err := s.jwtManager.RefreshToken(claims)
			if err != nil {
				// Log but don't fail the request - just continue with the existing token
				s.logger.Warn("Failed to refresh token", "error", err)
			} else {
				// Set refreshed cookie with the same path as the original token
				s.cookieManager.SetCookie(w, newToken, claims.Path)
				s.logger.Info("Token refreshed successfully", "user", claims.User, "path", claims.Path)
			}
		}
	}

	w.WriteHeader(http.StatusOK)
}
