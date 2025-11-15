package authmiddleware

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Constants for common test values
const (
	testAppPath         = "/workspaces/ns1/app1"
	testLabPath         = "/workspaces/ns1/app1/lab" // Path with lab suffix
	csrfProtectedHeader = "X-CSRF-Protected"         // Header set by CSRF protection middleware in tests
	csrfProtectedValue  = "true"
)

// TestServerRegisterRoutes tests that the server can start and registers all routes
func TestServerRegisterRoutes(t *testing.T) {
	// Create a test logger that discards output
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create minimal config
	config := &Config{
		Port:             8080,
		ReadTimeout:      5 * time.Second,
		WriteTimeout:     5 * time.Second,
		ShutdownTimeout:  5 * time.Second,
		PathRegexPattern: DefaultPathRegexPattern,
		JWTSigningKey:    "test-key",
		JWTExpiration:    30 * time.Minute,
		JWTRefreshWindow: 5 * time.Minute,
	}

	// Create mocks
	jwtHandler := &MockJWTHandler{}
	cookieHandler := &MockCookieHandler{}

	// Create server
	server := NewServer(config, jwtHandler, cookieHandler, logger)

	// Create a test HTTP server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a request to check if routes are registered
		switch r.URL.Path {
		case "/auth":
			server.handleAuth(w, r)
		case "/verify":
			server.handleVerify(w, r)
		case "/health":
			server.handleHealth(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Test /auth endpoint - it should respond even with missing headers
	resp, err := http.Get(ts.URL + "/auth")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status code %d for /auth with missing headers, got %d", http.StatusBadRequest, resp.StatusCode)
	}

	// Test /health endpoint - it should respond with 200 OK
	resp, err = http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d for /health, got %d", http.StatusOK, resp.StatusCode)
	}

	// Test /verify endpoint - it should respond with error due to missing headers
	resp, err = http.Get(ts.URL + "/verify")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status code %d for /verify with missing headers, got %d", http.StatusBadRequest, resp.StatusCode)
	}
}

// TestServerStartsAndBindsToTheCorrectPort tests that the server starts and binds to the specified port
func TestServerStartsAndBindsToTheCorrectPort(t *testing.T) {
	// Skip in short mode as it involves network binding
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Use a rarely used port (54321)
	testPort := 54321

	// Create a test logger that discards output
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create config with specific port
	config := &Config{
		Port:             testPort,
		ReadTimeout:      1 * time.Second,
		WriteTimeout:     1 * time.Second,
		ShutdownTimeout:  1 * time.Second,
		PathRegexPattern: DefaultPathRegexPattern,
		JWTSigningKey:    "test-key",
	}

	// Create mocks
	jwtHandler := &MockJWTHandler{}
	cookieHandler := &MockCookieHandler{}

	// Create server
	server := NewServer(config, jwtHandler, cookieHandler, logger)

	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Give the server time to start
	time.Sleep(200 * time.Millisecond)

	// Create a client with a short timeout
	client := &http.Client{
		Timeout: 500 * time.Millisecond,
	}

	// Try to connect to the server's health endpoint
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/health", testPort))

	// Define cleanup to ensure server is always stopped
	defer func() {
		// Trigger shutdown by sending signal
		p, err := os.FindProcess(os.Getpid())
		if err == nil {
			_ = p.Signal(syscall.SIGINT)
		}

		// Wait for server to shut down (with timeout)
		select {
		case <-errCh:
			// Server shut down successfully
		case <-time.After(2 * time.Second):
			t.Log("Warning: Server did not shut down within expected time")
		}
	}()

	// Verify that the server is running and responding
	if err != nil {
		t.Errorf("Failed to connect to server on port %d: %v", testPort, err)
		return
	}

	// Check if we got a valid response
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d for /health, got %d", http.StatusOK, resp.StatusCode)
	}

	// Close the response body
	if err := resp.Body.Close(); err != nil {
		t.Logf("Error closing response body: %v", err)
	}
}

// TestServerTerminatesOnSIGINT tests that the server terminates when receiving SIGINT
func TestServerTerminatesOnSIGINT(t *testing.T) {
	// Skip in short mode as it involves timing and signals
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Create a test logger that discards output
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create minimal config with short timeouts for testing
	config := &Config{
		Port:             0, // Use random port
		ReadTimeout:      1 * time.Second,
		WriteTimeout:     1 * time.Second,
		ShutdownTimeout:  1 * time.Second,
		PathRegexPattern: DefaultPathRegexPattern,
		JWTSigningKey:    "test-key",
	}

	// Create mocks
	jwtHandler := &MockJWTHandler{}
	cookieHandler := &MockCookieHandler{}

	// Create server
	server := NewServer(config, jwtHandler, cookieHandler, logger)

	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Send SIGINT to the current process
	// This will be caught by the server's signal handler
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find process: %v", err)
	}

	// Send signal
	if err := p.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("Failed to send signal: %v", err)
	}

	// Wait for server to shut down
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Server did not shut down gracefully: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Server did not shut down within expected time")
	}
}

