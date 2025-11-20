/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/rest"
)

// testAppPath is already defined in server_test.go

// TestHandleAuth_PostMethodAllowed tests that POST methods are allowed
func TestHandleAuth_PostMethodAllowed(t *testing.T) {
	// Create a minimal server for testing
	server := createTestServer(nil)

	// Create POST request
	req := httptest.NewRequest(http.MethodPost, "/auth", nil)
	w := httptest.NewRecorder()

	// Call handler
	server.handleAuth(w, req)

	// Check response
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

// createTestServer creates a minimal server for testing
// The mockRestClient parameter is currently unused, but kept for future expansion
// when we need to test with a configured REST client
func createTestServer(_ rest.Interface) *Server {
	config := &Config{
		PathRegexPattern:            DefaultPathRegexPattern,
		WorkspaceNamespacePathRegex: DefaultWorkspaceNamespacePathRegex,
		WorkspaceNamePathRegex:      DefaultWorkspaceNamePathRegex,
		RoutingMode:                 DefaultRoutingMode,
		OidcUsernamePrefix:          DefaultOidcUsernamePrefix,
		OidcGroupsPrefix:            DefaultOidcGroupsPrefix,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	server := &Server{
		config: config,
		logger: logger,
	}

	return server
}

// TestHandleAuth_RequiresHeaders tests that all required headers are properly checked
func TestHandleAuth_RequiresHeaders(t *testing.T) {
	testCases := []struct {
		name           string
		setupRequest   func(*http.Request)
		setupServer    func(*Server)
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Missing Authorization header",
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-Auth-Request-Groups", "org1:group1,org1:group2")
				req.Header.Set("X-Forwarded-Uri", "/workspaces/ns1/app1/lab")
				req.Header.Set("X-Forwarded-Host", "example.com")
				// No Authorization header
			},
			setupServer: func(server *Server) {
				// OIDC is required but verifier is set up
				server.oidcVerifier = &MockOIDCVerifier{}
			},
			expectedStatus: http.StatusUnauthorized,
			expectedError:  "Missing Authorization header",
		},
		{
			name: "Missing URI header",
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-Forwarded-Host", "example.com")
				req.Header.Set("Authorization", "Bearer mock-token")
			},
			setupServer: func(server *Server) {
				// The claims.Username must match X-Auth-Request-User
				setupOIDCVerifier(server, &OIDCClaims{
					Subject:  "user-uid",
					Username: "user-uid",
					Groups:   []string{"org2:group1", "org2:group2"},
				})
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Missing X-Forwarded-Uri header",
		},
		{
			name: "Missing host header",
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-Auth-Request-User", "user-uid")
				req.Header.Set("X-Auth-Request-Groups", "org3:group1,org3:group2")
				req.Header.Set("X-Forwarded-Uri", "/workspaces/ns1/app1/lab")
				req.Header.Set("Authorization", "Bearer mock-token")
			},
			setupServer: func(server *Server) {
				// The claims.Username must match X-Auth-Request-User
				setupOIDCVerifier(server, &OIDCClaims{
					Subject:  "user-uid",
					Username: "user-uid",
					Groups:   []string{"org3:group1", "org3:group2"},
				})
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Missing X-Forwarded-Host header",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test server
			server := createTestServer(nil)

			// Set up the server if needed
			if tc.setupServer != nil {
				tc.setupServer(server)
			}

			// Create and setup the request
			req := httptest.NewRequest(http.MethodGet, "/auth", nil)
			tc.setupRequest(req)
			w := httptest.NewRecorder()

			// Call handler
			server.handleAuth(w, req)

			// Check response status code
			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, w.Code)
			}

			// Check error message
			body := w.Body.String()
			if !strings.Contains(body, tc.expectedError) {
				t.Errorf("Expected error message '%s', got: %s", tc.expectedError, body)
			}
		})
	}
}

// For this test file, we are using mock implementations of the JWTHandler and CookieHandler interfaces
// defined in mock_test.go within the same package.

