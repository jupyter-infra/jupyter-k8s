package authmiddleware

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	jwt5 "github.com/golang-jwt/jwt/v5"
)

// Test constants
const (
	testUserValue   = "test-user"
	testUIDValue    = "test-uid"
	testPathValue   = "/workspaces/ns1/app1"
	testDomainValue = "example.com"
)

// contains checks if a string is present in a slice
func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

// TestNewJWTManager verifies that the JWT manager is initialized with the correct values from the config
func TestNewJWTManager(t *testing.T) {
	// Create test config
	cfg := &Config{
		JWTSigningKey: "test-signing-key",
		JWTIssuer:     "test-issuer",
		JWTAudience:   "test-audience",
		JWTExpiration: 30 * time.Minute,
	}

	// Create a JWT manager
	manager := NewJWTManager(cfg)

	// Verify manager was initialized with correct values
	if string(manager.signingKey) != cfg.JWTSigningKey {
		t.Errorf("Expected signing key %q, got %q", cfg.JWTSigningKey, string(manager.signingKey))
	}

	if manager.issuer != cfg.JWTIssuer {
		t.Errorf("Expected issuer %q, got %q", cfg.JWTIssuer, manager.issuer)
	}

	if manager.audience != cfg.JWTAudience {
		t.Errorf("Expected audience %q, got %q", cfg.JWTAudience, manager.audience)
	}

	if manager.expiration != cfg.JWTExpiration {
		t.Errorf("Expected expiration %v, got %v", cfg.JWTExpiration, manager.expiration)
	}
}

// TestGenerateToken verifies that the token is generated correctly with the right claims
func TestGenerateToken(t *testing.T) {
	// Create test config
	cfg := &Config{
		JWTSigningKey: "test-signing-key",
		JWTIssuer:     "test-issuer",
		JWTAudience:   "test-audience",
		JWTExpiration: 30 * time.Minute,
	}

	// Create a JWT manager
	manager := NewJWTManager(cfg)

	// Test data
	testUser := testUserValue
	testGroups := []string{"group1", "group2"}
	testUID := testUIDValue
	testPath := testPathValue
	testDomain := testDomainValue

	// Generate token
	tokenString, err := manager.GenerateToken(testUser, testGroups, testUID, nil, testPath, testDomain, TokenTypeSession)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Parse token without validation to extract claims
	token, err := jwt5.ParseWithClaims(
		tokenString,
		&Claims{},
		func(t *jwt5.Token) (any, error) {
			return []byte(cfg.JWTSigningKey), nil
		},
	)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	// Extract claims
	claims, ok := token.Claims.(*Claims)
	if !ok {
		t.Fatalf("Failed to extract claims from token")
	}

	// Test that time values are present
	if claims.IssuedAt == nil {
		t.Fatalf("IssuedAt claim is nil")
	}
	issuedAt := claims.IssuedAt.Time

	// Check that time is approximately correct (within a few seconds)
	now := time.Now()
	if issuedAt.Before(now.Add(-5*time.Second)) || issuedAt.After(now.Add(5*time.Second)) {
		t.Errorf("IssuedAt not within reasonable range of current time: got %v, now is %v",
			issuedAt, now)
	}

	// Check expiry time
	if claims.ExpiresAt == nil {
		t.Fatalf("ExpiresAt claim is nil")
	}
	expiresAt := claims.ExpiresAt.Time
	expectedExpiry := issuedAt.Add(cfg.JWTExpiration)
	if !expiresAt.Equal(expectedExpiry) {
		t.Errorf("ExpiresAt incorrect: got %v, expected %v", expiresAt, expectedExpiry)
	}

	// Check NotBefore time
	if claims.NotBefore == nil {
		t.Fatalf("NotBefore claim is nil")
	}
	// NotBefore should equal IssuedAt
	nbf := claims.NotBefore.Time
	if !nbf.Equal(issuedAt) {
		t.Errorf("NotBefore incorrect: got %v, expected %v", nbf, issuedAt)
	}

	// Check standard claims
	if claims.Subject != testUser {
		t.Errorf("Subject incorrect: got %q, expected %q", claims.Subject, testUser)
	}

	if claims.Issuer != cfg.JWTIssuer {
		t.Errorf("Issuer incorrect: got %q, expected %q", claims.Issuer, cfg.JWTIssuer)
	}

	// Check audience (just check that it contains the expected audience)
	if !contains(claims.Audience, cfg.JWTAudience) {
		t.Errorf("Audience does not contain %q: got %v", cfg.JWTAudience, claims.Audience)
	}

	// Check custom claims
	if claims.User != testUser {
		t.Errorf("User claim incorrect: got %q, expected %q", claims.User, testUser)
	}

	if !reflect.DeepEqual(claims.Groups, testGroups) {
		t.Errorf("Groups claim incorrect: got %v, expected %v", claims.Groups, testGroups)
	}

	if claims.UID != testUID {
		t.Errorf("UID claim incorrect: got %q, expected %q", claims.UID, testUID)
	}

	if claims.Path != testPath {
		t.Errorf("Path claim incorrect: got %q, expected %q", claims.Path, testPath)
	}

	// Check domain claim
	if claims.Domain != testDomain {
		t.Errorf("Domain claim incorrect: got %q, expected %q", claims.Domain, testDomain)
	}
}

