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

const testUser = "testuser"

// Helper function to create a test signer with a single key
func createTestSigner(key, issuer, audience string, expiration time.Duration) *StandardSigner {
	kid := "1234567890"
	signingKeys := map[string][]byte{
		kid: []byte(key),
	}
	signer := NewStandardSigner(issuer, audience, expiration, 0)
	// Load initial keys for testing
	_ = signer.UpdateKeys(signingKeys, kid)
	return signer
}

func TestStandardSigner_GenerateValidateRoundtrip(t *testing.T) {
	signer := createTestSigner("test-signing-key-32-characters-long", "test-issuer", "test-audience", time.Hour)

	// Generate token
	token, err := signer.GenerateToken(testUser, []string{"group1", "group2"}, "uid123", nil, "/path", "domain.com", TokenTypeSession)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Validate token
	claims, err := signer.ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	// Check claims
	if claims.User != testUser {
		t.Errorf("Expected user '%s', got %s", testUser, claims.User)
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
	signer := createTestSigner("test-signing-key-32-characters-long", "test-issuer", "test-audience", -time.Hour) // Negative expiration

	// Generate expired token
	token, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
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
	signer1 := createTestSigner("key1-32-characters-long-enough", "test-issuer", "test-audience", time.Hour)
	signer2 := createTestSigner("key2-32-characters-long-enough", "test-issuer", "test-audience", time.Hour)

	// Generate token with signer1
	token, err := signer1.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
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
	signer1 := createTestSigner("test-signing-key-32-characters-long", "issuer1", "test-audience", time.Hour)
	signer2 := createTestSigner("test-signing-key-32-characters-long", "issuer2", "test-audience", time.Hour)

	// Generate token with signer1
	token, err := signer1.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
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
	signer1 := createTestSigner("test-signing-key-32-characters-long", "test-issuer", "audience1", time.Hour)
	signer2 := createTestSigner("test-signing-key-32-characters-long", "test-issuer", "audience2", time.Hour)

	// Generate token with signer1
	token, err := signer1.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
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
	signer := createTestSigner("test-signing-key-32-characters-long", "test-issuer", "test-audience", time.Hour)

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
		User: testUser,
	}

	// Create token with wrong signing method (this will fail validation)
	token := jwt5.NewWithClaims(jwt5.SigningMethodRS256, claims)
	tokenString, _ := token.SignedString([]byte("fake-key")) // This will create invalid token

	signer := createTestSigner("test-signing-key-32-characters-long", "test-issuer", "test-audience", time.Hour)

	_, err := signer.ValidateToken(tokenString)
	if err == nil {
		t.Fatal("Expected error for wrong signing method")
	}
}

func TestStandardSigner_ValidateToken_EmptyToken(t *testing.T) {
	signer := createTestSigner("test-signing-key-32-characters-long", "test-issuer", "test-audience", time.Hour)

	_, err := signer.ValidateToken("")
	if err == nil {
		t.Fatal("Expected error for empty token")
	}
	if !strings.Contains(err.Error(), "invalid token") {
		t.Errorf("Expected error containing 'invalid token', got %v", err)
	}
}

func TestStandardSigner_MultipleKeys_Validation(t *testing.T) {
	// Create signer with multiple keys
	signingKeys := map[string][]byte{
		"1000": []byte("key1-32-characters-long-enough"),
		"2000": []byte("key2-32-characters-long-enough"),
		"3000": []byte("key3-32-characters-long-enough"),
	}
	latestKid := "3000"
	signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 0)
	_ = signer.UpdateKeys(signingKeys, latestKid)

	// Generate token (should use latest key)
	token, err := signer.GenerateToken(testUser, []string{"group1"}, "uid123", nil, "/path", "domain.com", TokenTypeSession)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Should be able to validate with same signer
	claims, err := signer.ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token with multi-key signer: %v", err)
	}

	if claims.User != testUser {
		t.Errorf("Expected user 'testUser', got %s", claims.User)
	}
}

