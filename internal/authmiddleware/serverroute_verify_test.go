package authmiddleware

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testAppPath2 = "/workspaces/ns2/app2"
)

// TestHandleVerifyMissingUriHeader tests that handleVerify returns 400 if missing X-Forwarded-Uri header
func TestHandleVerifyMissingUriHeader(t *testing.T) {
	// Create a request without X-Forwarded-Uri header
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	req.Header.Set("X-Forwarded-Host", "example.com")
	w := httptest.NewRecorder()

	// Create a Server with minimal setup
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &Config{
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
	}
	server := &Server{
		config: cfg,
		logger: logger,
	}

	// Call handler
	server.handleVerify(w, req)

	// Check response
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !strings.Contains(body, "Missing X-Forwarded-Uri header") {
		t.Errorf("Expected error message about missing header, got: %s", body)
	}
}

// TestHandleVerifyMissingHostHeader tests that handleVerify returns 400 if missing X-Forwarded-Host header
func TestHandleVerifyMissingHostHeader(t *testing.T) {
	fwdUrl := fmt.Sprintf("%s/lab", testAppPath2)

	// Create a request without X-Forwarded-Host header
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	req.Header.Set("X-Forwarded-Uri", fwdUrl)
	w := httptest.NewRecorder()

	// Create a Server with minimal setup
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &Config{
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
	}
	server := &Server{
		config: cfg,
		logger: logger,
	}

	// Call handler
	server.handleVerify(w, req)

	// Check response
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !strings.Contains(body, "Missing X-Forwarded-Host header") {
		t.Errorf("Expected error message about missing header, got: %s", body)
	}
}

// TestHandleVerifyMissingCookie tests that handleVerify returns 401 if cookie is missing
func TestHandleVerifyMissingCookie(t *testing.T) {
	fwdUrl := fmt.Sprintf("%s/lab", testAppPath2)

	// Create a request with required headers
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	req.Header.Set("X-Forwarded-Uri", fwdUrl)
	req.Header.Set("X-Forwarded-Host", "example.com")
	w := httptest.NewRecorder()

	// Create cookie handler mock that returns an error
	cookieHandler := &MockCookieHandler{
		GetCookieFunc: func(r *http.Request, path string) (string, error) {
			// Verify parameter
			if path != fwdUrl {
				t.Errorf("Expected path '%s', got '%s'", fwdUrl, path)
			}
			return "", errors.New("no cookie found")
		},
	}

	// Create server with mocks
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &Config{
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
	}
	server := &Server{
		config:        cfg,
		logger:        logger,
		cookieManager: cookieHandler,
	}

	// Call handler
	server.handleVerify(w, req)

	// Check response
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !strings.Contains(body, "Unauthorized") {
		t.Errorf("Expected error message about unauthorized, got: %s", body)
	}
}

// For this test file, we are using mock implementations of the JWTHandler and CookieHandler interfaces
// defined in mock_test.go within the same package.

// TestHandleVerifyInvalidJWT tests that handleVerify returns 401 if JWT is invalid
func TestHandleVerifyInvalidJWT(t *testing.T) {
	fwdUrl := fmt.Sprintf("%s/lab", testAppPath2)

	// Create a request with required headers
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	req.Header.Set("X-Forwarded-Uri", fwdUrl)
	req.Header.Set("X-Forwarded-Host", "example.com")
	w := httptest.NewRecorder()

	// Create mock handlers
	cookieHandler := &MockCookieHandler{
		GetCookieFunc: func(r *http.Request, path string) (string, error) {
			return "invalid-token", nil
		},
	}

	jwtHandler := &MockJWTHandler{
		ValidateTokenFunc: func(tokenString string) (*Claims, error) {
			// Verify parameter
			if tokenString != "invalid-token" {
				t.Errorf("Expected token 'invalid-token', got '%s'", tokenString)
			}
			return nil, errors.New("invalid token")
		},
	}

	// Create server with mocks
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &Config{
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
	}
	server := &Server{
		config:        cfg,
		logger:        logger,
		cookieManager: cookieHandler,
		jwtManager:    jwtHandler,
	}

	// Call handler
	server.handleVerify(w, req)

	// Check response
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !strings.Contains(body, "Unauthorized") {
		t.Errorf("Expected error message about unauthorized, got: %s", body)
	}
}