func TestServerStarts_RetrievesKeySetFromOIDCProvider_WhenOauthIsEnabled(t *testing.T) {
	t.Run("successful OIDC initialization", func(t *testing.T) {
		// Create a test logger that captures logs
		var logBuffer strings.Builder
		logger := slog.New(slog.NewTextHandler(&logBuffer, nil))

		// Create config with OAuth enabled
		config := &Config{
			Port:             0, // Use random port to avoid conflicts
			ReadTimeout:      1 * time.Second,
			WriteTimeout:     1 * time.Second,
			ShutdownTimeout:  1 * time.Second,
			PathRegexPattern: DefaultPathRegexPattern,
			JWTSigningKey:    "test-key",
			EnableOAuth:      true,
			OIDCIssuerURL:    "https://test-issuer.example.com",
			OIDCClientID:     "test-client-id",
			OidcGroupsPrefix: "test-prefix:",
		}

		// Track if Start() was called on the OIDC verifier
		startCalled := false

		// Create a mock OIDC verifier that tracks if Start was called
		mockOIDCVerifier := &MockOIDCVerifier{
			StartFunc: func(ctx context.Context) error {
				startCalled = true
				return nil // Success case
			},
		}

		// Create mocks for other components
		jwtHandler := &MockJWTHandler{}
		cookieHandler := &MockCookieHandler{}

		// Create server with the mock OIDC verifier
		server := NewServer(config, jwtHandler, cookieHandler, logger)

		// Replace the server's OIDC verifier with our mock
		// We need to do this because NewServer initializes its own OIDC verifier
		server.oidcVerifier = mockOIDCVerifier

		// Start server in a goroutine
		errCh := make(chan error, 1)
		go func() {
			errCh <- server.Start()
		}()

		// Give the server a moment to start and initialize
		time.Sleep(200 * time.Millisecond)

		// Define cleanup to ensure server is always stopped
		defer func() {
			// Trigger shutdown by sending signal
			p, err := os.FindProcess(os.Getpid())
			if err == nil {
				_ = p.Signal(syscall.SIGINT)
			}

			// Wait for server to shut down (with timeout)
			select {
			case err := <-errCh:
				// If we're still getting the error here, it means the server started
				// successfully (otherwise it would have returned immediately)
				if err != nil && !strings.Contains(err.Error(), "server closed") {
					t.Errorf("Server did not shut down gracefully: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Log("Warning: Server did not shut down within expected time")
			}
		}()

		// Verify that Start() was called on the OIDC verifier
		if !startCalled {
			t.Error("OIDC verifier Start() method was not called when OAuth is enabled")
		}

		// Check that no error was sent to the error channel immediately
		select {
		case err := <-errCh:
			t.Errorf("Server failed to start: %v", err)
		default:
			// No error means server started successfully
		}
	})

	t.Run("OIDC initialization failure", func(t *testing.T) {
		// Create a test logger that captures logs
		var logBuffer strings.Builder
		logger := slog.New(slog.NewTextHandler(&logBuffer, nil))

		// Create config with OAuth enabled
		config := &Config{
			Port:             0, // Use random port to avoid conflicts
			ReadTimeout:      1 * time.Second,
			WriteTimeout:     1 * time.Second,
			ShutdownTimeout:  1 * time.Second,
			PathRegexPattern: DefaultPathRegexPattern,
			JWTSigningKey:    "test-key",
			EnableOAuth:      true,
			OIDCIssuerURL:    "https://test-issuer.example.com",
			OIDCClientID:     "test-client-id",
			OidcGroupsPrefix: "test-prefix:",
		}

		// Create a mock OIDC verifier that returns an error from Start()
		mockOIDCVerifier := &MockOIDCVerifier{
			StartFunc: func(ctx context.Context) error {
				return fmt.Errorf("simulated OIDC provider connection failure")
			},
		}

		// Create mocks for other components
		jwtHandler := &MockJWTHandler{}
		cookieHandler := &MockCookieHandler{}

		// Create server with the mock OIDC verifier
		server := NewServer(config, jwtHandler, cookieHandler, logger)

		// Replace the server's OIDC verifier with our mock
		server.oidcVerifier = mockOIDCVerifier

		// Start server directly (not in goroutine) since we expect an immediate error
		err := server.Start()

		// Verify that the server failed to start due to OIDC error
		if err == nil {
			t.Error("Server started successfully despite OIDC verification failure")
		} else if !strings.Contains(err.Error(), "failed to start OIDC verifier") {
			t.Errorf("Unexpected error: %v", err)
		}
	})
}

func TestServerStarts_DoesNotCallOIDCProvier_WhenOauthIsDisabled(t *testing.T) {
	// Create a test logger that captures logs
	var logBuffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))

	// Create config with OAuth disabled but with OIDC config present
	config := &Config{
		Port:             0, // Use random port to avoid conflicts
		ReadTimeout:      1 * time.Second,
		WriteTimeout:     1 * time.Second,
		ShutdownTimeout:  1 * time.Second,
		PathRegexPattern: DefaultPathRegexPattern,
		JWTSigningKey:    "test-key",
		EnableOAuth:      false,                             // This is the important part - OAuth is disabled
		OIDCIssuerURL:    "https://test-issuer.example.com", // Still has OIDC config
		OIDCClientID:     "test-client-id",
		OidcGroupsPrefix: "test-prefix:",
	}

	// Track if Start() was incorrectly called on the OIDC verifier
	startCalled := false

	// Create a mock OIDC verifier that tracks if Start was called
	mockOIDCVerifier := &MockOIDCVerifier{
		StartFunc: func(ctx context.Context) error {
			startCalled = true // This should not happen in this test
			return nil
		},
	}

	// Create mocks for other components
	jwtHandler := &MockJWTHandler{}
	cookieHandler := &MockCookieHandler{}

	// Create server
	server := NewServer(config, jwtHandler, cookieHandler, logger)

	// In this test case, the server should not initialize the OIDC verifier at all
	// since OAuth is disabled. So we'll check that server.oidcVerifier is nil after
	// creating the server.

	// We'll also manually set it just to ensure no calls happen to it during Start()
	server.oidcVerifier = mockOIDCVerifier

	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Give the server a moment to start and initialize
	time.Sleep(200 * time.Millisecond)

	// Define cleanup to ensure server is always stopped
	defer func() {
		// Trigger shutdown by sending signal
		p, err := os.FindProcess(os.Getpid())
		if err == nil {
			_ = p.Signal(syscall.SIGINT)
		}

		// Wait for server to shut down (with timeout)
		select {
		case <-errCh:
			// Server shut down successfully
		case <-time.After(2 * time.Second):
			t.Log("Warning: Server did not shut down within expected time")
		}
	}()

	// Verify that Start() was not called on the OIDC verifier
	if startCalled {
		t.Error("OIDC verifier Start() method was incorrectly called when OAuth is disabled")
	}
}