// setupOIDCVerifier configures a mock OIDC verifier for testing
func setupOIDCVerifier(server *Server, claims *OIDCClaims) {
	// Default claims if none provided
	if claims == nil {
		claims = &OIDCClaims{
			Subject:  "user-uid",   // Match X-Auth-Request-User
			Username: "valid-user", // Match X-Auth-Request-Preferred-Username
			Groups:   []string{"org1:team1", "org1:team2"},
		}
	}

	server.oidcVerifier = &MockOIDCVerifier{
		VerifyTokenFunc: func(ctx context.Context, tokenString string, logger *slog.Logger) (*OIDCClaims, bool, error) {
			return claims, false, nil
		},
	}
}

// TestHandleAuth_HappyPath tests that auth route submits an access review,
// generate a new JWT, set the cookie and returns a 2xx.
func TestHandleAuth_HappyPath(t *testing.T) {
	// Create a server instance
	server := createTestServer(nil)
	// Ensure proper workspace namespace and name regex patterns are configured
	server.config.WorkspaceNamespacePathRegex = DefaultWorkspaceNamespacePathRegex
	server.config.WorkspaceNamePathRegex = DefaultWorkspaceNamePathRegex

	// Set up the OIDC verifier with claims that match what will be compared with headers
	// after the prefixes are applied by GetOidcUsername and GetOidcGroups
	server.oidcVerifier = &MockOIDCVerifier{
		VerifyTokenFunc: func(ctx context.Context, tokenString string, logger *slog.Logger) (*OIDCClaims, bool, error) {
			// Return claims WITHOUT prefixes - the code will add prefixes when comparing with headers
			return &OIDCClaims{
				Subject:  "user-uid",                           // Matches X-Auth-Request-User directly (no prefix for UIDs)
				Username: "valid-user",                         // Without prefix - code will add prefix when comparing with headers
				Groups:   []string{"org1:team1", "org1:team2"}, // Without prefix - code will add prefix
			}, false, nil
		},
	}

	// Create a request - Note: X-Auth-Request-User must match claims UID
	req := httptest.NewRequest(http.MethodGet, "/auth?some-key=some-value", nil)
	// X-Auth-Request-User matches Subject exactly (no prefix for UID)
	req.Header.Set("X-Auth-Request-User", "user-uid")
	// Don't include prefix in the header - the server will add the prefix when comparing with claims
	req.Header.Set("X-Auth-Request-Preferred-Username", "valid-user") // Without prefix
	req.Header.Set("X-Auth-Request-Groups", "org1:team1,org1:team2")  // Without prefix
	// Use testAppPath directly to match exactly what's expected in the test
	req.Header.Set("X-Forwarded-Uri", testAppPath)
	req.Header.Set("X-Forwarded-Host", "example.com")
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("Authorization", "Bearer mock-token") // Add mock bearer token
	w := httptest.NewRecorder()

	// Create mocks
	generatedToken := "generated-jwt-token"
	tokenGenerated := false
	cookieSet := false

	// expects
	// Include prefix as it's applied by GetOidcUsername and GetOidcGroups
	// The prefix is in the server config and is DefaultOidcUsernamePrefix = "github:"
	expectUsername := "github:valid-user"                              // With prefix
	expectGroups := []string{"github:org1:team1", "github:org1:team2"} // With prefix
	expectedUID := "user-uid"                                          // No prefix

	// Create JWT handler mock
	jwtHandler := &MockJWTHandler{
		GenerateTokenFunc: func(
			user string,
			groups []string,
			uid string,
			extra map[string][]string,
			path string,
			domain string,
			tokenType string) (string, error) {
			tokenGenerated = true
			// Verify parameters
			if user != expectUsername {
				t.Errorf("Expected user '%s', got '%s'", expectUsername, user)
			}
			if !reflect.DeepEqual(groups, expectGroups) {
				t.Errorf("Expected groups %v, got %v", expectGroups, groups)
			}
			if uid != expectedUID {
				t.Errorf("Expected uid '%s', got '%s", expectedUID, uid)
			}
			if path != testAppPath {
				t.Errorf("Expected path '%s', got '%s'", testAppPath, path)
			}
			if domain != "example.com" {
				t.Errorf("Expected domain 'example.com', got '%s'", domain)
			}
			return generatedToken, nil
		},
	}

	// Create cookie handler mock
	cookieHandler := &MockCookieHandler{
		SetCookieFunc: func(w http.ResponseWriter, token string, path string, domain string) {
			cookieSet = true
			// Verify parameters
			if token != generatedToken {
				t.Errorf("Expected token '%s', got '%s'", generatedToken, token)
			}
			if path != testAppPath {
				t.Errorf("Expected path '%s', got '%s'", testAppPath, path)
			}
		},
	}

	// Configure server
	server.config.PathRegexPattern = `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`
	server.config.WorkspaceNamespacePathRegex = DefaultWorkspaceNamespacePathRegex
	server.config.WorkspaceNamePathRegex = DefaultWorkspaceNamePathRegex
	server.jwtManager = jwtHandler
	server.cookieManager = cookieHandler

	// Create a mock K8s server for testing
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	// mock response
	reason := "User authorized by RBAC and workspace is public"
	mockedResponse := CreateConnectionAccessReviewResponse(
		"ns1",
		"app1",
		expectUsername,
		expectGroups,
		expectedUID,
		true,  // allowed
		false, // not found
		reason,
	)

	// Set up the mock server to return a 200 OK response
	mockServer.SetupServer200OK(mockedResponse)

	// Create a REST client pointing to our test server
	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	// Set the REST client
	server.restClient = restClient

	// Call handler
	server.handleAuth(w, req)

	// Check response status
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// Verify that methods were called
	if !tokenGenerated {
		t.Error("JWT token was not generated")
	}
	if !cookieSet {
		t.Error("Cookie was not set")
	}

	// Check JSON response
	var response map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Errorf("Failed to parse JSON response: %v", err)
	}

	// Verify the request was made to the correct URL
	expectedPath := fmt.Sprintf("/apis/%s/namespaces/ns1/connectionaccessreview",
		expectApiGroupVersion)
	mockServer.AssertRequestPath(expectedPath)
}