// TestHandleVerifyNoRefreshBeforeWindow tests that handleVerify does not refresh the JWT before the RefreshWindow and returns a 2xx
func TestHandleVerifyNoRefreshBeforeWindow(t *testing.T) {
	fwdUrl := fmt.Sprintf("%s/lab", testAppPath2)

	// Create a request with required headers
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	req.Header.Set("X-Forwarded-Uri", fwdUrl)
	req.Header.Set("X-Forwarded-Host", "example.com")
	w := httptest.NewRecorder()

	// Create claims
	claims := &Claims{
		User:      "testuser",
		Groups:    []string{"group1", "group2"},
		Path:      testAppPath2,
		Domain:    "example.com",
		TokenType: TokenTypeSession, // Add session token type
	}

	// Track method calls
	tokenValidated := false
	tokenRefreshChecked := false
	tokenRefreshed := false

	// Create mock handlers
	cookieHandler := &MockCookieHandler{
		GetCookieFunc: func(r *http.Request, path string) (string, error) {
			return "valid-token", nil
		},
		SetCookieFunc: func(w http.ResponseWriter, token string, path string) {
			t.Error("SetCookie should not have been called")
		},
	}

	jwtHandler := &MockJWTHandler{
		ValidateTokenFunc: func(tokenString string) (*Claims, error) {
			tokenValidated = true
			return claims, nil
		},
		ShouldRefreshTokenFunc: func(claims *Claims) bool {
			tokenRefreshChecked = true
			// Don't refresh
			return false
		},
		RefreshTokenFunc: func(claims *Claims) (string, error) {
			tokenRefreshed = true
			return "new-token", nil
		},
	}

	// Create server with mocks
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &Config{
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
	}
	server := &Server{
		config:        cfg,
		logger:        logger,
		cookieManager: cookieHandler,
		jwtManager:    jwtHandler,
	}

	// Call handler
	server.handleVerify(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Verify method calls
	if !tokenValidated {
		t.Error("Token was not validated")
	}
	if !tokenRefreshChecked {
		t.Error("ShouldRefreshToken was not called")
	}
	if tokenRefreshed {
		t.Error("RefreshToken should not have been called")
	}
}

// TestHandleVerifyWithRefresh tests that handleVerify refreshes the JWT during the RefreshWindow and returns a 2xx
func TestHandleVerifyWithRefresh(t *testing.T) {
	fwdUrl := fmt.Sprintf("%s/lab", testAppPath2)

	// Create a request with required headers
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	req.Header.Set("X-Forwarded-Uri", fwdUrl)
	req.Header.Set("X-Forwarded-Host", "example.com")
	w := httptest.NewRecorder()

	// Create claims
	claims := &Claims{
		User:      "testuser",
		Groups:    []string{"group1", "group2"},
		Path:      testAppPath2,
		Domain:    "example.com",
		TokenType: TokenTypeSession, // Add session token type
	}

	// Track method calls
	tokenValidated := false
	tokenRefreshChecked := false
	tokenRefreshed := false
	cookieSet := false
	newToken := "refreshed-token"

	// Create mock handlers
	cookieHandler := &MockCookieHandler{
		GetCookieFunc: func(r *http.Request, path string) (string, error) {
			return "valid-token", nil
		},
		SetCookieFunc: func(w http.ResponseWriter, token string, path string) {
			cookieSet = true
			// Verify parameters
			if token != newToken {
				t.Errorf("Expected token '%s', got '%s'", newToken, token)
			}
			if path != testAppPath2 {
				t.Errorf("Expected path %s, got '%s'", testAppPath2, path)
			}
		},
	}

	jwtHandler := &MockJWTHandler{
		ValidateTokenFunc: func(tokenString string) (*Claims, error) {
			tokenValidated = true
			return claims, nil
		},
		ShouldRefreshTokenFunc: func(claims *Claims) bool {
			tokenRefreshChecked = true
			// Do refresh
			return true
		},
		RefreshTokenFunc: func(claims *Claims) (string, error) {
			tokenRefreshed = true
			// Verify claims
			if claims.User != "testuser" {
				t.Errorf("Expected user 'testuser', got '%s'", claims.User)
			}
			if claims.Path != testAppPath2 {
				t.Errorf("Expected path '%s', got '%s'", testAppPath2, claims.Path)
			}
			return newToken, nil
		},
	}

	// Create server with mocks
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &Config{
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
	}
	server := &Server{
		config:        cfg,
		logger:        logger,
		cookieManager: cookieHandler,
		jwtManager:    jwtHandler,
	}

	// Call handler
	server.handleVerify(w, req)

	// Check response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Verify method calls
	if !tokenValidated {
		t.Error("Token was not validated")
	}
	if !tokenRefreshChecked {
		t.Error("ShouldRefreshToken was not called")
	}
	if !tokenRefreshed {
		t.Error("RefreshToken was not called")
	}
	if !cookieSet {
		t.Error("SetCookie was not called")
	}
}