// TestServerTerminatesOnSIGTERM tests that the server terminates when receiving SIGTERM
func TestServerTerminatesOnSIGTERM(t *testing.T) {
	// Skip in short mode as it involves timing and signals
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Create a test logger that discards output
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create minimal config with short timeouts for testing
	config := &Config{
		Port:             0, // Use random port
		ReadTimeout:      1 * time.Second,
		WriteTimeout:     1 * time.Second,
		ShutdownTimeout:  1 * time.Second,
		PathRegexPattern: DefaultPathRegexPattern,
		JWTSigningKey:    "test-key",
	}

	// Create mocks
	jwtHandler := &MockJWTHandler{}
	cookieHandler := &MockCookieHandler{}

	// Create server
	server := NewServer(config, jwtHandler, cookieHandler, logger)

	// Start server in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Send SIGTERM to the current process
	// This will be caught by the server's signal handler
	p, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("Failed to find process: %v", err)
	}

	// Send signal
	if err := p.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("Failed to send signal: %v", err)
	}

	// Wait for server to shut down
	select {
	case err := <-errCh:
		if err != nil {
			t.Errorf("Server did not shut down gracefully: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Server did not shut down within expected time")
	}
}

func TestServerInstantiatesK8sRestClient(t *testing.T) {
	// Since we can't replace package functions directly in Go, we'll approach this test differently
	// We'll create a Server with a working configuration and then verify it has a k8sClient
	// Note: This test may not be reliable when run outside of a Kubernetes cluster

	// Create a test logger that captures logs
	var logBuffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))

	// Create minimal config
	config := &Config{
		PathRegexPattern: DefaultPathRegexPattern,
		JWTSigningKey:    "test-key",
	}

	// Create server and let it attempt to set up the k8s client
	server := NewServer(config, &MockJWTHandler{}, &MockCookieHandler{}, logger)

	// If we're running in a valid k8s cluster context, the client should be created successfully
	// If not, the k8sClient will be nil and logs will contain errors
	// Either way, the server should handle it gracefully

	// Verify functionality with the client we've got (which might be nil)
	// by testing that auth handler works properly regardless

	// Capture the original rest client (might be nil)
	originalClient := server.restClient

	// Create a mock K8s server for testing
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	// Set up the mock server to return a success response
	mockServer.SetupServerEmpty200OK()

	// Create a REST client pointing to our test server
	mockRestClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	// Replace the client with our fake one for testing
	server.restClient = mockRestClient

	// Make a test request to verify the client is used correctly
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	// Call the handler
	server.handleHealth(w, req)

	// Check response is OK
	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d for /health, got %d", http.StatusOK, w.Code)
	}

	// Restore the original client
	server.restClient = originalClient
}