func TestHandleAuth_Returns5xxWhenK8sClientNotSet(t *testing.T) {
	// Create a standard server
	server := createTestServer(nil)
	server.jwtManager = &MockJWTHandler{}
	server.cookieManager = &MockCookieHandler{}
	server.config.WorkspaceNamespacePathRegex = DefaultWorkspaceNamespacePathRegex
	server.config.WorkspaceNamePathRegex = DefaultWorkspaceNamePathRegex

	// Setup OIDC verifier with claims matching the request headers
	setupOIDCVerifier(server, &OIDCClaims{
		Subject:  "user-uid",
		Username: "user1",
		Groups:   []string{"org1:group1", "org1:group2"},
	})

	// Ensure restClient is nil
	server.restClient = nil

	// Create a request with all necessary headers
	req := httptest.NewRequest(http.MethodGet, "/auth", nil)
	// Match the UID exactly (Subject field in claims)
	req.Header.Set("X-Auth-Request-User", "user-uid")
	req.Header.Set("X-Auth-Request-Preferred-Username", "user1")
	req.Header.Set("X-Auth-Request-Groups", "org1:group1,org1:group2")
	req.Header.Set("X-Forwarded-Uri", testAppPath)
	req.Header.Set("X-Forwarded-Host", "example.com")
	req.Header.Set("Authorization", "Bearer mock-token")
	w := httptest.NewRecorder()

	// Call handler
	server.handleAuth(w, req)

	// Check for 500 status code
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !strings.Contains(body, "Internal server error") {
		t.Errorf("Expected 'Internal server error' message, got: %s", body)
	}
}

