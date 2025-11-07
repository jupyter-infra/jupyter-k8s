package authmiddleware

import (
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
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Missing user header",
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-Auth-Request-Groups", "org1:group1,org1:group2")
				req.Header.Set("X-Forwarded-Uri", "/workspaces/ns1/app1/lab")
				req.Header.Set("X-Forwarded-Host", "example.com")
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Missing X-Auth-Request-User header",
		},
		{
			name: "Missing URI header",
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-Auth-Request-User", "user-uid")
				req.Header.Set("X-Auth-Request-Groups", "org2:group1,org2:group2")
				req.Header.Set("X-Forwarded-Host", "example.com")
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
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Missing X-Forwarded-Host header",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test server
			server := createTestServer(nil)

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

// TestHandleAuth_HappyPath tests that auth route submits an access review,
// generate a new JWT, set the cookie and returns a 2xx.
func TestHandleAuth_HappyPath(t *testing.T) {
	// Create a server instance
	server := createTestServer(nil)

	// Create a request
	req := httptest.NewRequest(http.MethodGet, "/auth?some-key=some-value", nil)
	req.Header.Set("X-Auth-Request-User", "user-uid")
	req.Header.Set("X-Auth-Request-Preferred-Username", "valid-user")
	req.Header.Set("X-Auth-Request-Groups", "org1:team1,org1:team2")
	req.Header.Set("X-Forwarded-Uri", "/workspaces/ns1/app1/notebooks/nb1.ipynb")
	req.Header.Set("X-Forwarded-Host", "example.com")
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()

	// Create mocks
	generatedToken := "generated-jwt-token"
	tokenGenerated := false
	cookieSet := false

	// expects
	expectUsername := "github:valid-user"
	expectGroups := []string{"github:org1:team1", "github:org1:team2"}
	expectedUID := "user-uid"

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

	// Ensure restClient is nil
	server.restClient = nil

	// Create a request with all necessary headers
	req := httptest.NewRequest(http.MethodGet, "/auth", nil)
	req.Header.Set("X-Auth-Request-User", "user-uid")
	req.Header.Set("X-Auth-Request-Preferred-Username", "user1")
	req.Header.Set("X-Auth-Request-Groups", "github:org1:group1,org1:group2")
	req.Header.Set("X-Forwarded-Uri", testAppPath)
	req.Header.Set("X-Forwarded-Host", "example.com")
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

func TestHandleAuth_Returns5xx_WhenVerifyAccessWorkspaceReturnsError(t *testing.T) {
	// Create a standard server
	server := createTestServer(nil)
	server.jwtManager = &MockJWTHandler{}
	server.cookieManager = &MockCookieHandler{}
	server.config.WorkspaceNamespacePathRegex = DefaultWorkspaceNamespacePathRegex
	server.config.WorkspaceNamePathRegex = DefaultWorkspaceNamePathRegex

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
	req.Header.Set("X-Auth-Request-User", "user-uid")
	req.Header.Set("X-Auth-Request-Preferred-Username", "user2")
	req.Header.Set("X-Auth-Request-Groups", "org5:group1,org5:group2")
	req.Header.Set("X-Forwarded-Uri", testAppPath)
	req.Header.Set("X-Forwarded-Host", "example.com")
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

	// Create a mock K8s server for testing
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	reason := "User not authorized by RBAC"
	mockedResponse := CreateConnectionAccessReviewResponse(
		"ns1",
		"app1",
		"github:user1",
		[]string{"github:org6:group1", "github:org6:group2"},
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
	req.Header.Set("X-Auth-Request-User", "user-uid")
	req.Header.Set("X-Auth-Request-Preferred-Username", "user1")
	req.Header.Set("X-Auth-Request-Groups", "org6:group1,org6:group2")
	req.Header.Set("X-Forwarded-Uri", testAppPath)
	req.Header.Set("X-Forwarded-Host", "example.com")
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