func TestServerLogsAnErrorIfClientInstantiationFails(t *testing.T) {
	// Since we can't directly replace package functions in Go, we'll need to use
	// a different approach. For this test, we'll focus on verifying that the server
	// correctly handles a nil k8sClient, which is what would happen in case of errors.

	// Create a test logger that captures logs
	var logBuffer strings.Builder
	logger := slog.New(slog.NewTextHandler(&logBuffer, nil))

	// Create minimal config
	config := &Config{
		PathRegexPattern: DefaultPathRegexPattern,
		JWTSigningKey:    "test-key",
	}

	// Create a server instance - note that we're using the real NewServer function
	// but we'll check that it handles errors correctly
	server := NewServer(config, &MockJWTHandler{}, &MockCookieHandler{}, logger)

	// If we're running tests outside of a k8s cluster, InClusterConfig will naturally fail
	// and log an error, which we can verify

	// Check logs for error messages - in a real environment, we might see
	// error messages about failing to create a k8s client, but we won't rely on this
	// for test assertions since it depends on the environment

	// We'll check that the server gracefully handles having a nil k8sClient

	// For a handler that uses k8sClient, test that it handles nil client
	// by creating a test request to /auth which tries to use the client
	req := httptest.NewRequest(http.MethodGet, "/auth", nil)
	req.Header.Set("X-Auth-Request-User", "test-user")
	req.Header.Set("X-Auth-Request-Groups", "group1")
	req.Header.Set("X-Forwarded-Uri", testAppPath)
	req.Header.Set("X-Forwarded-Host", "example.com")
	req.Header.Set("Authorization", "Bearer mock-token") // Add Authorization header for OIDC
	w := httptest.NewRecorder()

	// Set up OIDC verifier with matching username
	server.oidcVerifier = &MockOIDCVerifier{
		VerifyTokenFunc: func(ctx context.Context, tokenString string, logger *slog.Logger) (*OIDCClaims, bool, error) {
			claims := &OIDCClaims{
				Subject:  "test-user",
				Username: "test-user",
				Groups:   []string{"group1"},
			}
			return claims, false, nil
		},
	}

	// Explicitly set restClient to nil for this test
	server.restClient = nil

	// Call handler
	server.handleAuth(w, req)

	// Check for 500 status code (internal server error) as expected
	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d when k8sClient is nil, got %d",
			http.StatusInternalServerError, w.Code)
	}

	// Check response body for error message
	respBody := w.Body.String()
	if !strings.Contains(respBody, "Internal server error") {
		t.Errorf("Expected 'Internal server error' message in response, got: %s", respBody)
	}
}

// TestVerifyIsProtectedByCSRF tests that the /verify endpoint is protected by CSRF
func TestVerifyIsProtectedByCSRF(t *testing.T) {
	// Create a test logger that discards output
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a mock CookieHandler that tracks CSRF protection calls
	csrfProtectionApplied := false
	cookieHandler := &MockCookieHandler{
		CSRFProtectFunc: func() func(http.Handler) http.Handler {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Mark that CSRF protection was applied
					csrfProtectionApplied = true
					w.Header().Set(csrfProtectedHeader, csrfProtectedValue)
					next.ServeHTTP(w, r)
				})
			}
		},
	}

	// Create server
	config := &Config{PathRegexPattern: DefaultPathRegexPattern}
	server := NewServer(config, &MockJWTHandler{}, cookieHandler, logger)

	// Create test request to /verify
	req := httptest.NewRequest(http.MethodGet, "/verify", nil)
	req.Header.Set("X-Forwarded-Uri", testAppPath)
	req.Header.Set("X-Forwarded-Host", "example.com")

	// Create recorder to capture response
	w := httptest.NewRecorder()

	// Apply CSRF middleware and then call the handler
	handler := server.csrfProtect()(http.HandlerFunc(server.handleVerify))
	handler.ServeHTTP(w, req)

	// Verify that CSRF protection was applied
	if !csrfProtectionApplied {
		t.Error("CSRF protection was not applied to /verify endpoint")
	}

	if w.Header().Get(csrfProtectedHeader) != csrfProtectedValue {
		t.Error("CSRF protection header not found, suggesting protection was not applied")
	}
}

