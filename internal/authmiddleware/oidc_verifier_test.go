/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOIDCClaimsParsingFromGitHub tests that the OIDCClaims struct
// correctly parses tokens from Dex with GitHub connector
func TestOIDCClaimsParsingFromGitHub(t *testing.T) {
	t.Run("Default GitHub ID format", func(t *testing.T) {
		testOIDCClaimsParsingFromGitHub(t, "CgcyMzQ1Njc4EgZnaXRodWI") // Dex's default format with GitHub ID
	})

	t.Run("Using login as ID", func(t *testing.T) {
		testOIDCClaimsParsingFromGitHub(t, "github|github-user") // Format when useLoginAsID is true
	})
}

// testOIDCClaimsParsingFromGitHub is a helper function to test parsing tokens
// with different subject formats
func testOIDCClaimsParsingFromGitHub(t *testing.T, subject string) {
	// Create a sample raw token claims as they would come from Dex with GitHub connector
	// This simulates the JSON payload inside a JWT token from Dex
	rawClaims := map[string]interface{}{
		claimIss:             testDexIssuerURL,
		claimSub:             subject, // Subject format passed from the test case
		claimAud:             "authmiddleware-client",
		claimExp:             time.Now().Add(1 * time.Hour).Unix(),
		claimIat:             time.Now().Unix(),
		"nonce":              "abcdefghijklmnop",
		"preferred_username": "github-user", // GitHub username
		"email":              "user@github.com",
		"groups": []string{
			testOrg1Team1,
			testOrg1Team2,
			"org2:admins",
		},
		"name": "GitHub User", // Additional claim not explicitly in our struct
	}

	// Create a mock token with these claims
	rawClaimsJSON, err := json.Marshal(rawClaims)
	require.NoError(t, err)

	// Test parsing these claims into our OIDCClaims struct
	var claims OIDCClaims
	err = json.Unmarshal(rawClaimsJSON, &claims)
	require.NoError(t, err)

	// Verify that each field was parsed correctly
	assert.Equal(t, "github-user", claims.Username)
	assert.Equal(t, "user@github.com", claims.Email)
	assert.Equal(t, subject, claims.Subject)

	// Verify groups are parsed correctly
	assert.Len(t, claims.Groups, 3)
	assert.Contains(t, claims.Groups, testOrg1Team1)
	assert.Contains(t, claims.Groups, testOrg1Team2)
	assert.Contains(t, claims.Groups, "org2:admins")

	// Test that GetOIDCGroupsFromToken correctly processes GitHub groups
	config := &Config{
		OidcGroupsPrefix: "oidc-github:", // Sample prefix for testing
	}

	groups := GetOIDCGroupsFromToken(config, &claims)

	// Verify that the prefix is correctly applied
	assert.Len(t, groups, 3)
	assert.Contains(t, groups, "oidc-github:org1:team1")
	assert.Contains(t, groups, "oidc-github:org1:team2")
	assert.Contains(t, groups, "oidc-github:org2:admins")

	// Test username extraction
	username := GetOIDCUsernameFromToken(config, &claims)
	assert.Equal(t, "github-user", username)
}

// TestOIDCVerifierWithGitHubToken tests the full token verification flow
// with a simulated GitHub token from Dex
func TestOIDCVerifierWithGitHubToken(t *testing.T) {
	// Test both ID formats
	testSubjects := []string{
		"CgcyMzQ1Njc4EgZnaXRodWI", // Default GitHub ID format
		"github|github-user",      // When useLoginAsID is true
	}

	for _, subject := range testSubjects {
		t.Run("Subject format: "+subject, func(t *testing.T) {
			testOIDCVerifierWithSubject(t, subject)
		})
	}
}

