/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

import (
	"strings"
	"testing"
	"time"

	jwt5 "github.com/golang-jwt/jwt/v5"
)

const mockTokenValue = "mock-token"

// mockSigner implements Signer for testing
type mockSigner struct {
	generateFunc     func(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string, skipRefresh bool) (string, error)
	validateFunc     func(tokenString string) (*Claims, error)
	refreshTokenFunc func(claims *Claims) (string, error)
}

func (m *mockSigner) GenerateToken(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string, skipRefresh bool) (string, error) {
	if m.generateFunc != nil {
		return m.generateFunc(user, groups, uid, extra, path, domain, tokenType, skipRefresh)
	}
	return mockTokenValue, nil
}

func (m *mockSigner) GenerateRefreshToken(claims *Claims) (string, error) {
	if m.refreshTokenFunc != nil {
		return m.refreshTokenFunc(claims)
	}
	return mockTokenValue, nil
}

func (m *mockSigner) ValidateToken(tokenString string) (*Claims, error) {
	if m.validateFunc != nil {
		return m.validateFunc(tokenString)
	}
	now := time.Now().UTC()
	return &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt5.NewNumericDate(now),
		},
		User: "testuser",
	}, nil
}

func TestManager_GenerateToken(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, false, 0, 0)

	token, err := manager.GenerateToken("user", []string{"group1"}, "uid", nil, "/path", "domain", "session")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if token != mockTokenValue {
		t.Fatalf("Expected '%s', got %s", mockTokenValue, token)
	}
}

func TestManager_ValidateToken(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, false, 0, 0)

	claims, err := manager.ValidateToken("test-token")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if claims.User != "testuser" {
		t.Fatalf("Expected user 'testuser', got %s", claims.User)
	}
}

func TestManager_RefreshToken_Success(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, true, time.Minute, time.Hour)

	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt5.NewNumericDate(now),
		},
		User:   "testuser",
		Groups: []string{"group1"},
	}

	token, err := manager.RefreshToken(claims)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if token != mockTokenValue {
		t.Fatalf("Expected '%s', got %s", mockTokenValue, token)
	}
}

func TestManager_RefreshToken_NilClaims(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, true, time.Minute, time.Hour)

	_, err := manager.RefreshToken(nil)
	if err == nil {
		t.Fatal("Expected error for nil claims")
	}
	if err.Error() != "claims cannot be nil" {
		t.Fatalf("Expected 'claims cannot be nil', got %s", err.Error())
	}
}

func TestManager_ShouldRefreshToken_WithinWindow(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, true, 10*time.Minute, time.Hour)

	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(5 * time.Minute)),   // Expires in 5 min, within 10 min window
			IssuedAt:  jwt5.NewNumericDate(now.Add(-10 * time.Minute)), // Issued 10 min ago, within 1 hour horizon
		},
	}

	if !manager.ShouldRefreshToken(claims) {
		t.Fatal("Expected token to need refresh")
	}
}

func TestManager_ShouldRefreshToken_OutsideWindow(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, true, 10*time.Minute, time.Hour)

	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(20 * time.Minute)), // Expires in 20 min, outside 10 min window
			IssuedAt:  jwt5.NewNumericDate(now),
		},
	}

	if manager.ShouldRefreshToken(claims) {
		t.Fatal("Expected token to not need refresh")
	}
}

func TestManager_ShouldRefreshToken_RefreshDisabled(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, false, 10*time.Minute, time.Hour) // Refresh disabled

	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(5 * time.Minute)),
			IssuedAt:  jwt5.NewNumericDate(now),
		},
	}

	if manager.ShouldRefreshToken(claims) {
		t.Fatal("Expected no refresh when disabled")
	}
}

func TestManager_ShouldRefreshToken_SkipRefresh(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, true, 10*time.Minute, time.Hour)

	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(5 * time.Minute)),
			IssuedAt:  jwt5.NewNumericDate(now),
		},
		SkipRefresh: true,
	}

	if manager.ShouldRefreshToken(claims) {
		t.Fatal("Expected no refresh when SkipRefresh is true")
	}
}

func TestManager_UpdateSkipRefreshToken_Success(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, true, time.Minute, time.Hour)

	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt5.NewNumericDate(now),
		},
		User:   "testuser",
		Groups: []string{"group1"},
	}

	token, err := manager.UpdateSkipRefreshToken(claims)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if token != mockTokenValue {
		t.Fatalf("Expected '%s', got %s", mockTokenValue, token)
	}
}