func TestHandleAuth_Returns403_WhenTokenIsInvalid(t *testing.T) {
	// Create a standard server
	server := createTestServer(nil)
	server.jwtManager = &MockJWTHandler{}
	server.cookieManager = &MockCookieHandler{}

	// Set up OIDC verifier to return a token error (client error)
	server.oidcVerifier = &MockOIDCVerifier{
		VerifyTokenFunc: func(ctx context.Context, tokenString string, logger *slog.Logger) (*OIDCClaims, bool, error) {
			return nil, false, fmt.Errorf("token validation failed: token has expired")
		},
	}

	// Create a request with all necessary headers
	req := httptest.NewRequest(http.MethodGet, "/auth", nil)
	req.Header.Set("X-Auth-Request-User", "user-uid")
	req.Header.Set("X-Auth-Request-Preferred-Username", "user-uid")
	req.Header.Set("X-Auth-Request-Groups", "group1,group2")
	req.Header.Set("X-Forwarded-Uri", "/workspaces/ns1/app1")
	req.Header.Set("X-Forwarded-Host", "example.com")
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	// Call handler
	server.handleAuth(w, req)

	// Check for 403 Forbidden status code
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !strings.Contains(body, "Invalid or expired OIDC token") {
		t.Errorf("Expected error message about token validation, got: %s", body)
	}
}

func TestHandleAuth_Returns5xx_WhenVerifyTokenFails(t *testing.T) {
	// Create a standard server
	server := createTestServer(nil)
	server.jwtManager = &MockJWTHandler{}
	server.cookieManager = &MockCookieHandler{}
	server.config.WorkspaceNamespacePathRegex = DefaultWorkspaceNamespacePathRegex
	server.config.WorkspaceNamePathRegex = DefaultWorkspaceNamePathRegex

	// Set up OIDC verifier to return a setup error (server error)
	server.oidcVerifier = &MockOIDCVerifier{
		VerifyTokenFunc: func(ctx context.Context, tokenString string, logger *slog.Logger) (*OIDCClaims, bool, error) {
			return nil, true, fmt.Errorf("failed to connect to OIDC provider: context deadline exceeded")
		},
	}

	// Create a request with all necessary headers
	req := httptest.NewRequest(http.MethodGet, "/auth", nil)
	req.Header.Set("X-Auth-Request-User", "user-uid")
	req.Header.Set("X-Auth-Request-Preferred-Username", "user-uid")
	req.Header.Set("X-Auth-Request-Groups", "group1,group2")
	req.Header.Set("X-Forwarded-Uri", "/workspaces/ns1/app1")
	req.Header.Set("X-Forwarded-Host", "example.com")
	req.Header.Set("Authorization", "Bearer some-token")
	w := httptest.NewRecorder()

	// Call handler
	server.handleAuth(w, req)

	// Check for 500 Internal Server Error status code
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !strings.Contains(body, "OIDC provider not available") {
		t.Errorf("Expected error message about OIDC provider connection, got: %s", body)
	}
}

func TestHandleAuth_Returns4xx_WhenAnyAuthHeaderAndVerifyDoesNotMatch(t *testing.T) {
	// Create a standard server
	server := createTestServer(nil)
	server.jwtManager = &MockJWTHandler{}
	server.cookieManager = &MockCookieHandler{}
	server.config.WorkspaceNamespacePathRegex = DefaultWorkspaceNamespacePathRegex
	server.config.WorkspaceNamePathRegex = DefaultWorkspaceNamePathRegex

	// Set up OIDC verifier to return claims that don't match the headers
	// In this case, the username in the claims is different from the X-Auth-Request-User header
	server.oidcVerifier = &MockOIDCVerifier{
		VerifyTokenFunc: func(ctx context.Context, tokenString string, logger *slog.Logger) (*OIDCClaims, bool, error) {
			claims := &OIDCClaims{
				Subject:  "different-user-uid", // Different subject
				Username: "different-user",     // Different username
				Groups:   []string{"org:group1", "org:group2"},
			}
			return claims, false, nil
		},
	}

	// Create a request with headers that won't match the OIDC claims
	req := httptest.NewRequest(http.MethodGet, "/auth", nil)
	req.Header.Set("X-Auth-Request-User", "user-uid")               // Doesn't match claims
	req.Header.Set("X-Auth-Request-Preferred-Username", "user-uid") // Doesn't match claims
	req.Header.Set("X-Auth-Request-Groups", "group1,group2")
	req.Header.Set("X-Forwarded-Uri", "/workspaces/ns1/app1")
	req.Header.Set("X-Forwarded-Host", "example.com")
	req.Header.Set("Authorization", "Bearer valid-token-wrong-user")
	w := httptest.NewRecorder()

	// Call handler
	server.handleAuth(w, req)

	// Check for 401 Unauthorized status code
	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !strings.Contains(body, "Username mismatch") {
		t.Errorf("Expected error message about username mismatch, got: %s", body)
	}
}