func TestStandardSigner_UpdateKeys_HotReload(t *testing.T) {
	// Create initial signer with one key
	initialKeys := map[string][]byte{
		"1000": []byte("initial-key-32-characters-long"),
	}
	signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 0)
	_ = signer.UpdateKeys(initialKeys, "1000")

	// Generate token with initial key
	token, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Update keys to include old and new keys
	updatedKeys := map[string][]byte{
		"1000": []byte("initial-key-32-characters-long"),
		"2000": []byte("new-key-32-characters-long-here"),
	}
	if err := signer.UpdateKeys(updatedKeys, "2000"); err != nil {
		t.Fatalf("Failed to update keys: %v", err)
	}

	// Old token should still validate (backward compatibility)
	claims, err := signer.ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate old token after key update: %v", err)
	}
	if claims.User != testUser {
		t.Errorf("Expected user 'testUser', got %s", claims.User)
	}

	// New tokens should use the new latest key
	newToken, err := signer.GenerateToken("newuser", []string{}, "uid2", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate new token: %v", err)
	}

	claims2, err := signer.ValidateToken(newToken)
	if err != nil {
		t.Fatalf("Failed to validate new token: %v", err)
	}
	if claims2.User != "newuser" {
		t.Errorf("Expected user 'newuser', got %s", claims2.User)
	}
}

func TestStandardSigner_UpdateKeys_KeyRemoval(t *testing.T) {
	// Create signer with two keys
	initialKeys := map[string][]byte{
		"1000": []byte("old-key-32-characters-long-here"),
		"2000": []byte("new-key-32-characters-long-here"),
	}
	signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 0)
	_ = signer.UpdateKeys(initialKeys, "2000")

	// Generate token with old key manually by creating signer with only old key
	oldKeySigner := NewStandardSigner("test-issuer", "test-audience", time.Hour, 0)
	_ = oldKeySigner.UpdateKeys(
		map[string][]byte{"1000": []byte("old-key-32-characters-long-here")},
		"1000",
	)
	oldToken, err := oldKeySigner.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate old token: %v", err)
	}

	// Token should validate with both keys
	_, err = signer.ValidateToken(oldToken)
	if err != nil {
		t.Fatalf("Old token should validate with both keys present: %v", err)
	}

	// Update keys to remove old key
	updatedKeys := map[string][]byte{
		"2000": []byte("new-key-32-characters-long-here"),
	}
	if err := signer.UpdateKeys(updatedKeys, "2000"); err != nil {
		t.Fatalf("Failed to update keys: %v", err)
	}

	// Old token should now fail validation (key removed)
	_, err = signer.ValidateToken(oldToken)
	if err == nil {
		t.Fatal("Expected error validating old token after key removal")
	}
	if !strings.Contains(err.Error(), "unknown key ID") {
		t.Errorf("Expected error about unknown key ID, got %v", err)
	}
}

func TestStandardSigner_ValidateToken_MissingKidHeader(t *testing.T) {
	signingKeys := map[string][]byte{
		"1000": []byte("test-signing-key-32-characters-long"),
	}
	signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 0)
	_ = signer.UpdateKeys(signingKeys, "1000")

	// Create a token without kid header (using direct JWT library)
	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt5.NewNumericDate(now),
			Issuer:    "test-issuer",
			Audience:  []string{"test-audience"},
		},
		User: testUser,
	}

	// Create token without kid header
	token := jwt5.NewWithClaims(jwt5.SigningMethodHS384, claims)
	tokenString, err := token.SignedString([]byte("test-signing-key-32-characters-long"))
	if err != nil {
		t.Fatalf("Failed to create test token: %v", err)
	}

	// Validation should fail due to missing kid
	_, err = signer.ValidateToken(tokenString)
	if err == nil {
		t.Fatal("Expected error for missing kid header")
	}
	if !strings.Contains(err.Error(), "kid") {
		t.Errorf("Expected error about missing kid, got: %v", err)
	}
}