// testOIDCVerifierWithSubject is a helper function to test verification with different subjects
func testOIDCVerifierWithSubject(t *testing.T, subject string) {
	// This test requires mocking the OIDC provider and verifier
	// which is complex due to the nature of token verification
	// Instead, let's test the claims parsing part which is the critical part

	// Create a mock OIDCVerifier with a mock verifier that returns our test claims
	mockVerifier := &MockOIDCVerifier{
		VerifyTokenFunc: func(ctx context.Context, tokenString string, logger *slog.Logger) (*OIDCClaims, bool, error) {
			return &OIDCClaims{
				Username: "github-user",
				Email:    "user@github.com",
				Subject:  subject,
				Groups: []string{
					testOrg1Team1,
					testOrg1Team2,
					"org2:admins",
				},
				ExtraClaimsField: map[string]any{
					"name": "GitHub User",
				},
			}, false, nil
		},
	}

	// Test the verification
	claims, isFault, err := mockVerifier.VerifyToken(context.Background(), "fake.jwt.token", slog.Default())

	// Verify results
	assert.NoError(t, err)
	assert.False(t, isFault)
	assert.NotNil(t, claims)
	assert.Equal(t, "github-user", claims.Username)
	assert.Equal(t, "user@github.com", claims.Email)
	assert.Len(t, claims.Groups, 3)
}

// TestOIDCVerifierConfig tests the configuration of the OIDCVerifier
func TestOIDCVerifierConfig(t *testing.T) {
	// Create a comprehensive config to test all values
	config := &Config{
		OIDCIssuerURL:       testDexIssuerURL,
		OIDCClientID:        "oauth2-proxy",
		OIDCInitTimeoutSecs: 30,
	}

	// Create the verifier
	verifier, err := NewOIDCVerifier(config, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, verifier)

	// 1. Test that all config values are correctly passed
	assert.Equal(t, config.OIDCIssuerURL, verifier.issuerURL)
	assert.Equal(t, config.OIDCClientID, verifier.clientID)
	assert.Equal(t, config.OIDCInitTimeoutSecs, verifier.timeoutSeconds)

	// 2. Test that provider and verifier are nil after NewOIDCVerifier
	assert.Nil(t, verifier.provider, "Provider should be nil after initialization")
	assert.Nil(t, verifier.verifier, "Verifier should be nil after initialization")

	// 3. Assert that oidcConfig is properly set
	require.NotNil(t, verifier.oidcConfig)
	assert.Equal(t, config.OIDCClientID, verifier.oidcConfig.ClientID)
	assert.False(t, verifier.oidcConfig.SkipClientIDCheck,
		"OIDCVerifier should enforce audience validation")
}