func TestHandleAuth_Returns5xx_WhenVerifyAccessWorkspaceReturnsError(t *testing.T) {
	// Create a standard server
	server := createTestServer(nil)
	server.jwtManager = &MockJWTHandler{}
	server.cookieManager = &MockCookieHandler{}
	server.config.WorkspaceNamespacePathRegex = DefaultWorkspaceNamespacePathRegex
	server.config.WorkspaceNamePathRegex = DefaultWorkspaceNamePathRegex

	// Setup OIDC verifier with claims matching the request headers
	setupOIDCVerifier(server, &OIDCClaims{
		Subject:  "user-uid",
		Username: "user2",
		Groups:   []string{"org5:group1", "org5:group2"},
	})

	// Create a mock K8s server for testing
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	// Set up the mock server to return a 500 Internal Server Error response
	mockServer.SetupServer500InternalServerError()

	// Create a REST client pointing to our test server
	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	// Set the REST client
	server.restClient = restClient

	// Create a request with all necessary headers
	req := httptest.NewRequest(http.MethodGet, "/auth", nil)
	req.Header.Set("X-Auth-Request-User", "user-uid") // Must match Subject in OIDC claims
	req.Header.Set("X-Auth-Request-Preferred-Username", "user2")
	req.Header.Set("X-Auth-Request-Groups", "org5:group1,org5:group2")
	req.Header.Set("X-Forwarded-Uri", testAppPath)
	req.Header.Set("X-Forwarded-Host", "example.com")
	req.Header.Set("Authorization", "Bearer mock-token")
	w := httptest.NewRecorder()

	// Call handler
	server.handleAuth(w, req)

	// Check for 500 status code
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !strings.Contains(body, "Failed to verify workspace access") {
		t.Errorf("Expected error message about failed validation, got: %s", body)
	}
}

func TestHandleAuth_Returns403_WhenVerifyAccessWorkspaceReturnsDisallowed(t *testing.T) {
	// Create a standard server
	server := createTestServer(nil)
	server.jwtManager = &MockJWTHandler{}
	server.cookieManager = &MockCookieHandler{}
	server.config.WorkspaceNamespacePathRegex = DefaultWorkspaceNamespacePathRegex
	server.config.WorkspaceNamePathRegex = DefaultWorkspaceNamePathRegex

	// Setup OIDC verifier with claims matching the request headers
	setupOIDCVerifier(server, &OIDCClaims{
		Subject:  "user-uid",
		Username: "user1",
		Groups:   []string{"org6:group1", "org6:group2"},
	})

	// Create a mock K8s server for testing
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	reason := "User not authorized by RBAC"
	mockedResponse := CreateConnectionAccessReviewResponse(
		"ns1",
		"app1",
		"user1",
		[]string{"org6:group1", "org6:group2"},
		"user-uid",
		false, // allowed
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

	// Create a request with all necessary headers
	req := httptest.NewRequest(http.MethodGet, "/auth", nil)
	req.Header.Set("X-Auth-Request-User", "user-uid") // Must match Subject in OIDC claims
	req.Header.Set("X-Auth-Request-Preferred-Username", "user1")
	req.Header.Set("X-Auth-Request-Groups", "org6:group1,org6:group2")
	req.Header.Set("X-Forwarded-Uri", testAppPath)
	req.Header.Set("X-Forwarded-Host", "example.com")
	req.Header.Set("Authorization", "Bearer mock-token")
	w := httptest.NewRecorder()

	// Call handler
	server.handleAuth(w, req)

	// Check for 403 status code
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !strings.Contains(body, "Access denied") {
		t.Errorf("Expected error message about access denied, got: %s", body)
	}
}