func TestStandardSigner_ValidateToken_UnknownKid(t *testing.T) {
	signingKeys := map[string][]byte{
		"1000": []byte("test-signing-key-32-characters-long"),
		"2000": []byte("another-key-32-characters-long-1"),
	}
	signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 0)
	_ = signer.UpdateKeys(signingKeys, "2000")

	// Create a signer with a different kid
	otherSigner := NewStandardSigner("test-issuer", "test-audience", time.Hour, 0)
	_ = otherSigner.UpdateKeys(
		map[string][]byte{"9999": []byte("unknown-key-32-characters-long-1")},
		"9999",
	)

	// Generate token with unknown kid
	token, err := otherSigner.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Validation should fail due to unknown kid
	_, err = signer.ValidateToken(token)
	if err == nil {
		t.Fatal("Expected error for unknown kid")
	}
	if !strings.Contains(err.Error(), "unknown key ID") {
		t.Errorf("Expected error about unknown key ID, got: %v", err)
	}
}

func TestStandardSigner_HS384Algorithm(t *testing.T) {
	signingKeys := map[string][]byte{
		"1000": []byte("test-signing-key-32-characters-long"),
	}
	signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 0)
	_ = signer.UpdateKeys(signingKeys, "1000")

	// Generate token
	token, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Parse token to verify algorithm
	parsedToken, _, err := jwt5.NewParser().ParseUnverified(token, &Claims{})
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	if parsedToken.Method.Alg() != "HS384" {
		t.Errorf("Expected HS384 algorithm, got %s", parsedToken.Method.Alg())
	}
}

