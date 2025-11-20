/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
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

	// Validate the short-lived JWT token
	claims, err := s.jwtManager.ValidateToken(token)
	if err != nil {
		s.logger.Error("Invalid token", "error", err)
		http.Error(w, "Invalid token", http.StatusUnauthorized)
		return
	}

	// Validate token type - bearer auth should only accept bootstrap tokens
	if claims.TokenType != jwt.TokenTypeBootstrap {
		s.logger.Error("Invalid token type for bearer auth", "expected", jwt.TokenTypeBootstrap, "actual", claims.TokenType)
		http.Error(w, "Invalid token type", http.StatusUnauthorized)
		return
	}

	// Get headers for path extraction
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

	// Validate token claims against current path
	if claims.Path != appPath {
		s.logger.Error("Token path mismatch", "token_path", claims.Path, "request_path", appPath)
		http.Error(w, "Token path mismatch", http.StatusForbidden)
		return
	}

	// Generate new long-term session token
	sessionToken, err := s.jwtManager.GenerateToken(
		claims.Subject, claims.Groups, claims.UID, claims.Extra, appPath, host, jwt.TokenTypeSession)
	if err != nil {
		s.logger.Error("Failed to generate session token", "error", err, "user", claims.Subject)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set session cookie using appPath and same domain as JWT token
	s.cookieManager.SetCookie(w, sessionToken, appPath, host)

	// Log successful token exchange
	s.logger.Info("Token exchange successful",
		"user", claims.Subject,
		"path", appPath,
		"host", host)

	w.WriteHeader(http.StatusOK)
}
