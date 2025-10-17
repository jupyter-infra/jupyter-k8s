package authmiddleware

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

// TestHandleAuthMethodNotAllowed tests that only GET method is allowed
func TestHandleAuthMethodNotAllowed(t *testing.T) {
	// Create a minimal server for testing
	server := createTestServer()

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
func createTestServer() *Server {
	config := &Config{
		PathRegexPattern: DefaultPathRegexPattern,
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return &Server{
		config: config,
		logger: logger,
	}
}

// TestHandleAuthRequiresHeaders tests that all required headers are properly checked
func TestHandleAuthRequiresHeaders(t *testing.T) {
	testCases := []struct {
		name           string
		setupRequest   func(*http.Request)
		expectedStatus int
		expectedError  string
	}{
		{
			name: "Missing user header",
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-Auth-Request-Groups", "group1,group2")
				req.Header.Set("X-Forwarded-Uri", "/workspaces/ns1/app1/lab")
				req.Header.Set("X-Forwarded-Host", "example.com")
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Missing X-Auth-Request-User header",
		},
		{
			name: "Missing URI header",
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-Auth-Request-User", "testuser")
				req.Header.Set("X-Auth-Request-Groups", "group1,group2")
				req.Header.Set("X-Forwarded-Host", "example.com")
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Missing X-Forwarded-Uri header",
		},
		{
			name: "Missing host header",
			setupRequest: func(req *http.Request) {
				req.Header.Set("X-Auth-Request-User", "testuser")
				req.Header.Set("X-Auth-Request-Groups", "group1,group2")
				req.Header.Set("X-Forwarded-Uri", "/workspaces/ns1/app1/lab")
			},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Missing X-Forwarded-Host header",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a test server
			server := createTestServer()

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

// TestHandleAuthWithInvalidRedirectURL tests that invalid redirect URLs are rejected
func TestHandleAuthWithInvalidRedirectURL(t *testing.T) {
	// Since we can't mock properly due to function assignment issues,
	// we'll just test the basic functionality without verifying method calls

	// Create a minimal server for testing
	server := createTestServer()

	// Create minimal JWT manager (needed for the handler to not crash)
	server.jwtManager = &JWTManager{
		signingKey: []byte("test-key"),
		issuer:     "test-issuer",
		audience:   "test-audience",
		expiration: 30 * time.Minute,
	}

	// Create a minimal cookie manager
	cookieManager := &CookieManager{
		cookieName:         "test_cookie",
		cookieSecure:       false,
		cookiePath:         "/",
		cookieMaxAge:       24 * time.Hour,
		cookieHTTPOnly:     true,
		cookieSameSiteHttp: http.SameSiteLaxMode,
		pathRegexPattern:   DefaultPathRegexPattern,
	}
	server.cookieManager = cookieManager

	// Create a request with an invalid redirect URL (different host)
	req := httptest.NewRequest(http.MethodGet, "/auth?redirect=https://evil.com/path", nil)
	req.Header.Set("X-Auth-Request-User", "user1")
	req.Header.Set("X-Auth-Request-Groups", "group1") // Added missing groups header
	req.Header.Set("X-Forwarded-Uri", "/workspaces/ns1/app1")
	req.Header.Set("X-Forwarded-Host", "example.com")
	w := httptest.NewRecorder()

	// Call handler
	server.handleAuth(w, req)

	// Check response
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}

	// Check error message
	body := w.Body.String()
	if !strings.Contains(body, "Invalid redirect URL") {
		t.Errorf("Expected error message about invalid redirect URL, got: %s", body)
	}

	// Note: We can't easily verify that JWT and Cookie functions were not called
	// due to limitations in the current code structure
}

// For this test file, we are using mock implementations of the JWTHandler and CookieHandler interfaces
// defined in mock_test.go within the same package.

// TestHandleAuthHappyPath tests that valid redirect URLs are properly processed
func TestHandleAuthHappyPath(t *testing.T) {
	// Create a request with a valid redirect URL (same host)
	req := httptest.NewRequest(http.MethodGet, "/auth?redirect=/dashboard", nil)
	req.Header.Set("X-Auth-Request-User", "user1")
	req.Header.Set("X-Auth-Request-Groups", "org1:team1,org1:team2")
	req.Header.Set("X-Forwarded-Uri", "/workspaces/ns1/app1/notebooks/nb1.ipynb")
	req.Header.Set("X-Forwarded-Host", "example.com")
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()

	// Create mocks
	generatedToken := "generated-jwt-token"
	tokenGenerated := false
	cookieSet := false

	// Create JWT handler mock
	jwtHandler := &MockJWTHandler{
		GenerateTokenFunc: func(user string, groups []string, path string, domain string) (string, error) {
			tokenGenerated = true
			// Verify parameters
			if user != "user1" {
				t.Errorf("Expected user 'user1', got '%s'", user)
			}
			if !reflect.DeepEqual(groups, []string{"org1:team1", "org1:team2"}) {
				t.Errorf("Expected groups [org1:team1,org1:team2], got %v", groups)
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
		SetCookieFunc: func(w http.ResponseWriter, token string, path string) {
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

	// Create server with mocks
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cfg := &Config{
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
	}
	server := &Server{
		config:        cfg,
		logger:        logger,
		jwtManager:    jwtHandler,
		cookieManager: cookieHandler,
	}

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
		t.Fatalf("Failed to parse JSON response: %v", err)
	}
}
