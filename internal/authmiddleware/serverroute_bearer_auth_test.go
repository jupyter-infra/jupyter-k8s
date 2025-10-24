package authmiddleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestHandleBearerAuthInvalidToken(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	jwtHandler := &MockJWTHandler{
		ValidateTokenFunc: func(tokenString string) (*Claims, error) {
			return nil, ErrInvalidToken
		},
	}
	server := &Server{
		logger:     logger,
		jwtManager: jwtHandler,
	}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	req.Header.Set(HeaderForwardedURI, "/workspaces/test/workspace?token=invalid")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
	if w.Body.String() != "Invalid token\n" {
		t.Errorf("Unexpected body: %s", w.Body.String())
	}
}

func TestHandleBearerAuthInvalidTokenType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	jwtHandler := &MockJWTHandler{
		ValidateTokenFunc: func(tokenString string) (*Claims, error) {
			return &Claims{
				User:      "test-user",
				Groups:    []string{"users"},
				Path:      "/workspaces/test/workspace",
				TokenType: TokenTypeSession, // Wrong type - should be bootstrap
			}, nil
		},
	}
	server := &Server{
		logger:     logger,
		jwtManager: jwtHandler,
	}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	req.Header.Set(HeaderForwardedURI, "/workspaces/test/workspace?token=session-token")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, w.Code)
	}
	if w.Body.String() != "Invalid token type\n" {
		t.Errorf("Unexpected body: %s", w.Body.String())
	}
}

func TestHandleBearerAuthMissingForwardedHost(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	jwtHandler := &MockJWTHandler{
		ValidateTokenFunc: func(tokenString string) (*Claims, error) {
			return &Claims{
				User:      "test-user",
				Groups:    []string{"users"},
				Path:      "/workspaces/test/workspace",
				TokenType: TokenTypeBootstrap,
			}, nil
		},
	}
	server := &Server{
		logger:     logger,
		jwtManager: jwtHandler,
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

func TestHandleBearerAuthTokenPathMismatch(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	jwtHandler := &MockJWTHandler{
		ValidateTokenFunc: func(tokenString string) (*Claims, error) {
			return &Claims{
				User:      "test-user",
				Groups:    []string{"users"},
				Path:      "/workspaces/different/workspace",
				TokenType: TokenTypeBootstrap,
			}, nil
		},
	}
	config := &Config{
		PathRegexPattern: "^(/workspaces/[^/]+/[^/]+)(?:/.*)?$",
	}
	server := &Server{
		logger:     logger,
		jwtManager: jwtHandler,
		config:     config,
	}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	req.Header.Set(HeaderForwardedURI, "/workspaces/test/workspace?token=valid")
	req.Header.Set(HeaderForwardedHost, "example.com")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status %d, got %d", http.StatusForbidden, w.Code)
	}
	if w.Body.String() != "Token path mismatch\n" {
		t.Errorf("Unexpected body: %s", w.Body.String())
	}
}

func TestHandleBearerAuthSuccess(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	jwtHandler := &MockJWTHandler{
		ValidateTokenFunc: func(tokenString string) (*Claims, error) {
			return &Claims{
				User:      "test-user",
				Groups:    []string{"users"},
				Path:      "/workspaces/test/workspace",
				TokenType: TokenTypeBootstrap, // Correct type for bearer auth
			}, nil
		},
		GenerateTokenFunc: func(user string, groups []string, path string, domain string, tokenType string) (string, error) {
			return "session-token", nil
		},
	}
	cookieHandler := &MockCookieHandler{
		SetCookieFunc: func(w http.ResponseWriter, token string, path string) {
			// Mock cookie setting
		},
	}
	config := &Config{
		// Use the actual production regex pattern
		PathRegexPattern: DefaultPathRegexPattern,
	}
	server := &Server{
		logger:        logger,
		jwtManager:    jwtHandler,
		cookieManager: cookieHandler,
		config:        config,
	}

	req := httptest.NewRequest(http.MethodGet, "/bearer-auth", nil)
	// Use a path that matches the production regex - add trailing slash to match (?:/.*)?
	req.Header.Set(HeaderForwardedURI, "/workspaces/test/workspace/?token=valid")
	req.Header.Set(HeaderForwardedHost, "example.com")
	w := httptest.NewRecorder()

	server.handleBearerAuth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d. Body: %s", http.StatusOK, w.Code, w.Body.String())
	}
}