func TestManager_UpdateSkipRefreshToken_NilClaims(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, true, time.Minute, time.Hour)

	_, err := manager.UpdateSkipRefreshToken(nil)
	if err == nil {
		t.Fatal("Expected error for nil claims")
	}
	if err.Error() != "claims cannot be nil" {
		t.Fatalf("Expected 'claims cannot be nil', got %s", err.Error())
	}
}

func TestManager_ShouldRefreshToken_AlreadyExpired(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, true, 10*time.Minute, time.Hour)

	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(-1 * time.Minute)), // Already expired
			IssuedAt:  jwt5.NewNumericDate(now.Add(-30 * time.Minute)),
		},
	}

	if manager.ShouldRefreshToken(claims) {
		t.Fatal("Expected no refresh when token is already expired")
	}
}

func TestManager_ShouldRefreshToken_BeyondHorizon_StillReturnsTrue(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, true, 10*time.Minute, time.Hour)

	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(5 * time.Minute)),
			IssuedAt:  jwt5.NewNumericDate(now.Add(-2 * time.Hour)),
		},
	}

	if !manager.ShouldRefreshToken(claims) {
		t.Fatal("Expected token to need refresh (horizon is enforced in RefreshToken, not ShouldRefreshToken)")
	}
}

func TestManager_ShouldRefreshToken_NilExpiresAt(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, true, 10*time.Minute, time.Hour)

	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: nil, // Nil ExpiresAt
			IssuedAt:  jwt5.NewNumericDate(time.Now().UTC()),
		},
	}

	if manager.ShouldRefreshToken(claims) {
		t.Fatal("Expected no refresh when ExpiresAt is nil")
	}
}

func TestManager_ShouldRefreshToken_NilIssuedAt(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, true, 10*time.Minute, time.Hour)

	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(5 * time.Minute)),
			IssuedAt:  nil, // Nil IssuedAt - this would cause panic in time calculation
		},
	}

	// This should not panic and should return false
	if manager.ShouldRefreshToken(claims) {
		t.Fatal("Expected no refresh when IssuedAt is nil")
	}
}

func TestManager_RefreshToken_PreservesIssuedAt(t *testing.T) {
	originalIssuedAt := time.Now().UTC().Add(-30 * time.Minute)
	refreshCalled := false

	signer := &mockSigner{
		refreshTokenFunc: func(claims *Claims) (string, error) {
			refreshCalled = true
			// Verify the claims passed have the original IssuedAt
			if claims.IssuedAt.Unix() != originalIssuedAt.Unix() {
				t.Errorf("Expected IssuedAt %v, got %v", originalIssuedAt, claims.IssuedAt.Time)
			}
			return mockTokenValue, nil
		},
	}
	manager := NewManager(signer, true, time.Minute, time.Hour)

	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(time.Now().UTC().Add(time.Hour)),
			IssuedAt:  jwt5.NewNumericDate(originalIssuedAt),
		},
		User:   "testuser",
		Groups: []string{"group1"},
	}

	token, err := manager.RefreshToken(claims)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if token != mockTokenValue {
		t.Fatalf("Expected '%s', got %s", mockTokenValue, token)
	}
	if !refreshCalled {
		t.Fatal("Expected GenerateRefreshToken to be called")
	}
}

func TestManager_RefreshToken_BeyondHorizon_ReturnsError(t *testing.T) {
	signer := &mockSigner{}
	manager := NewManager(signer, true, time.Minute, time.Hour) // 1 hour horizon

	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(time.Now().UTC().Add(time.Hour)),
			IssuedAt:  jwt5.NewNumericDate(time.Now().UTC().Add(-2 * time.Hour)), // 2 hours ago, beyond 1h horizon
		},
		User:   "testuser",
		Groups: []string{"group1"},
	}

	_, err := manager.RefreshToken(claims)
	if err == nil {
		t.Fatal("Expected error when beyond refresh horizon")
	}
	if !strings.Contains(err.Error(), "beyond refresh horizon") {
		t.Fatalf("Expected horizon error, got: %v", err)
	}
}

func TestManager_UpdateSkipRefreshToken_SetsSkipRefreshTrue(t *testing.T) {
	skipRefreshValue := false
	signer := &mockSigner{
		generateFunc: func(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string, skipRefresh bool) (string, error) {
			skipRefreshValue = skipRefresh
			return mockTokenValue, nil
		},
	}
	manager := NewManager(signer, true, time.Minute, time.Hour)

	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(time.Now().UTC().Add(time.Hour)),
			IssuedAt:  jwt5.NewNumericDate(time.Now().UTC()),
		},
		User:   "testuser",
		Groups: []string{"group1"},
	}

	_, err := manager.UpdateSkipRefreshToken(claims)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if !skipRefreshValue {
		t.Fatal("Expected GenerateToken to be called with skipRefresh=true")
	}
}