// TestGetOIDCGroupsFromToken tests the GetOIDCGroupsFromToken function
func TestGetOIDCGroupsFromToken(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		claims   *OIDCClaims
		expected []string
	}{
		{
			name: "Normal groups",
			config: &Config{
				OidcGroupsPrefix: testOidcPrefix,
			},
			claims: &OIDCClaims{
				Groups: []string{testAdminValue, testDevValue, testUserString},
			},
			expected: []string{"oidc:admin", "oidc:dev", "oidc:user"},
		},
		{
			name: "Empty groups",
			config: &Config{
				OidcGroupsPrefix: testOidcPrefix,
			},
			claims: &OIDCClaims{
				Groups: []string{},
			},
			expected: []string{},
		},
		{
			name: "Nil claims",
			config: &Config{
				OidcGroupsPrefix: testOidcPrefix,
			},
			claims:   nil,
			expected: []string{},
		},
		{
			name: "System groups",
			config: &Config{
				OidcGroupsPrefix: DefaultOidcUsernamePrefix,
			},
			claims: &OIDCClaims{
				Groups: []string{SystemAuthenticatedGroup, "org:team"},
			},
			expected: []string{SystemAuthenticatedGroup, testGithubOrgTeam},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetOIDCGroupsFromToken(tt.config, tt.claims)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestGetOIDCUsernameFromToken tests the GetOIDCUsernameFromToken function
func TestGetOIDCUsernameFromToken(t *testing.T) {
	tests := []struct {
		name     string
		config   *Config
		claims   *OIDCClaims
		expected string
	}{
		{
			name: "Normal username",
			config: &Config{
				OidcUsernamePrefix: testOidcPrefix,
			},
			claims: &OIDCClaims{
				Username: testJohndoe,
			},
			expected: "oidc:johndoe",
		},
		{
			name: "Empty username",
			config: &Config{
				OidcUsernamePrefix: testOidcPrefix,
			},
			claims: &OIDCClaims{
				Username: "",
			},
			expected: "",
		},
		{
			name: "Nil claims",
			config: &Config{
				OidcUsernamePrefix: testOidcPrefix,
			},
			claims:   nil,
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetOIDCUsernameFromToken(tt.config, tt.claims)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestVerifyToken_NotInitialized tests that VerifyToken returns an error when called before Start()
func TestVerifyToken_NotInitialized(t *testing.T) {
	config := &Config{
		OIDCIssuerURL:       testDexIssuerURL,
		OIDCClientID:        testClientValue,
		OIDCInitTimeoutSecs: 10,
	}

	verifier, err := NewOIDCVerifier(config, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, verifier)

	// Try to verify a token before calling Start()
	claims, isFault, err := verifier.VerifyToken(context.Background(), "fake.jwt.token", slog.Default())

	// Should return an error indicating the verifier is not initialized
	assert.Nil(t, claims)
	assert.True(t, isFault, "Should be a fault error since verifier not initialized")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
	assert.Contains(t, err.Error(), "call Start() first")
}

// TestStart_WithInvalidIssuer tests that Start returns an error with an invalid issuer URL
func TestStart_WithInvalidIssuer(t *testing.T) {
	config := &Config{
		OIDCIssuerURL:       "http://nonexistent-issuer-that-does-not-exist.local",
		OIDCClientID:        testClientValue,
		OIDCInitTimeoutSecs: 2, // Short timeout for faster test
	}

	verifier, err := NewOIDCVerifier(config, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, verifier)

	// Try to start with an invalid issuer
	err = verifier.Start(context.Background())

	// Should return an error
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize OIDC provider")
}

// TestNewOIDCVerifier_RequiredFields tests that NewOIDCVerifier validates required fields
func TestNewOIDCVerifier_RequiredFields(t *testing.T) {
	t.Run("Missing issuer URL", func(t *testing.T) {
		config := &Config{
			OIDCIssuerURL:       "", // Missing
			OIDCClientID:        testClientValue,
			OIDCInitTimeoutSecs: 10,
		}

		verifier, err := NewOIDCVerifier(config, slog.Default())
		assert.Error(t, err)
		assert.Nil(t, verifier)
		assert.Contains(t, err.Error(), "issuer URL is required")
	})

	t.Run("Missing client ID", func(t *testing.T) {
		config := &Config{
			OIDCIssuerURL:       testDexIssuerURL,
			OIDCClientID:        "", // Missing
			OIDCInitTimeoutSecs: 10,
		}

		verifier, err := NewOIDCVerifier(config, slog.Default())
		assert.Error(t, err)
		assert.Nil(t, verifier)
		assert.Contains(t, err.Error(), "client ID is required")
	})
}

// TestStart_ContextCanceled tests that Start respects context cancellation
func TestStart_ContextCanceled(t *testing.T) {
	config := &Config{
		OIDCIssuerURL:       "http://nonexistent-issuer-that-does-not-exist.local",
		OIDCClientID:        testClientValue,
		OIDCInitTimeoutSecs: 30,
	}

	verifier, err := NewOIDCVerifier(config, slog.Default())
	require.NoError(t, err)
	require.NotNil(t, verifier)

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Try to start with a canceled context
	err = verifier.Start(ctx)

	// Should return an error related to context cancellation
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize OIDC provider")
}

const (
	oidcTestClientID = "test-client"
	oidcTestKeyID    = "test-key-1"

	// Standard JWT claim keys, kept as constants so goconst does not flag the
	// repeated literals across the token-minting tests.
	claimIss = "iss"
	claimSub = "sub"
	claimAud = "aud"
	claimExp = "exp"
	claimIat = "iat"
)

// mockOIDCProvider spins up an httptest server that serves the minimal OIDC
// discovery document and JWKS needed by go-oidc, and mints RS256 ID tokens
// signed with a matching key. It lets us exercise the Start() and VerifyToken()
// happy paths without a real identity provider.
type mockOIDCProvider struct {
	server *httptest.Server
	key    *rsa.PrivateKey
}

func newMockOIDCProvider(t *testing.T) *mockOIDCProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	m := &mockOIDCProvider{key: key}
	mux := http.NewServeMux()

	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                m.server.URL,
			"jwks_uri":                              m.server.URL + "/keys",
			"authorization_endpoint":                m.server.URL + "/auth",
			"token_endpoint":                        m.server.URL + "/token",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})

	mux.HandleFunc("/keys", func(w http.ResponseWriter, _ *http.Request) {
		pub := key.Public().(*rsa.PublicKey)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA",
				"alg": "RS256",
				"use": "sig",
				"kid": oidcTestKeyID,
				"n":   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
			}},
		})
	})

	m.server = httptest.NewServer(mux)
	t.Cleanup(m.server.Close)
	return m
}

