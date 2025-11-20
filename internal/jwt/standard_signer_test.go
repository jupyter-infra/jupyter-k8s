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
	"strings"
	"testing"
	"time"

	jwt5 "github.com/golang-jwt/jwt/v5"
)

func TestStandardSigner_GenerateValidateRoundtrip(t *testing.T) {
	signer := NewStandardSigner("test-signing-key-32-characters-long", "test-issuer", "test-audience", time.Hour)

	// Generate token
	token, err := signer.GenerateToken("testuser", []string{"group1", "group2"}, "uid123", nil, "/path", "domain.com", TokenTypeSession)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Validate token
	claims, err := signer.ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	// Check claims
	if claims.User != "testuser" {
		t.Errorf("Expected user 'testuser', got %s", claims.User)
	}
	if len(claims.Groups) != 2 || claims.Groups[0] != "group1" || claims.Groups[1] != "group2" {
		t.Errorf("Expected groups [group1, group2], got %v", claims.Groups)
	}
	if claims.UID != "uid123" {
		t.Errorf("Expected UID 'uid123', got %s", claims.UID)
	}
	if claims.Path != "/path" {
		t.Errorf("Expected path '/path', got %s", claims.Path)
	}
	if claims.Domain != "domain.com" {
		t.Errorf("Expected domain 'domain.com', got %s", claims.Domain)
	}
	if claims.TokenType != TokenTypeSession {
		t.Errorf("Expected token type '%s', got %s", TokenTypeSession, claims.TokenType)
	}
}

func TestStandardSigner_ValidateToken_ExpiredToken(t *testing.T) {
	signer := NewStandardSigner("test-signing-key-32-characters-long", "test-issuer", "test-audience", -time.Hour) // Negative expiration

	// Generate expired token
	token, err := signer.GenerateToken("testuser", []string{}, "uid", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Try to validate expired token
	_, err = signer.ValidateToken(token)
	if err == nil {
		t.Fatal("Expected error for expired token")
	}
	if err != ErrTokenExpired {
		t.Errorf("Expected ErrTokenExpired, got %v", err)
	}
}

func TestStandardSigner_ValidateToken_InvalidSignature(t *testing.T) {
	signer1 := NewStandardSigner("key1-32-characters-long-enough", "test-issuer", "test-audience", time.Hour)
	signer2 := NewStandardSigner("key2-32-characters-long-enough", "test-issuer", "test-audience", time.Hour)

	// Generate token with signer1
	token, err := signer1.GenerateToken("testuser", []string{}, "uid", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Try to validate with signer2 (different key)
	_, err = signer2.ValidateToken(token)
	if err == nil {
		t.Fatal("Expected error for invalid signature")
	}
	if err != ErrInvalidSignature {
		t.Errorf("Expected ErrInvalidSignature, got %v", err)
	}
}

func TestStandardSigner_ValidateToken_WrongIssuer(t *testing.T) {
	signer1 := NewStandardSigner("test-signing-key-32-characters-long", "issuer1", "test-audience", time.Hour)
	signer2 := NewStandardSigner("test-signing-key-32-characters-long", "issuer2", "test-audience", time.Hour)

	// Generate token with signer1
	token, err := signer1.GenerateToken("testuser", []string{}, "uid", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Try to validate with signer2 (different issuer)
	_, err = signer2.ValidateToken(token)
	if err == nil {
		t.Fatal("Expected error for wrong issuer")
	}
	// Should be wrapped ErrInvalidToken
	if !strings.Contains(err.Error(), "invalid token") {
		t.Errorf("Expected error containing 'invalid token', got %v", err)
	}
}

func TestStandardSigner_ValidateToken_WrongAudience(t *testing.T) {
	signer1 := NewStandardSigner("test-signing-key-32-characters-long", "test-issuer", "audience1", time.Hour)
	signer2 := NewStandardSigner("test-signing-key-32-characters-long", "test-issuer", "audience2", time.Hour)

	// Generate token with signer1
	token, err := signer1.GenerateToken("testuser", []string{}, "uid", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Try to validate with signer2 (different audience)
	_, err = signer2.ValidateToken(token)
	if err == nil {
		t.Fatal("Expected error for wrong audience")
	}
	// Should be wrapped ErrInvalidToken
	if !strings.Contains(err.Error(), "invalid token") {
		t.Errorf("Expected error containing 'invalid token', got %v", err)
	}
}

func TestStandardSigner_ValidateToken_InvalidFormat(t *testing.T) {
	signer := NewStandardSigner("test-signing-key-32-characters-long", "test-issuer", "test-audience", time.Hour)

	// Try to validate malformed token
	_, err := signer.ValidateToken("not.a.jwt")
	if err == nil {
		t.Fatal("Expected error for malformed token")
	}
	// Should be wrapped ErrInvalidToken
	if !strings.Contains(err.Error(), "invalid token") {
		t.Errorf("Expected error containing 'invalid token', got %v", err)
	}
}

func TestStandardSigner_ValidateToken_WrongSigningMethod(t *testing.T) {
	// Create a token with RS256 instead of HS256
	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt5.NewNumericDate(now),
			Issuer:    "test-issuer",
			Audience:  []string{"test-audience"},
		},
		User: "testuser",
	}

	// Create token with wrong signing method (this will fail validation)
	token := jwt5.NewWithClaims(jwt5.SigningMethodRS256, claims)
	tokenString, _ := token.SignedString([]byte("fake-key")) // This will create invalid token

	signer := NewStandardSigner("test-signing-key-32-characters-long", "test-issuer", "test-audience", time.Hour)

	_, err := signer.ValidateToken(tokenString)
	if err == nil {
		t.Fatal("Expected error for wrong signing method")
	}
}

func TestStandardSigner_ValidateToken_EmptyToken(t *testing.T) {
	signer := NewStandardSigner("test-signing-key-32-characters-long", "test-issuer", "test-audience", time.Hour)

	_, err := signer.ValidateToken("")
	if err == nil {
		t.Fatal("Expected error for empty token")
	}
	if !strings.Contains(err.Error(), "invalid token") {
		t.Errorf("Expected error containing 'invalid token', got %v", err)
	}
}