// TestGenerateTokenWithNilGroups verifies that the token generation works with nil groups
func TestGenerateTokenWithNilGroups(t *testing.T) {
	// Create test config
	cfg := &Config{
		JWTSigningKey: "test-signing-key",
		JWTIssuer:     "test-issuer",
		JWTAudience:   "test-audience",
		JWTExpiration: 30 * time.Minute,
	}

	// Create a JWT manager
	manager := NewJWTManager(cfg)

	// Test data
	testUser := testUserValue
	var testGroups []string // Nil by default
	testUID := testUIDValue
	testPath := testPathValue
	testDomain := testDomainValue

	// Generate token
	tokenString, err := manager.GenerateToken(testUser, testGroups, testUID, nil, testPath, testDomain, TokenTypeSession)
	if err != nil {
		t.Fatalf("Failed to generate token with nil groups: %v", err)
	}

	// Parse token
	token, err := jwt5.ParseWithClaims(
		tokenString,
		&Claims{},
		func(t *jwt5.Token) (any, error) {
			return []byte(cfg.JWTSigningKey), nil
		},
	)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	// Extract claims
	claims, ok := token.Claims.(*Claims)
	if !ok {
		t.Fatalf("Failed to extract claims from token")
	}

	// Check that groups is empty
	if len(claims.Groups) != 0 {
		t.Errorf("Expected empty groups, got %v", claims.Groups)
	}
}

// TestGenerateTokenWithEmptyGroups verifies that the token generation works with empty groups slice
func TestGenerateTokenWithEmptyGroups(t *testing.T) {
	// Create test config
	cfg := &Config{
		JWTSigningKey: "test-signing-key",
		JWTIssuer:     "test-issuer",
		JWTAudience:   "test-audience",
		JWTExpiration: 30 * time.Minute,
	}

	// Create a JWT manager
	manager := NewJWTManager(cfg)

	// Test data
	testUser := testUserValue
	testGroups := []string{} // Empty slice
	testUID := testUIDValue
	testPath := testPathValue
	testDomain := testDomainValue

	// Generate token
	tokenString, err := manager.GenerateToken(testUser, testGroups, testUID, nil, testPath, testDomain, TokenTypeSession)
	if err != nil {
		t.Fatalf("Failed to generate token with empty groups: %v", err)
	}

	// Parse token
	token, err := jwt5.ParseWithClaims(
		tokenString,
		&Claims{},
		func(t *jwt5.Token) (any, error) {
			return []byte(cfg.JWTSigningKey), nil
		},
	)
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	// Extract claims
	claims, ok := token.Claims.(*Claims)
	if !ok {
		t.Fatalf("Failed to extract claims from token")
	}

	// Check that the groups length is 0
	if len(claims.Groups) != 0 {
		t.Errorf("Expected empty groups slice, got %v with length %d", claims.Groups, len(claims.Groups))
	}
}