func TestStandardSigner_CoolOffKeySelection(t *testing.T) {
	t.Run("no keys beyond cooloff returns error", func(t *testing.T) {
		signingKeys := map[string][]byte{
			"1000": []byte("key1-32-characters-long-enough"),
		}
		signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 5*time.Second)
		_ = signer.UpdateKeys(signingKeys, "1000")

		// Manually set keyAddedTimes to recent times (within cooloff)
		signer.mu.Lock()
		signer.keyAddedTimes = map[string]time.Time{
			"1000": time.Now().Add(-1 * time.Second),
			"2000": time.Now().Add(-500 * time.Millisecond),
		}
		signer.mu.Unlock()

		// Should fail to generate token since all keys are within cooloff
		_, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
		if err == nil {
			t.Error("Expected error when all keys within cooloff, got nil")
		}
		if !strings.Contains(err.Error(), "no signing key available") {
			t.Errorf("Expected 'no signing key available' error, got: %v", err)
		}
	})

	t.Run("one key beyond cooloff is used", func(t *testing.T) {
		signingKeys := map[string][]byte{
			"1000": []byte("key1-32-characters-long-enough"),
			"2000": []byte("key2-32-characters-long-enough"),
		}
		signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 2*time.Second)
		_ = signer.UpdateKeys(signingKeys, "2000")

		// Manually set keyAddedTimes
		signer.mu.Lock()
		signer.keyAddedTimes = map[string]time.Time{
			"1000": time.Now().Add(-10 * time.Second), // Beyond cooloff
			"2000": time.Now().Add(-1 * time.Second),  // Within cooloff
		}
		signer.mu.Unlock()

		token, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
		if err != nil {
			t.Fatalf("Expected token generation to succeed, got error: %v", err)
		}

		// Verify token uses kid "1000"
		parsedToken, _, _ := jwt5.NewParser().ParseUnverified(token, &Claims{})
		if kid, ok := parsedToken.Header["kid"].(string); !ok || kid != "1000" {
			t.Errorf("Expected token to use kid '1000', got %v", parsedToken.Header["kid"])
		}
	})

	t.Run("multiple keys beyond cooloff returns latest", func(t *testing.T) {
		signingKeys := map[string][]byte{
			"1000": []byte("key1-32-characters-long-enough"),
			"2000": []byte("key2-32-characters-long-enough"),
			"3000": []byte("key3-32-characters-long-enough"),
		}
		signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 2*time.Second)
		_ = signer.UpdateKeys(signingKeys, "3000")

		// Manually set keyAddedTimes - all beyond cooloff
		signer.mu.Lock()
		signer.keyAddedTimes = map[string]time.Time{
			"1000": time.Now().Add(-10 * time.Second),
			"2000": time.Now().Add(-8 * time.Second),
			"3000": time.Now().Add(-6 * time.Second),
		}
		signer.mu.Unlock()

		token, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
		if err != nil {
			t.Fatalf("Expected token generation to succeed, got error: %v", err)
		}

		// Should use latest kid "3000"
		parsedToken, _, _ := jwt5.NewParser().ParseUnverified(token, &Claims{})
		if kid, ok := parsedToken.Header["kid"].(string); !ok || kid != "3000" {
			t.Errorf("Expected token to use latest kid '3000', got %v", parsedToken.Header["kid"])
		}
	})

	t.Run("zero cooloff period makes all keys immediately usable", func(t *testing.T) {
		signingKeys := map[string][]byte{
			"1000": []byte("key1-32-characters-long-enough"),
			"2000": []byte("key2-32-characters-long-enough"),
			"3000": []byte("key3-32-characters-long-enough"),
		}
		signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 0)
		_ = signer.UpdateKeys(signingKeys, "3000")

		// Manually set keyAddedTimes to very recent
		signer.mu.Lock()
		signer.keyAddedTimes = map[string]time.Time{
			"1000": time.Now().Add(-100 * time.Millisecond),
			"2000": time.Now().Add(-50 * time.Millisecond),
			"3000": time.Now().Add(-10 * time.Millisecond),
		}
		signer.mu.Unlock()

		token, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
		if err != nil {
			t.Fatalf("Expected token generation to succeed with zero cooloff, got error: %v", err)
		}

		// Should use latest kid "3000" even though it's very recent
		parsedToken, _, _ := jwt5.NewParser().ParseUnverified(token, &Claims{})
		if kid, ok := parsedToken.Header["kid"].(string); !ok || kid != "3000" {
			t.Errorf("Expected token to use latest kid '3000' with zero cooloff, got %v", parsedToken.Header["kid"])
		}
	})

	t.Run("lexicographic ordering selects latest", func(t *testing.T) {
		signingKeys := map[string][]byte{
			"1000": []byte("key1-32-characters-long-enough"),
			"1500": []byte("key2-32-characters-long-enough"),
			"2000": []byte("key3-32-characters-long-enough"),
		}
		signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 1*time.Second)
		_ = signer.UpdateKeys(signingKeys, "2000")

		// Set keys with non-sequential timestamps but all beyond cooloff
		signer.mu.Lock()
		signer.keyAddedTimes = map[string]time.Time{
			"2000": time.Now().Add(-10 * time.Second),
			"1000": time.Now().Add(-20 * time.Second),
			"1500": time.Now().Add(-15 * time.Second),
		}
		signer.mu.Unlock()

		token, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
		if err != nil {
			t.Fatalf("Expected token generation to succeed, got error: %v", err)
		}

		// "2000" > "1500" > "1000" lexicographically
		parsedToken, _, _ := jwt5.NewParser().ParseUnverified(token, &Claims{})
		if kid, ok := parsedToken.Header["kid"].(string); !ok || kid != "2000" {
			t.Errorf("Expected lexicographically latest kid '2000', got %v", parsedToken.Header["kid"])
		}
	})

	t.Run("returns both kid and signing key correctly", func(t *testing.T) {
		keyData := []byte("test-signing-key-32-characters-l")
		signingKeys := map[string][]byte{
			"1000": keyData,
		}
		signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 1*time.Second)
		_ = signer.UpdateKeys(signingKeys, "1000")

		// Set key beyond cooloff
		signer.mu.Lock()
		signer.keyAddedTimes = map[string]time.Time{
			"1000": time.Now().Add(-10 * time.Second),
		}
		signer.mu.Unlock()

		// Generate token and validate it can be verified (proving the key was correctly retrieved)
		token, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
		if err != nil {
			t.Fatalf("Expected token generation to succeed, got error: %v", err)
		}

		// If ValidateToken succeeds, it means the kid and key were both correct
		claims, err := signer.ValidateToken(token)
		if err != nil {
			t.Errorf("Token validation failed, indicating kid/key mismatch: %v", err)
		}
		if claims.User != testUser {
			t.Errorf("Expected user %s, got %s", testUser, claims.User)
		}
	})
}

