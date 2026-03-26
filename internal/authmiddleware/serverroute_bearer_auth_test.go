/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleBearerAuthMethodNotAllowed(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &Server{logger: logger}

	req := httptest.NewRequest(http.MethodPost, "/bearer-auth", nil)
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestHandleBearerAuthMissingForwardedURI(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &Server{logger: logger}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if w.Body.String() != "Missing "+HeaderForwardedURI+" header\n" {
		t.Errorf("Unexpected body: %s", w.Body.String())
	}
}

func TestHandleBearerAuthInvalidForwardedURI(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &Server{logger: logger}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	req.Header.Set(HeaderForwardedURI, "://invalid-url")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if w.Body.String() != "Invalid forwarded URI\n" {
		t.Errorf("Unexpected body: %s", w.Body.String())
	}
}

func TestHandleBearerAuthMissingToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &Server{logger: logger}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	req.Header.Set(HeaderForwardedURI, "/workspaces/test/workspace")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if w.Body.String() != "Missing token parameter\n" {
		t.Errorf("Unexpected body: %s", w.Body.String())
	}
}

func TestHandleBearerAuthMissingForwardedHost(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &Server{
		logger: logger,
		config: &Config{},
	}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	req.Header.Set(HeaderForwardedURI, "/workspaces/test/workspace?token=valid")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
	if w.Body.String() != "Missing "+HeaderForwardedHost+" header\n" {
		t.Errorf("Unexpected body: %s", w.Body.String())
	}
}

// --- BearerTokenReview tests ---