// TestValidateToken verifies that token validation works correctly
func TestValidateToken(t *testing.T) {
	// Create test config
	cfg := &Config{
		JWTSigningKey: "test-signing-key",
		JWTIssuer:     "test-issuer",
		JWTAudience:   "test-audience",
		JWTExpiration: 30 * time.Minute,
	}

	// Create a JWT manager
	manager := NewJWTManager(cfg)

	// Test data
	testUser := testUserValue
	testGroups := []string{"group1", "group2"}
	testUID := testUIDValue
	testPath := testPathValue
	testDomain := testDomainValue

	// Generate valid token
	validToken, err := manager.GenerateToken(testUser, testGroups, testUID, nil, testPath, testDomain, TokenTypeSession)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Verify valid token
	claims, err := manager.ValidateToken(validToken)
	if err != nil {
		t.Errorf("Failed to validate valid token: %v", err)
	}
	if claims == nil {
		t.Fatal("ValidateToken returned nil claims for valid token")
		return
	}

	// Check that the claims match what we expect
	if claims.User != testUser {
		t.Errorf("Expected user %q, got %q", testUser, claims.User)
	}
	if !reflect.DeepEqual(claims.Groups, testGroups) {
		t.Errorf("Expected groups %v, got %v", testGroups, claims.Groups)
	}
	if claims.Path != testPath {
		t.Errorf("Expected path %q, got %q", testPath, claims.Path)
	}
	if claims.Domain != testDomain {
		t.Errorf("Expected domain %q, got %q", testDomain, claims.Domain)
	}

	// Test with wrong audience
	wrongAudienceCfg := &Config{
		JWTSigningKey: "test-signing-key",
		JWTIssuer:     "test-issuer",
		JWTAudience:   "wrong-audience",
		JWTExpiration: 30 * time.Minute,
	}
	wrongAudienceManager := NewJWTManager(wrongAudienceCfg)
	_, err = wrongAudienceManager.ValidateToken(validToken)
	if err == nil {
		t.Error("Expected error for token with wrong audience, got nil")
	}

	// Test with wrong issuer
	wrongIssuerCfg := &Config{
		JWTSigningKey: "test-signing-key",
		JWTIssuer:     "wrong-issuer",
		JWTAudience:   "test-audience",
		JWTExpiration: 30 * time.Minute,
	}
	wrongIssuerManager := NewJWTManager(wrongIssuerCfg)
	_, err = wrongIssuerManager.ValidateToken(validToken)
	if err == nil {
		t.Error("Expected error for token with wrong issuer, got nil")
	}

	// Test with wrong signing key
	wrongKeyManager := NewJWTManager(&Config{
		JWTSigningKey: "wrong-signing-key",
		JWTIssuer:     "test-issuer",
		JWTAudience:   "test-audience",
		JWTExpiration: 30 * time.Minute,
	})
	_, err = wrongKeyManager.ValidateToken(validToken)
	if err == nil {
		t.Error("Expected error for token with wrong signing key, got nil")
	}
}