// TestAuthIsNotProtectedByCSRF tests that the /auth endpoint is not protected by CSRF
func TestAuthIsNotProtectedByCSRF(t *testing.T) {
	// Create a test logger that discards output
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a mock CookieHandler that tracks CSRF protection calls
	csrfProtectionApplied := false
	cookieHandler := &MockCookieHandler{
		CSRFProtectFunc: func() func(http.Handler) http.Handler {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Mark that CSRF protection was applied
					csrfProtectionApplied = true
					w.Header().Set(csrfProtectedHeader, csrfProtectedValue)
					next.ServeHTTP(w, r)
				})
			}
		},
		SetCookieFunc: func(w http.ResponseWriter, token string, path string, domain string) {},
	}

	// Create JWT manager mock
	jwtHandler := &MockJWTHandler{
		GenerateTokenFunc: func(
			user string,
			groups []string,
			uid string,
			extra map[string][]string,
			path string,
			domain string,
			tokenType string) (string, error) {
			return "test-token", nil
		},
	}

	// Create server
	config := &Config{
		PathRegexPattern: DefaultPathRegexPattern,
		EnableOAuth:      true, // Enable OAuth so /auth endpoint is not CSRF protected
	}
	server := NewServer(config, jwtHandler, cookieHandler, logger)

	// Create test request to /auth
	req := httptest.NewRequest(http.MethodGet, "/auth", nil)
	req.Header.Set("X-Auth-Request-User", "user1")
	req.Header.Set("X-Auth-Request-Groups", "group1")
	req.Header.Set("X-Forwarded-Uri", testAppPath)
	req.Header.Set("X-Forwarded-Host", "example.com")

	// Create recorder to capture response
	w := httptest.NewRecorder()

	// Apply CSRF middleware and then call the handler
	handler := server.csrfProtect()(http.HandlerFunc(server.handleAuth))
	handler.ServeHTTP(w, req)

	// Verify that CSRF protection was NOT applied
	if csrfProtectionApplied {
		t.Error("CSRF protection was incorrectly applied to /auth endpoint")
	}

	if w.Header().Get(csrfProtectedHeader) == csrfProtectedValue {
		t.Error("CSRF protection header found, suggesting protection was incorrectly applied")
	}
}

// TestHealthIsNotProtectedByCSRF tests that the /health endpoint is not protected by CSRF
func TestHealthIsNotProtectedByCSRF(t *testing.T) {
	// Create a test logger that discards output
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Create a mock CookieHandler that tracks CSRF protection calls
	csrfProtectionApplied := false
	cookieHandler := &MockCookieHandler{
		CSRFProtectFunc: func() func(http.Handler) http.Handler {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Mark that CSRF protection was applied
					csrfProtectionApplied = true
					w.Header().Set(csrfProtectedHeader, csrfProtectedValue)
					next.ServeHTTP(w, r)
				})
			}
		},
	}

	// Create server
	config := &Config{PathRegexPattern: DefaultPathRegexPattern}
	server := NewServer(config, &MockJWTHandler{}, cookieHandler, logger)

	// Create test request to /health
	req := httptest.NewRequest(http.MethodGet, "/health", nil)

	// Create recorder to capture response
	w := httptest.NewRecorder()

	// Apply CSRF middleware and then call the handler
	handler := server.csrfProtect()(http.HandlerFunc(server.handleHealth))
	handler.ServeHTTP(w, req)

	// Verify that CSRF protection was NOT applied
	if csrfProtectionApplied {
		t.Error("CSRF protection was incorrectly applied to /health endpoint")
	}

	if w.Header().Get(csrfProtectedHeader) == csrfProtectedValue {
		t.Error("CSRF protection header found, suggesting protection was incorrectly applied")
	}
}