func TestHandleBearerAuth_BearerTokenReview_ExtractWorkspaceInfoFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &Server{
		logger: logger,
		config: &Config{
			// KMSKeyId is empty → enters BearerTokenReview branch
			PathRegexPattern:            DefaultPathRegexPattern,
			RoutingMode:                 DefaultRoutingMode,
			WorkspaceNamespacePathRegex: `^/nonexistent/([^/]+)/path`,
			WorkspaceNamePathRegex:      `^/nonexistent/[^/]+/path/([^/]+)`,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	req.Header.Set(HeaderForwardedURI, "/workspaces/default/myworkspace/?token=some-token")
	req.Header.Set(HeaderForwardedHost, "example.com")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to extract workspace info")
}

func TestHandleBearerAuth_BearerTokenReview_CreateReviewError(t *testing.T) {
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	mockServer.SetupServer500InternalServerError()

	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &Server{
		logger:     logger,
		restClient: restClient,
		config: &Config{
			PathRegexPattern:            DefaultPathRegexPattern,
			RoutingMode:                 DefaultRoutingMode,
			WorkspaceNamespacePathRegex: DefaultWorkspaceNamespacePathRegex,
			WorkspaceNamePathRegex:      DefaultWorkspaceNamePathRegex,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	req.Header.Set(HeaderForwardedURI, "/workspaces/default/myworkspace/?token=some-token")
	req.Header.Set(HeaderForwardedHost, "example.com")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Token verification failed")
}

func TestHandleBearerAuth_BearerTokenReview_Unauthenticated(t *testing.T) {
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	response := CreateBearerTokenReviewResponse(
		TestDefaultNamespace,
		false, // not authenticated
		"/workspaces/default/myworkspace",
		"", "", nil, nil,
		"token expired",
	)
	mockServer.SetupServerBearerTokenReview200OK(response)

	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &Server{
		logger:     logger,
		restClient: restClient,
		config: &Config{
			PathRegexPattern:            DefaultPathRegexPattern,
			RoutingMode:                 DefaultRoutingMode,
			WorkspaceNamespacePathRegex: DefaultWorkspaceNamespacePathRegex,
			WorkspaceNamePathRegex:      DefaultWorkspaceNamePathRegex,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	req.Header.Set(HeaderForwardedURI, "/workspaces/default/myworkspace/?token=expired-token")
	req.Header.Set(HeaderForwardedHost, "example.com")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid token")
}

func TestHandleBearerAuth_BearerTokenReview_PathMismatch(t *testing.T) {
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	response := CreateBearerTokenReviewResponse(
		TestDefaultNamespace,
		true,
		"/workspaces/default/different-workspace", // path doesn't match request
		testUserValue, testUIDValue, []string{"users"}, nil,
		"",
	)
	mockServer.SetupServerBearerTokenReview200OK(response)

	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := &Server{
		logger:     logger,
		restClient: restClient,
		config: &Config{
			PathRegexPattern:            DefaultPathRegexPattern,
			RoutingMode:                 DefaultRoutingMode,
			WorkspaceNamespacePathRegex: DefaultWorkspaceNamespacePathRegex,
			WorkspaceNamePathRegex:      DefaultWorkspaceNamePathRegex,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	req.Header.Set(HeaderForwardedURI, "/workspaces/default/myworkspace/?token=valid-token")
	req.Header.Set(HeaderForwardedHost, "example.com")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Token path mismatch")
}

func TestHandleBearerAuth_BearerTokenReview_Success(t *testing.T) {
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	response := CreateBearerTokenReviewResponse(
		TestDefaultNamespace,
		true,
		"/workspaces/default/myworkspace", // matches appPath extracted from forwarded URI
		testUserValue, testUIDValue, []string{"users"}, nil,
		"",
	)
	mockServer.SetupServerBearerTokenReview200OK(response)

	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	jwtHandler := &MockJWTHandler{
		GenerateTokenFunc: func(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string) (string, error) {
			assert.Equal(t, testUserValue, user)
			assert.Equal(t, testUIDValue, uid)
			assert.Equal(t, []string{"users"}, groups)
			assert.Equal(t, "/workspaces/default/myworkspace", path)
			assert.Equal(t, "example.com", domain)
			assert.Equal(t, jwt.TokenTypeSession, tokenType)
			return "session-token", nil
		},
	}
	var cookieSet bool
	cookieHandler := &MockCookieHandler{
		SetCookieFunc: func(w http.ResponseWriter, token string, path string, domain string) {
			assert.Equal(t, "session-token", token)
			assert.Equal(t, "/workspaces/default/myworkspace", path)
			assert.Equal(t, "example.com", domain)
			cookieSet = true
		},
	}
	server := &Server{
		logger:        logger,
		restClient:    restClient,
		jwtManager:    jwtHandler,
		cookieManager: cookieHandler,
		config: &Config{
			PathRegexPattern:            DefaultPathRegexPattern,
			RoutingMode:                 DefaultRoutingMode,
			WorkspaceNamespacePathRegex: DefaultWorkspaceNamespacePathRegex,
			WorkspaceNamePathRegex:      DefaultWorkspaceNamePathRegex,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	req.Header.Set(HeaderForwardedURI, "/workspaces/default/myworkspace/?token=valid-token")
	req.Header.Set(HeaderForwardedHost, "example.com")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, cookieSet, "Expected cookie to be set")

	// Verify the request was made to the correct BearerTokenReview endpoint
	expectedPath := "/apis/" + expectApiGroupVersion + "/namespaces/" + TestDefaultNamespace + "/bearertokenreviews"
	mockServer.AssertRequestPath(expectedPath)
	mockServer.AssertRequestMethod("POST")
}

func TestHandleBearerAuth_BearerTokenReview_GenerateTokenError(t *testing.T) {
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	response := CreateBearerTokenReviewResponse(
		TestDefaultNamespace,
		true,
		"/workspaces/default/myworkspace",
		testUserValue, testUIDValue, []string{"users"}, nil,
		"",
	)
	mockServer.SetupServerBearerTokenReview200OK(response)

	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	jwtHandler := &MockJWTHandler{
		GenerateTokenFunc: func(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string) (string, error) {
			return "", assert.AnError
		},
	}
	server := &Server{
		logger:     logger,
		restClient: restClient,
		jwtManager: jwtHandler,
		config: &Config{
			PathRegexPattern:            DefaultPathRegexPattern,
			RoutingMode:                 DefaultRoutingMode,
			WorkspaceNamespacePathRegex: DefaultWorkspaceNamespacePathRegex,
			WorkspaceNamePathRegex:      DefaultWorkspaceNamePathRegex,
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	req.Header.Set(HeaderForwardedURI, "/workspaces/default/myworkspace/?token=valid-token")
	req.Header.Set(HeaderForwardedHost, "example.com")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Internal server error")
}