// TestRefreshToken verifies that token refresh works correctly
func TestRefreshToken(t *testing.T) {
	// Create test config with a short expiration for testing
	cfg := &Config{
		JWTSigningKey: "test-signing-key",
		JWTIssuer:     "test-issuer",
		JWTAudience:   "test-audience",
		JWTExpiration: 30 * time.Minute,
	}

	// Create a JWT manager
	manager := NewJWTManager(cfg)

	// Test data
	testUser := testUserValue
	testGroups := []string{"group1", "group2"}
	testUID := testUIDValue
	testPath := testPathValue
	testDomain := testDomainValue

	// Generate original token
	originalToken, err := manager.GenerateToken(testUser, testGroups, testUID, nil, testPath, testDomain, TokenTypeSession)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Parse original token to get claims
	originalClaims, err := manager.ValidateToken(originalToken)
	if err != nil {
		t.Fatalf("Failed to validate original token: %v", err)
	}

	// Refresh token using claims directly
	refreshedToken, err := manager.RefreshToken(originalClaims)
	if err != nil {
		t.Fatalf("Failed to refresh token: %v", err)
	}

	// Parse refreshed token
	refreshedClaims, err := manager.ValidateToken(refreshedToken)
	if err != nil {
		t.Fatalf("Failed to validate refreshed token: %v", err)
	}

	// Check that time fields were properly updated
	if refreshedClaims.IssuedAt == nil {
		t.Fatalf("IssuedAt claim is nil in refreshed token")
	}
	issuedAt := refreshedClaims.IssuedAt.Time

	// Check that time is approximately correct (within a few seconds)
	now := time.Now()
	if issuedAt.Before(now.Add(-5*time.Second)) || issuedAt.After(now.Add(5*time.Second)) {
		t.Errorf("IssuedAt not within reasonable range of current time: got %v, now is %v",
			issuedAt, now)
	}

	// Check that expiry was extended
	if refreshedClaims.ExpiresAt == nil {
		t.Fatalf("ExpiresAt claim is nil in refreshed token")
	}

	// Not using original expiry anymore, but ensure it exists
	if originalClaims.ExpiresAt == nil {
		t.Fatalf("Original ExpiresAt claim is nil")
	}

	// New expiry
	refreshedExpiry := refreshedClaims.ExpiresAt.Time

	// New expiry should be approximately issuedAt + expiration
	expectedExpiry := issuedAt.Add(cfg.JWTExpiration)
	allowedDelta := 2 * time.Second // Allow for slight differences due to timing
	if refreshedExpiry.Before(expectedExpiry.Add(-allowedDelta)) || refreshedExpiry.After(expectedExpiry.Add(allowedDelta)) {
		t.Errorf("Refreshed token expiry not as expected: got %v, expected about %v (± %v)",
			refreshedExpiry, expectedExpiry, allowedDelta)
	}

	// Check that user data was preserved
	if refreshedClaims.User != testUser {
		t.Errorf("User claim changed: got %q, expected %q", refreshedClaims.User, testUser)
	}

	if !reflect.DeepEqual(refreshedClaims.Groups, testGroups) {
		t.Errorf("Groups claim changed: got %v, expected %v", refreshedClaims.Groups, testGroups)
	}

	if refreshedClaims.UID != testUID {
		t.Errorf("UID claim changed: got %q, expected %q", refreshedClaims.UID, testUID)
	}

	if refreshedClaims.Path != testPath {
		t.Errorf("Path claim changed: got %q, expected %q", refreshedClaims.Path, testPath)
	}

	if refreshedClaims.Domain != testDomain {
		t.Errorf("Domain claim changed: got %q, expected %q", refreshedClaims.Domain, testDomain)
	}
}

// TestUpdateSkipRefreshToken_HappyCase tests the logic of flipping the skip flag to false
func TestUpdateSkipRefreshToken_HappyCase(t *testing.T) {
	// Create test config
	cfg := &Config{
		JWTSigningKey: "test-signing-key",
		JWTIssuer:     "test-issuer",
		JWTAudience:   "test-audience",
		JWTExpiration: 30 * time.Minute,
	}

	// Create a JWT manager
	manager := NewJWTManager(cfg)

	// Test data
	testUser := testUserValue
	testGroups := []string{"group1", "group2"}
	testUID := testUIDValue
	testPath := testPathValue
	testDomain := testDomainValue

	// Generate token with SkipRefresh = true
	originalToken, err := manager.GenerateToken(testUser, testGroups, testUID, nil, testPath, testDomain, TokenTypeSession)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Parse original token to get claims
	originalClaims, err := manager.ValidateToken(originalToken)
	if err != nil {
		t.Fatalf("Failed to validate original token: %v", err)
	}

	if originalClaims.SkipRefresh {
		t.Error("originalClaims.SkipRefresh should not already be true")
	}

	// Update the token with SkipRefresh = true
	updatedToken, err := manager.UpdateSkipRefreshToken(originalClaims)
	if err != nil {
		t.Fatalf("Failed to update token: %v", err)
	}

	// Parse updated token
	updatedClaims, err := manager.ValidateToken(updatedToken)
	if err != nil {
		t.Fatalf("Failed to validate updated token: %v", err)
	}

	// Check that SkipRefresh was set to false
	if !updatedClaims.SkipRefresh {
		t.Error("SkipRefresh flag was not set to false")
	}

	// Check that other claims remain unchanged
	if updatedClaims.User != testUser {
		t.Errorf("User claim changed: got %q, expected %q", updatedClaims.User, testUser)
	}

	if !reflect.DeepEqual(updatedClaims.Groups, testGroups) {
		t.Errorf("Groups claim changed: got %v, expected %v", updatedClaims.Groups, testGroups)
	}

	if updatedClaims.UID != testUID {
		t.Errorf("UID claim changed: got %q, expected %q", updatedClaims.UID, testUID)
	}

	if updatedClaims.Path != testPath {
		t.Errorf("Path claim changed: got %q, expected %q", updatedClaims.Path, testPath)
	}

	if updatedClaims.Domain != testDomain {
		t.Errorf("Domain claim changed: got %q, expected %q", updatedClaims.Domain, testDomain)
	}
}