func TestStandardSigner_NewKeyUseDelay(t *testing.T) {
	// Create signer with 2 second cooloff period
	initialKeys := map[string][]byte{
		"1000": []byte("initial-key-32-characters-long"),
	}
	signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 2*time.Second)
	_ = signer.UpdateKeys(initialKeys, "1000")

	// Simulate that initial key was added long ago (beyond cooloff)
	signer.mu.Lock()
	signer.keyAddedTimes["1000"] = time.Now().Add(-10 * time.Second)
	signer.mu.Unlock()

	// Generate token with initial key (should work immediately since key was added at creation)
	token1, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate token with initial key: %v", err)
	}

	// Verify token uses kid "1000"
	parsedToken1, _, _ := jwt5.NewParser().ParseUnverified(token1, &Claims{})
	if kid1, ok := parsedToken1.Header["kid"].(string); !ok || kid1 != "1000" {
		t.Errorf("Expected token to use kid '1000', got %v", parsedToken1.Header["kid"])
	}

	// Add a new key
	updatedKeys := map[string][]byte{
		"1000": []byte("initial-key-32-characters-long"),
		"2000": []byte("new-key-32-characters-long-here"),
	}
	if err := signer.UpdateKeys(updatedKeys, "2000"); err != nil {
		t.Fatalf("Failed to update keys: %v", err)
	}

	// Immediately try to generate token - should still use old key "1000" due to cooloff
	token2, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate token immediately after update: %v", err)
	}

	parsedToken2, _, _ := jwt5.NewParser().ParseUnverified(token2, &Claims{})
	if kid2, ok := parsedToken2.Header["kid"].(string); !ok || kid2 != "1000" {
		t.Errorf("Expected token to still use kid '1000' during cooloff, got %v", parsedToken2.Header["kid"])
	}

	// Wait for cooloff period to pass
	time.Sleep(2100 * time.Millisecond)

	// Now token should use the new key "2000"
	token3, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate token after cooloff: %v", err)
	}

	parsedToken3, _, _ := jwt5.NewParser().ParseUnverified(token3, &Claims{})
	if kid3, ok := parsedToken3.Header["kid"].(string); !ok || kid3 != "2000" {
		t.Errorf("Expected token to use kid '2000' after cooloff, got %v", parsedToken3.Header["kid"])
	}

	// Verify all tokens still validate
	if _, err := signer.ValidateToken(token1); err != nil {
		t.Errorf("Token1 should still be valid: %v", err)
	}
	if _, err := signer.ValidateToken(token2); err != nil {
		t.Errorf("Token2 should still be valid: %v", err)
	}
	if _, err := signer.ValidateToken(token3); err != nil {
		t.Errorf("Token3 should still be valid: %v", err)
	}
}

func TestStandardSigner_ConcurrentAccess(t *testing.T) {
	signingKeys := map[string][]byte{
		"1000": []byte("test-signing-key-32-characters-long"),
	}
	signer := NewStandardSigner("test-issuer", "test-audience", time.Hour, 0)
	_ = signer.UpdateKeys(signingKeys, "1000")

	// Generate initial token
	token, err := signer.GenerateToken(testUser, []string{}, "uid", nil, "", "", "")
	if err != nil {
		t.Fatalf("Failed to generate initial token: %v", err)
	}

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		// Concurrent token generation
		go func() {
			_, err := signer.GenerateToken("user", []string{}, "uid", nil, "", "", "")
			if err != nil {
				t.Errorf("Concurrent GenerateToken failed: %v", err)
			}
			done <- true
		}()

		// Concurrent token validation
		go func() {
			_, err := signer.ValidateToken(token)
			if err != nil {
				t.Errorf("Concurrent ValidateToken failed: %v", err)
			}
			done <- true
		}()

		// Concurrent key updates
		go func() {
			newKeys := map[string][]byte{
				"1000": []byte("test-signing-key-32-characters-long"),
				"2000": []byte("new-key-32-characters-long-here1"),
			}
			if err := signer.UpdateKeys(newKeys, "2000"); err != nil {
				t.Errorf("Failed to update keys: %v", err)
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 30; i++ {
		<-done
	}
}