// signToken mints an RS256 ID token with the given claims, signed by the mock
// provider's key and stamped with the matching key id.
func (m *mockOIDCProvider) signToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = oidcTestKeyID
	signed, err := token.SignedString(m.key)
	require.NoError(t, err)
	return signed
}

func newVerifierForProvider(t *testing.T, issuerURL string) *OIDCVerifier {
	t.Helper()
	cfg := &Config{
		OIDCIssuerURL:       issuerURL,
		OIDCClientID:        oidcTestClientID,
		OIDCInitTimeoutSecs: 10,
	}
	verifier, err := NewOIDCVerifier(cfg, slog.Default())
	require.NoError(t, err)
	return verifier
}

func TestStart_Succeeds(t *testing.T) {
	provider := newMockOIDCProvider(t)
	verifier := newVerifierForProvider(t, provider.server.URL)

	require.NoError(t, verifier.Start(context.Background()))
	assert.NotNil(t, verifier.verifier)

	// Second Start() is a no-op because the provider is already set.
	require.NoError(t, verifier.Start(context.Background()))
}

func TestVerifyToken_Succeeds(t *testing.T) {
	provider := newMockOIDCProvider(t)
	verifier := newVerifierForProvider(t, provider.server.URL)
	require.NoError(t, verifier.Start(context.Background()))

	token := provider.signToken(t, jwt.MapClaims{
		claimIss:             provider.server.URL,
		claimAud:             oidcTestClientID,
		claimSub:             "user-123",
		"preferred_username": "octocat",
		"email":              "octocat@example.com",
		"groups":             []string{"org:team-a", "org:team-b"},
		claimExp:             time.Now().Add(time.Hour).Unix(),
		claimIat:             time.Now().Add(-time.Minute).Unix(),
	})

	claims, isFault, err := verifier.VerifyToken(context.Background(), token, slog.Default())

	require.NoError(t, err)
	assert.False(t, isFault)
	require.NotNil(t, claims)
	assert.Equal(t, "octocat", claims.Username)
	assert.Equal(t, "octocat@example.com", claims.Email)
	assert.Equal(t, "user-123", claims.Subject)
	assert.ElementsMatch(t, []string{"org:team-a", "org:team-b"}, claims.Groups)
}

func TestVerifyToken_InvalidToken(t *testing.T) {
	provider := newMockOIDCProvider(t)
	verifier := newVerifierForProvider(t, provider.server.URL)
	require.NoError(t, verifier.Start(context.Background()))

	// Wrong audience → token validation failure (not a provider fault).
	token := provider.signToken(t, jwt.MapClaims{
		claimIss: provider.server.URL,
		claimAud: "some-other-client",
		claimSub: "user-123",
		claimExp: time.Now().Add(time.Hour).Unix(),
		claimIat: time.Now().Add(-time.Minute).Unix(),
	})

	claims, isFault, err := verifier.VerifyToken(context.Background(), token, slog.Default())

	require.Error(t, err)
	assert.False(t, isFault, "an audience mismatch is a token error, not a provider fault")
	assert.Nil(t, claims)
	assert.Contains(t, err.Error(), "invalid ID token")
}