// TestUpdateSkipRefreshToken_FailCase tests the logic when the claims are invalid
func TestUpdateSkipRefreshToken_FailCase(t *testing.T) {
	// Create test config
	cfg := &Config{
		JWTSigningKey: "test-signing-key",
		JWTIssuer:     "test-issuer",
		JWTAudience:   "test-audience",
		JWTExpiration: 30 * time.Minute,
	}

	// Create a JWT manager
	manager := NewJWTManager(cfg)

	// Test with nil claims
	token, err := manager.UpdateSkipRefreshToken(nil)
	if err == nil {
		t.Error("Expected error with nil claims, got nil")
	}
	if token != "" {
		t.Errorf("Expected empty token with nil claims, got: %v", token)
	}
}

// TestShouldRefreshToken tests the logic for determining when a token should be refreshed
func TestShouldRefreshToken(t *testing.T) {
	// Set default refresh window for all tests
	const refreshWindow = 15 * time.Minute
	const refreshHorizon = 12 * time.Hour

	// Get current time for test cases
	now := time.Now().UTC()

	// Create test cases
	tests := []struct {
		name   string
		claims *Claims
		want   bool
	}{
		{
			name:   "nil claims",
			claims: nil,
			want:   false,
		},
		{
			name: "nil ExpiresAt",
			claims: &Claims{
				RegisteredClaims: jwt5.RegisteredClaims{
					ExpiresAt: nil,
				},
			},
			want: false,
		},
		{
			name: "expired token",
			claims: &Claims{
				RegisteredClaims: jwt5.RegisteredClaims{
					IssuedAt:  jwt5.NewNumericDate(now.Add(-6 * time.Hour)),
					ExpiresAt: jwt5.NewNumericDate(now.Add(-10 * time.Minute)),
				},
			},
			want: false,
		},
		{
			name: "token not in refresh window",
			claims: &Claims{
				RegisteredClaims: jwt5.RegisteredClaims{
					IssuedAt:  jwt5.NewNumericDate(now.Add(-6 * time.Hour)),
					ExpiresAt: jwt5.NewNumericDate(now.Add(20 * time.Minute)),
				},
			},
			want: false,
		},
		{
			name: "token within refresh window",
			claims: &Claims{
				RegisteredClaims: jwt5.RegisteredClaims{
					IssuedAt:  jwt5.NewNumericDate(now.Add(-6 * time.Hour)),
					ExpiresAt: jwt5.NewNumericDate(now.Add(10 * time.Minute)),
				},
			},
			want: true,
		},
		{
			name: "token at edge of refresh window",
			claims: &Claims{
				RegisteredClaims: jwt5.RegisteredClaims{
					IssuedAt:  jwt5.NewNumericDate(now.Add(-6 * time.Hour)),
					ExpiresAt: jwt5.NewNumericDate(now.Add(15 * time.Minute)),
				},
			},
			want: true,
		},
		{
			name: "token in refresh window but beyond refresh horizon",
			claims: &Claims{
				RegisteredClaims: jwt5.RegisteredClaims{
					IssuedAt:  jwt5.NewNumericDate(now.Add(-24 * time.Hour)),
					ExpiresAt: jwt5.NewNumericDate(now.Add(5 * time.Minute)),
				},
			},
			want: false,
		},
		{
			name: "token within refresh window but with skip flag",
			claims: &Claims{
				RegisteredClaims: jwt5.RegisteredClaims{
					IssuedAt:  jwt5.NewNumericDate(now.Add(-6 * time.Hour)),
					ExpiresAt: jwt5.NewNumericDate(now.Add(10 * time.Minute)),
				},
				SkipRefresh: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a JWT manager with the configured refresh window
			cfg := &Config{
				JWTSigningKey:     "test-key",
				JWTIssuer:         "test-issuer",
				JWTAudience:       "test-audience",
				JWTExpiration:     60 * time.Minute,
				JWTRefreshEnable:  true,
				JWTRefreshWindow:  refreshWindow,
				JWTRefreshHorizon: refreshHorizon,
			}
			jwtManager := NewJWTManager(cfg)

			got := jwtManager.ShouldRefreshToken(tt.claims)
			if got != tt.want {
				t.Errorf("ShouldRefreshToken() = %v, want %v", got, tt.want)
			}
		})
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("norefresh-%s", tt.name), func(t *testing.T) {
			// Create a JWT manager with the configured refresh window
			cfg := &Config{
				JWTSigningKey:     "test-key",
				JWTIssuer:         "test-issuer",
				JWTAudience:       "test-audience",
				JWTExpiration:     60 * time.Minute,
				JWTRefreshEnable:  false,
				JWTRefreshWindow:  refreshWindow,
				JWTRefreshHorizon: refreshHorizon,
			}
			jwtManager := NewJWTManager(cfg)

			got := jwtManager.ShouldRefreshToken(tt.claims)
			if got != false {
				t.Errorf("ShouldRefreshToken() should be false when config.JWTRefreshEnable is false.")
			}
		})
	}
}

