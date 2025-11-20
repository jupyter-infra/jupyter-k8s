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

package authmiddleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
)

// No constants needed here

// OIDCVerifierInterface defines the interface for OIDC token verification
type OIDCVerifierInterface interface {
	// VerifyToken verifies an OIDC token and returns Claims, isFault, error.
	// It may call the provider to refresh the public keySet if not cached
	VerifyToken(ctx context.Context, tokenString string, logger *slog.Logger) (*OIDCClaims, bool, error)

	// Start initializes the OIDC provider and verifier
	// This allows deferring HTTP calls until the application is ready
	Start(ctx context.Context) error
}

// OIDCVerifier handles verification of OIDC tokens
type OIDCVerifier struct {
	provider       *oidc.Provider
	verifier       *oidc.IDTokenVerifier
	clientID       string
	issuerURL      string
	logger         *slog.Logger
	timeoutSeconds int // Timeout for OIDC provider initialization
	oidcConfig     *oidc.Config
}

// OIDCClaims represents the claims we extract from an OIDC ID token
type OIDCClaims struct {
	Username         string   `json:"preferred_username"`
	Email            string   `json:"email"`
	Groups           []string `json:"groups"`
	Subject          string   `json:"sub"`
	ExtraClaimsField map[string]any
}

// NewOIDCVerifier creates a new OIDC verifier without initializing connections
// The actual initialization is deferred to the Start method
func NewOIDCVerifier(config *Config, logger *slog.Logger) (*OIDCVerifier, error) {
	if config.OIDCIssuerURL == "" {
		return nil, fmt.Errorf("OIDC issuer URL is required")
	}

	if config.OIDCClientID == "" {
		return nil, fmt.Errorf("OIDC client ID is required")
	}

	logger.Info("Creating OIDC verifier (not initialized)",
		"issuer", config.OIDCIssuerURL,
		"client_id", config.OIDCClientID,
		"timeout_secs", config.OIDCInitTimeoutSecs,
	)

	// Create the token verifier
	// Note that any caller may obtain the public keys from the OIDC well-known page, a client ID
	// and client secret are only needed to issue new token (and secret are optional for public
	// access points).
	// In authmiddleware flow, the OIDC provider will issue token to a different component (e.g. oauth2-proxy)
	// and such component will pass the token as Authentication token to authmiddleware,
	// thus we use the client ID of the previous component as client.
	oidcConfig := &oidc.Config{
		ClientID:          config.OIDCClientID,
		SkipClientIDCheck: false,
	}

	return &OIDCVerifier{
		provider:       nil, // Will be initialized in Start()
		verifier:       nil, // Will be initialized in Start()
		clientID:       config.OIDCClientID,
		issuerURL:      config.OIDCIssuerURL,
		logger:         logger,
		timeoutSeconds: config.OIDCInitTimeoutSecs,
		oidcConfig:     oidcConfig,
	}, nil
}

// Start initializes the OIDC provider and verifier
// This allows deferring HTTP calls until the application is ready
func (v *OIDCVerifier) Start(ctx context.Context) error {
	if v.provider != nil {
		// Already initialized
		return nil
	}

	v.logger.Info("Starting OIDC verifier - initializing provider connection",
		"issuer", v.issuerURL,
		"client_id", v.clientID,
	)

	// Create a context with timeout for OIDC provider initialization
	initCtx, cancel := context.WithTimeout(ctx, time.Duration(v.timeoutSeconds)*time.Second)
	defer cancel()

	// Initialize OIDC provider and verifier - this makes HTTP calls
	if v.logger != nil {
		v.logger.Info("Configuring new OIDC provider", "issuerURL", v.issuerURL)
	}
	provider, err := oidc.NewProvider(initCtx, v.issuerURL)
	if err != nil {
		return fmt.Errorf("failed to initialize OIDC provider: %w", err)
	}
	v.provider = provider
	if v.logger != nil {
		v.logger.Info("OIDC provider is ready")
	}

	if v.logger != nil {
		v.logger.Info("Configuring token verifier",
			"issuer URL", v.issuerURL,
			"client ID", v.clientID)
	}
	v.verifier = provider.Verifier(v.oidcConfig)
	if v.logger != nil {
		v.logger.Info("Token verifier is ready")
	}
	return nil
}

// VerifyToken verifies an OIDC token and returns Claims, isFault, error.
// It may call the provider to refresh the public keySet if not cached
func (v *OIDCVerifier) VerifyToken(ctx context.Context, tokenString string, logger *slog.Logger) (*OIDCClaims, bool, error) {
	if v.verifier == nil {
		return nil, true, fmt.Errorf("OIDC verifier is not initialized - call Start() first")
	}

	// Verify the token
	idToken, err := v.verifier.Verify(ctx, tokenString)
	if err != nil {
		// Check if this is a discovery document error
		errMsg := err.Error()
		if strings.Contains(errMsg, "failed to get discovery document") ||
			errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, context.Canceled) {
			return nil, true, fmt.Errorf("failed to connect to OIDC provider: %w", err)
		}

		// All other errors are likely token validation errors
		return nil, false, fmt.Errorf("invalid ID token: %w", err)
	}

	// Extract claims from the token
	var claims OIDCClaims

	// Log the response from Dex for debugging
	logger.Info("Received verified token from Dex",
		"issuer", idToken.Issuer,
		"subject", idToken.Subject,
		"audience", idToken.Audience,
		"expiration", idToken.Expiry,
		"issued_at", idToken.IssuedAt)

	if err := idToken.Claims(&claims); err != nil {
		return nil, false, fmt.Errorf("failed to parse claims: %w", err)
	}

	// Log detailed claims information to help verify correct parsing in production
	// This is especially useful when integrating with Dex using GitHub connector
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		logger.Debug("Successfully parsed OIDC claims",
			"username_claim", claims.Username,
			"email_claim", claims.Email,
			"subject_format", claims.Subject,
			"groups_count", len(claims.Groups),
			"has_groups", len(claims.Groups) > 0,
		)

		// Log group format examples to help diagnose GitHub org/team format issues
		if len(claims.Groups) > 0 {
			groupSamples := claims.Groups
			if len(groupSamples) > 3 {
				groupSamples = groupSamples[:3] // Limit to first 3 groups to avoid log spam
			}
			logger.Debug("Group format examples",
				"group_samples", strings.Join(groupSamples, ", "),
			)
		}

		// Log any extra fields that might be useful for debugging
		if len(claims.ExtraClaimsField) > 0 {
			extraClaimKeys := make([]string, 0, len(claims.ExtraClaimsField))
			for k := range claims.ExtraClaimsField {
				extraClaimKeys = append(extraClaimKeys, k)
			}
			logger.Debug("Extra claims present",
				"extra_claim_keys", strings.Join(extraClaimKeys, ", "),
			)
		}
	}

	return &claims, false, nil
}

// GetOIDCGroupsFromToken extracts and formats group names from OIDC claims
func GetOIDCGroupsFromToken(config *Config, claims *OIDCClaims) []string {
	if claims == nil || len(claims.Groups) == 0 {
		return []string{}
	}
	return GetOidcGroups(config, claims.Groups)
}

// GetOIDCUsernameFromToken extracts and formats the username from OIDC claims
func GetOIDCUsernameFromToken(config *Config, claims *OIDCClaims) string {
	if claims == nil || claims.Username == "" {
		return ""
	}
	return GetOidcUsername(config, claims.Username)
}
