package authmiddleware

import (
	"net/http"
)

// MockJWTHandler implements the JWTHandler interface for testing
type MockJWTHandler struct {
	GenerateTokenFunc      func(user string, groups []string, path string, domain string, tokenType string) (string, error)
	ValidateTokenFunc      func(tokenString string) (*Claims, error)
	RefreshTokenFunc       func(claims *Claims) (string, error)
	ShouldRefreshTokenFunc func(claims *Claims) bool
}

// Ensure MockJWTHandler implements the JWTHandler interface
var _ JWTHandler = (*MockJWTHandler)(nil)

// GenerateToken calls the mock implementation
func (m *MockJWTHandler) GenerateToken(user string, groups []string, path string, domain string, tokenType string) (string, error) {
	if m.GenerateTokenFunc != nil {
		return m.GenerateTokenFunc(user, groups, path, domain, tokenType)
	}
	return "mock-token", nil
}

// ValidateToken calls the mock implementation
func (m *MockJWTHandler) ValidateToken(tokenString string) (*Claims, error) {
	if m.ValidateTokenFunc != nil {
		return m.ValidateTokenFunc(tokenString)
	}
	return &Claims{User: "mock-user"}, nil
}

// RefreshToken calls the mock implementation
func (m *MockJWTHandler) RefreshToken(claims *Claims) (string, error) {
	if m.RefreshTokenFunc != nil {
		return m.RefreshTokenFunc(claims)
	}
	return "refreshed-mock-token", nil
}

// ShouldRefreshToken calls the mock implementation
func (m *MockJWTHandler) ShouldRefreshToken(claims *Claims) bool {
	if m.ShouldRefreshTokenFunc != nil {
		return m.ShouldRefreshTokenFunc(claims)
	}
	return false
}

// MockCookieHandler implements the CookieHandler interface for testing
type MockCookieHandler struct {
	SetCookieFunc   func(w http.ResponseWriter, token string, path string)
	GetCookieFunc   func(r *http.Request, path string) (string, error)
	ClearCookieFunc func(w http.ResponseWriter, path string)
	CSRFProtectFunc func() func(http.Handler) http.Handler
}

// Ensure MockCookieHandler implements the CookieHandler interface
var _ CookieHandler = (*MockCookieHandler)(nil)

// SetCookie calls the mock implementation
func (m *MockCookieHandler) SetCookie(w http.ResponseWriter, token string, path string) {
	if m.SetCookieFunc != nil {
		m.SetCookieFunc(w, token, path)
	}
}

// GetCookie calls the mock implementation
func (m *MockCookieHandler) GetCookie(r *http.Request, path string) (string, error) {
	if m.GetCookieFunc != nil {
		return m.GetCookieFunc(r, path)
	}
	return "mock-cookie-value", nil
}

// ClearCookie calls the mock implementation
func (m *MockCookieHandler) ClearCookie(w http.ResponseWriter, path string) {
	if m.ClearCookieFunc != nil {
		m.ClearCookieFunc(w, path)
	}
}

// CSRFProtect calls the mock implementation
func (m *MockCookieHandler) CSRFProtect() func(http.Handler) http.Handler {
	if m.CSRFProtectFunc != nil {
		return m.CSRFProtectFunc()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}
}