// TestShouldRefreshTokenWithDifferentWindows tests the refresh logic with different refresh windows
func TestShouldRefreshTokenWithDifferentWindows(t *testing.T) {
	// Get current time for test cases
	now := time.Now().UTC()

	// Create a token that expires in 30 minutes
	claims := &Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			IssuedAt:  jwt5.NewNumericDate(now.Add(-120 * time.Minute)),
			ExpiresAt: jwt5.NewNumericDate(now.Add(30 * time.Minute)),
		},
		SkipRefresh: false,
	}

	// Test with different refresh windows
	testCases := []struct {
		refreshWindow  time.Duration
		refreshHorizon time.Duration
		want           bool
	}{
		{5 * time.Minute, 12 * time.Hour, false},  // 5 minute window, 12 hours horizon, token not due for refresh
		{25 * time.Minute, 12 * time.Hour, false}, // 25 minute window, 12 hours horizon, token not due for refresh
		{30 * time.Minute, 12 * time.Hour, true},  // 30 minute window, 12 hours horizon, token exactly at the refresh boundary
		{35 * time.Minute, 12 * time.Hour, true},  // 35 minute window, 12 hours horizon, token within refresh window
		{60 * time.Minute, 12 * time.Hour, true},  // Full expiration time, 12 hours horizon, token definitely within window
		{35 * time.Minute, 1 * time.Hour, false},  // 35 minute window, 1 hour horizon, should not refresh
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("refresh window %v", tc.refreshWindow), func(t *testing.T) {
			// Create a JWT manager with this specific refresh window
			cfg := &Config{
				JWTSigningKey:     "test-key",
				JWTIssuer:         "test-issuer",
				JWTAudience:       "test-audience",
				JWTExpiration:     60 * time.Minute,
				JWTRefreshEnable:  true,
				JWTRefreshWindow:  tc.refreshWindow,
				JWTRefreshHorizon: tc.refreshHorizon,
			}
			jwtManager := NewJWTManager(cfg)

			got := jwtManager.ShouldRefreshToken(claims)
			if got != tc.want {
				t.Errorf("Test case %d: ShouldRefreshToken() with %v window = %v, want %v",
					i, tc.refreshWindow, got, tc.want)
			}
		})
	}
}
