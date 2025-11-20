/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package jwt

import (
	"testing"
	"time"

	jwt5 "github.com/golang-jwt/jwt/v5"
)

const mockTokenValue = "mock-token"

// mockSigner implements Signer for testing
type mockSigner struct {
	generateFunc func(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string) (string, error)
	validateFunc func(tokenString string) (*Claims, error)
}

func (m *mockSigner) GenerateToken(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string) (string, error) {
	if m.generateFunc != nil {
		return m.generateFunc(user, groups, uid, extra, path, domain, tokenType)
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
	if !claims.SkipRefresh {
		t.Fatal("Expected SkipRefresh to be set to true")
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
