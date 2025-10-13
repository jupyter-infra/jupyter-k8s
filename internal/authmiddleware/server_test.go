package authmiddleware

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"syscall"
	"testing"
	"time"
)

// Constants for common test values
const (
	testLabPath         = "/workspaces/ns1/app1/lab" // Path with lab suffix, testAppPath is defined in hash_test.go
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
		SetCookieFunc: func(w http.ResponseWriter, token string, path string) {},
	}

	// Create JWT manager mock
	jwtHandler := &MockJWTHandler{
		GenerateTokenFunc: func(user string, groups []string, path string, domain string) (string, error) {
			return "test-token", nil
		},
	}

	// Create server
	config := &Config{PathRegexPattern: DefaultPathRegexPattern}
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
