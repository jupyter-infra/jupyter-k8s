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

	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	"github.com/stretchr/testify/require"
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
		PathRegexPattern:            `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
		RoutingMode:                 "path",
		WorkspaceNamespacePathRegex: `^/workspaces/([^/]+)/[^/]+`,
		WorkspaceNamePathRegex:      `^/workspaces/[^/]+/([^/]+)`,
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
		PathRegexPattern:            `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
		RoutingMode:                 "path",
		WorkspaceNamespacePathRegex: `^/workspaces/([^/]+)/[^/]+`,
		WorkspaceNamePathRegex:      `^/workspaces/[^/]+/([^/]+)`,
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
		PathRegexPattern:            `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
		RoutingMode:                 "path",
		WorkspaceNamespacePathRegex: `^/workspaces/([^/]+)/[^/]+`,
		WorkspaceNamePathRegex:      `^/workspaces/[^/]+/([^/]+)`,
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

// For this test file, we are using mock implementations of the jwt.Handler and CookieHandler interfaces
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
		ValidateTokenFunc: func(tokenString string) (*jwt.Claims, error) {
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
		PathRegexPattern:            `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
		RoutingMode:                 "path",
		WorkspaceNamespacePathRegex: `^/workspaces/([^/]+)/[^/]+`,
		WorkspaceNamePathRegex:      `^/workspaces/[^/]+/([^/]+)`,
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
	claims := &jwt.Claims{
		User:      "testuser",
		Groups:    []string{"group1", "group2"},
		UID:       "testuid",
		Path:      testAppPath2,
		Domain:    "example.com",
		TokenType: jwt.TokenTypeSession, // Add session token type
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
		SetCookieFunc: func(w http.ResponseWriter, token string, path string, domain string) {
			t.Error("SetCookie should not have been called")
		},
		ClearCookieFunc: func(w http.ResponseWriter, path string, domain string) {
			t.Error("ClearCookie should not have been called")
		},
	}

	jwtHandler := &MockJWTHandler{
		ValidateTokenFunc: func(tokenString string) (*jwt.Claims, error) {
			tokenValidated = true
			return claims, nil
		},
		ShouldRefreshTokenFunc: func(claims *jwt.Claims) bool {
			tokenRefreshChecked = true
			// Don't refresh
			return false
		},
		RefreshTokenFunc: func(claims *jwt.Claims) (string, error) {
			tokenRefreshed = true
			return "new-token", nil
		},
	}

	// Create server with mocks
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &Config{
		PathRegexPattern:            `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
		RoutingMode:                 "path",
		WorkspaceNamespacePathRegex: `^/workspaces/([^/]+)/[^/]+`,
		WorkspaceNamePathRegex:      `^/workspaces/[^/]+/([^/]+)`,
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

// createVerifyRefreshTestServer creates a minimal server for testing
// The mockRestClient parameter is currently unused, but kept for future expansion
// when we need to test with a configured REST client
func createVerifyRefreshTestServer(cookieHandler CookieHandler, jwtHandler jwt.Handler) *Server {
	config := &Config{
		PathRegexPattern:            DefaultPathRegexPattern,
		WorkspaceNamespacePathRegex: DefaultWorkspaceNamespacePathRegex,
		WorkspaceNamePathRegex:      DefaultWorkspaceNamePathRegex,
		RoutingMode:                 DefaultRoutingMode,
		JWTRefreshEnable:            true,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	server := &Server{
		config:        config,
		logger:        logger,
		cookieManager: cookieHandler,
		jwtManager:    jwtHandler,
	}

	return server
}

// TestHandleVerifyWithRefresh_HappyPath tests that handleVerify refreshes the JWT during the RefreshWindow,
// returns a 200 and refreshes the JWT/cookiewhen the ConnectionAccessReview returns allowed=true.
func TestHandleVerifyWithRefresh_HappyPath(t *testing.T) {
	fwdUrl := fmt.Sprintf("%s/lab", testAppPath2)

	// Create a request with required headers
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	req.Header.Set("X-Forwarded-Uri", fwdUrl)
	req.Header.Set("X-Forwarded-Host", "example.com")
	w := httptest.NewRecorder()

	// Create claims
	claims := &jwt.Claims{
		User:      "testuser1",
		Groups:    []string{"group1", "group2"},
		UID:       "testuid",
		Path:      testAppPath2,
		Domain:    "example.com",
		TokenType: jwt.TokenTypeSession, // Add session token type
	}

	// Track method calls
	tokenValidated := false
	tokenRefreshChecked := false
	tokenRefreshed := false
	tokenSkipUpdated := false
	cookieSet := false
	newToken := "refreshed-token1"

	// Create mock handlers
	cookieHandler := &MockCookieHandler{
		GetCookieFunc: func(r *http.Request, path string) (string, error) {
			return "valid-token", nil
		},
		SetCookieFunc: func(w http.ResponseWriter, token string, path string, domain string) {
			cookieSet = true
			// Verify parameters
			if token != newToken {
				t.Errorf("Expected token '%s', got '%s'", newToken, token)
			}
			if path != testAppPath2 {
				t.Errorf("Expected path %s, got '%s'", testAppPath2, path)
			}
		},
		ClearCookieFunc: func(w http.ResponseWriter, path string, domain string) {
			t.Error("ClearCookie should not have been called")
		},
	}

	jwtHandler := &MockJWTHandler{
		ValidateTokenFunc: func(tokenString string) (*jwt.Claims, error) {
			tokenValidated = true
			return claims, nil
		},
		ShouldRefreshTokenFunc: func(claims *jwt.Claims) bool {
			tokenRefreshChecked = true
			// Do refresh
			return true
		},
		RefreshTokenFunc: func(claims *jwt.Claims) (string, error) {
			tokenRefreshed = true
			// Verify claims
			if claims.User != "testuser1" {
				t.Errorf("Expected user 'testuser1', got '%s'", claims.User)
			}
			if claims.Path != testAppPath2 {
				t.Errorf("Expected path '%s', got '%s'", testAppPath2, claims.Path)
			}
			return newToken, nil
		},
		UpdateSkipRefreshTokenFunc: func(claims *jwt.Claims) (string, error) {
			tokenSkipUpdated = true
			return newToken, nil
		},
	}

	// Create server with mocks
	server := createVerifyRefreshTestServer(cookieHandler, jwtHandler)

	// Create a mock K8s server for testing
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	reason := "User is authorized by RBAC and is the owner of private workspace"
	mockedResponse := CreateConnectionAccessReviewResponse(
		"ns2",
		"app2",
		claims.User,
		claims.Groups,
		claims.UID,
		true,  // allowed
		false, // not found
		reason,
	)

	// Set up the mock server to return a 200 with allowed response
	mockServer.SetupServer200OK(mockedResponse)

	// Create a REST client pointing to our test server
	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	// Set the REST client
	server.restClient = restClient

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
	if tokenSkipUpdated {
		t.Error("UpdateSkipRefreshToken should not have been called")
	}
	if !cookieSet {
		t.Error("SetCookie was not called")
	}
}

// TestHandleVerifyWithRefresh_NoLongerAuthorizedPath_Returns403AndClearCookie tests that
// handleVerify refreshes the JWT during the RefreshWindow, returns a 403 and clear the cookie
// when the ConnectionAccessReview returns allowed=false.
func TestHandleVerifyWithRefresh_NoLongerAuthorizedPath_Returns403AndClearCookie(t *testing.T) {
	fwdUrl := fmt.Sprintf("%s/lab", testAppPath2)

	// Create a request with required headers
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	req.Header.Set("X-Forwarded-Uri", fwdUrl)
	req.Header.Set("X-Forwarded-Host", "example.com")
	w := httptest.NewRecorder()

	// Create claims
	claims := &jwt.Claims{
		User:      "testuser2",
		Groups:    []string{"group1", "group2"},
		UID:       "testuid",
		Path:      testAppPath2,
		Domain:    "example.com",
		TokenType: jwt.TokenTypeSession, // Add session token type
	}

	// Track method calls
	tokenValidated := false
	tokenRefreshChecked := false
	tokenRefreshed := false
	tokenSkipUpdated := false
	cookieSet := false
	cookieCleared := false
	newToken := "refreshed-token2"

	// Create mock handlers
	cookieHandler := &MockCookieHandler{
		GetCookieFunc: func(r *http.Request, path string) (string, error) {
			return "valid-token2", nil
		},
		SetCookieFunc: func(w http.ResponseWriter, token string, path string, domain string) {
			cookieSet = true
			// Verify parameters
			if token != newToken {
				t.Errorf("Expected token '%s', got '%s'", newToken, token)
			}
			if path != testAppPath2 {
				t.Errorf("Expected path %s, got '%s'", testAppPath2, path)
			}
		},
		ClearCookieFunc: func(w http.ResponseWriter, path string, domain string) {
			cookieCleared = true
			// Verify parameters
			if path != testAppPath2 {
				t.Errorf("Expected path %s, got '%s'", testAppPath2, path)
			}
		},
	}

	jwtHandler := &MockJWTHandler{
		ValidateTokenFunc: func(tokenString string) (*jwt.Claims, error) {
			tokenValidated = true
			return claims, nil
		},
		ShouldRefreshTokenFunc: func(claims *jwt.Claims) bool {
			tokenRefreshChecked = true
			// Do refresh
			return true
		},
		RefreshTokenFunc: func(claims *jwt.Claims) (string, error) {
			tokenRefreshed = true
			// Verify claims
			if claims.User != "testuser" {
				t.Errorf("Expected user 'testuser2', got '%s'", claims.User)
			}
			if claims.Path != testAppPath2 {
				t.Errorf("Expected path '%s', got '%s'", testAppPath2, claims.Path)
			}
			return newToken, nil
		},
		UpdateSkipRefreshTokenFunc: func(claims *jwt.Claims) (string, error) {
			tokenSkipUpdated = true
			return newToken, nil
		},
	}

	// Create server with mocks
	server := createVerifyRefreshTestServer(cookieHandler, jwtHandler)

	// Create a mock K8s server for testing
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	reason := "User is not authorized by RBAC"
	mockedResponse := CreateConnectionAccessReviewResponse(
		"ns2",
		"app2",
		claims.User,
		claims.Groups,
		claims.UID,
		false, // no longer allowed
		false, // not found
		reason,
	)

	// Set up the mock server to return a 200 with disallowed response
	mockServer.SetupServer200OK(mockedResponse)

	// Create a REST client pointing to our test server
	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	// Set the REST client
	server.restClient = restClient

	// Call handler
	server.handleVerify(w, req)

	// Check response
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	// Verify method calls
	if !tokenValidated {
		t.Error("Token was not validated")
	}
	if !tokenRefreshChecked {
		t.Error("ShouldRefreshToken was not called")
	}
	if tokenSkipUpdated {
		t.Error("UpdateSkipRefreshToken should not have been called")
	}
	if tokenRefreshed {
		t.Error("RefreshToken should not have been called")
	}
	if cookieSet {
		t.Error("SetCookie should not have been called")
	}
	if !cookieCleared {
		t.Error("ClearCookie was not called")
	}
}

// TestHandleVerifyWithRefresh_ConnectionAccessReviewFails_UpdateCookieToSkipFutureRefreshes tests
// that handleVerify returns a 200 and update the JWT/cookie with a skip
// flag that prevents future refreshes when the ConnectionAccessReview fails.
func TestHandleVerifyWithRefresh_ConnectionAccessReviewFails_UpdateCookieToSkipFutureRefreshes(t *testing.T) {
	fwdUrl := fmt.Sprintf("%s/lab", testAppPath2)

	// Create a request with required headers
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	req.Header.Set("X-Forwarded-Uri", fwdUrl)
	req.Header.Set("X-Forwarded-Host", "example.com")
	w := httptest.NewRecorder()

	// Create claims
	claims := &jwt.Claims{
		User:      "testuser3",
		Groups:    []string{"group1", "group2"},
		UID:       "testuid",
		Path:      testAppPath2,
		Domain:    "example.com",
		TokenType: jwt.TokenTypeSession, // Add session token type
	}

	// Track method calls
	tokenValidated := false
	tokenRefreshChecked := false
	tokenRefreshed := false
	tokenSkipUpdated := false
	cookieSet := false
	newToken := "refreshed-token3"

	// Create mock handlers
	cookieHandler := &MockCookieHandler{
		GetCookieFunc: func(r *http.Request, path string) (string, error) {
			return "valid-token3", nil
		},
		SetCookieFunc: func(w http.ResponseWriter, token string, path string, domain string) {
			cookieSet = true
			// Verify parameters
			if token != newToken {
				t.Errorf("Expected token '%s', got '%s'", newToken, token)
			}
			if path != testAppPath2 {
				t.Errorf("Expected path %s, got '%s'", testAppPath2, path)
			}
		},
		ClearCookieFunc: func(w http.ResponseWriter, path string, domain string) {
			t.Error("ClearCookie should not have been called")
		},
	}

	jwtHandler := &MockJWTHandler{
		ValidateTokenFunc: func(tokenString string) (*jwt.Claims, error) {
			tokenValidated = true
			return claims, nil
		},
		ShouldRefreshTokenFunc: func(claims *jwt.Claims) bool {
			tokenRefreshChecked = true
			// Do refresh
			return true
		},
		RefreshTokenFunc: func(claims *jwt.Claims) (string, error) {
			tokenRefreshed = true
			// Verify claims
			if claims.User != "testuser3" {
				t.Errorf("Expected user 'testuser3', got '%s'", claims.User)
			}
			if claims.Path != testAppPath2 {
				t.Errorf("Expected path '%s', got '%s'", testAppPath2, claims.Path)
			}
			return newToken, nil
		},
		UpdateSkipRefreshTokenFunc: func(claims *jwt.Claims) (string, error) {
			tokenSkipUpdated = true
			return newToken, nil
		},
	}

	// Create server with mocks
	server := createVerifyRefreshTestServer(cookieHandler, jwtHandler)

	// Create a mock K8s server for testing
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	// Set up the mock server to return a 500 with disallowed response
	mockServer.SetupServer500InternalServerError()

	// Create a REST client pointing to our test server
	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	// Set the REST client
	server.restClient = restClient

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
	if !tokenSkipUpdated {
		t.Error("UpdateSkipRefreshToken was not called")
	}
	if !cookieSet {
		t.Error("SetCookie was not called")
	}
}
