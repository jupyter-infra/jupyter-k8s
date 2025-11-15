package authmiddleware

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

// TestNewConfigDefault verifies that default values are used correctly if not passed
func TestNewConfigDefault(t *testing.T) {
	// Set required environment variables
	if err := os.Setenv(EnvJwtSigningKey, "test-signing-key"); err != nil {
		t.Fatalf("Failed to set environment variable %s: %v", EnvJwtSigningKey, err)
	}
	if err := os.Setenv(EnvCsrfAuthKey, "test-csrf-key"); err != nil {
		t.Fatalf("Failed to set environment variable %s: %v", EnvCsrfAuthKey, err)
	}
	defer func() {
		if err := os.Unsetenv(EnvJwtSigningKey); err != nil {
			t.Logf("Failed to unset environment variable %s: %v", EnvJwtSigningKey, err)
		}
		if err := os.Unsetenv(EnvCsrfAuthKey); err != nil {
			t.Logf("Failed to unset environment variable %s: %v", EnvCsrfAuthKey, err)
		}
	}()

	config, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	// Check config default values
	checkServerDefaults(t, config)
	checkAuthDefaults(t, config)
	checkCookieDefaults(t, config)
	checkPathDefaults(t, config)
	checkCSRFDefaults(t, config)
	checkOIDCDefaults(t, config)
}

// Helper functions for checking default config values

func checkServerDefaults(t *testing.T, config *Config) {
	if config.Port != DefaultPort {
		t.Errorf("Expected Port to be %d, got %d", DefaultPort, config.Port)
	}
	if config.ReadTimeout != DefaultReadTimeout {
		t.Errorf("Expected ReadTimeout to be %v, got %v", DefaultReadTimeout, config.ReadTimeout)
	}
	if config.WriteTimeout != DefaultWriteTimeout {
		t.Errorf("Expected WriteTimeout to be %v, got %v", DefaultWriteTimeout, config.WriteTimeout)
	}
	if config.ShutdownTimeout != DefaultShutdownTimeout {
		t.Errorf("Expected ShutdownTimeout to be %v, got %v", DefaultShutdownTimeout, config.ShutdownTimeout)
	}
	if len(config.TrustedProxies) != 2 || config.TrustedProxies[0] != "127.0.0.1" || config.TrustedProxies[1] != "::1" {
		t.Errorf("Expected TrustedProxies to be [127.0.0.1 ::1], got %v", config.TrustedProxies)
	}
}

func checkAuthDefaults(t *testing.T, config *Config) {
	if config.JWTSigningKey != "test-signing-key" {
		t.Errorf("Expected JWTSigningKey to be %s, got %s", "test-signing-key", config.JWTSigningKey)
	}
	if config.JWTIssuer != DefaultJwtIssuer {
		t.Errorf("Expected JWTIssuer to be %s, got %s", DefaultJwtIssuer, config.JWTIssuer)
	}
	if config.JWTAudience != DefaultJwtAudience {
		t.Errorf("Expected JWTAudience to be %s, got %s", DefaultJwtAudience, config.JWTAudience)
	}
	if config.JWTExpiration != DefaultJwtExpiration {
		t.Errorf("Expected JWTExpiration to be %v, got %v", DefaultJwtExpiration, config.JWTExpiration)
	}
	if config.JWTRefreshEnable != DefaultJwtRefreshEnable {
		t.Errorf("Expected JWTRefreshEbable to be %t, got %t", DefaultJwtRefreshEnable, config.JWTRefreshEnable)
	}
	if config.JWTRefreshHorizon != DefaultJwtRefreshHorizon {
		t.Errorf("Expected JWTRefreshHorizon to be %v, got %v", DefaultJwtRefreshHorizon, config.JWTRefreshHorizon)
	}
	if config.JWTRefreshWindow != DefaultJwtRefreshWindow {
		t.Errorf("Expected JWTRefreshWindow to be %v, got %v", DefaultJwtRefreshWindow, config.JWTRefreshWindow)
	}
}

func checkCookieDefaults(t *testing.T, config *Config) {
	if config.CookieName != DefaultCookieName {
		t.Errorf("Expected CookieName to be %s, got %s", DefaultCookieName, config.CookieName)
	}
	if config.CookieSecure != DefaultCookieSecure {
		t.Errorf("Expected CookieSecure to be %t, got %t", DefaultCookieSecure, config.CookieSecure)
	}
	if config.CookiePath != DefaultCookiePath {
		t.Errorf("Expected CookiePath to be %s, got %s", DefaultCookiePath, config.CookiePath)
	}
	if config.CookieMaxAge != DefaultCookieMaxAge {
		t.Errorf("Expected CookieMaxAge to be %v, got %v", DefaultCookieMaxAge, config.CookieMaxAge)
	}
	if config.CookieHTTPOnly != DefaultCookieHttpOnly {
		t.Errorf("Expected CookieHTTPOnly to be %t, got %t", DefaultCookieHttpOnly, config.CookieHTTPOnly)
	}
	if config.CookieSameSite != DefaultCookieSameSite {
		t.Errorf("Expected CookieSameSite to be %s, got %s", DefaultCookieSameSite, config.CookieSameSite)
	}
}

func checkPathDefaults(t *testing.T, config *Config) {
	if config.PathRegexPattern != DefaultPathRegexPattern {
		t.Errorf("Expected PathRegexPattern to be %s, got %s", DefaultPathRegexPattern, config.PathRegexPattern)
	}
	if config.WorkspaceNamespacePathRegex != DefaultWorkspaceNamespacePathRegex {
		t.Errorf("Expected WorkspaceNamespacePathRegex to be %s, got %s", DefaultWorkspaceNamespacePathRegex, config.WorkspaceNamespacePathRegex)
	}
	if config.WorkspaceNamePathRegex != DefaultWorkspaceNamePathRegex {
		t.Errorf("Expected WorkspaceNamePathRegex to be %s, got %s", DefaultWorkspaceNamePathRegex, config.WorkspaceNamePathRegex)
	}
}

func checkCSRFDefaults(t *testing.T, config *Config) {
	if config.CSRFAuthKey != "test-csrf-key" {
		t.Errorf("Expected CSRFAuthKey to be %s, got %s", "test-csrf-key", config.CSRFAuthKey)
	}
	if config.CSRFCookieName != DefaultCsrfCookieName {
		t.Errorf("Expected CSRFCookieName to be %s, got %s", DefaultCsrfCookieName, config.CSRFCookieName)
	}
	// CSRF cookies now use the same path as regular cookies (config.CookiePath)
	if config.CSRFCookieMaxAge != DefaultCsrfCookieMaxAge {
		t.Errorf("Expected CSRFCookieMaxAge to be %v, got %v", DefaultCsrfCookieMaxAge, config.CSRFCookieMaxAge)
	}
	if config.CSRFCookieSecure != DefaultCsrfCookieSecure {
		t.Errorf("Expected CSRFCookieSecure to be %t, got %t", DefaultCsrfCookieSecure, config.CSRFCookieSecure)
	}
	if config.CSRFFieldName != DefaultCsrfFieldName {
		t.Errorf("Expected CSRFFieldName to be %s, got %s", DefaultCsrfFieldName, config.CSRFFieldName)
	}
	if config.CSRFHeaderName != DefaultCsrfHeaderName {
		t.Errorf("Expected CSRFHeaderName to be %s, got %s", DefaultCsrfHeaderName, config.CSRFHeaderName)
	}
}

// checkOIDCDefaults verifies the default OIDC configuration values
func checkOIDCDefaults(t *testing.T, config *Config) {
	if config.OidcUsernamePrefix != DefaultOidcUsernamePrefix {
		t.Errorf("Expected OidcUsernamePrefix to be %s, got %s", DefaultOidcUsernamePrefix, config.OidcUsernamePrefix)
	}
	if config.OidcGroupsPrefix != DefaultOidcGroupsPrefix {
		t.Errorf("Expected OidcGroupsPrefix to be %s, got %s", DefaultOidcGroupsPrefix, config.OidcGroupsPrefix)
	}
	if config.OIDCIssuerURL != "" {
		t.Errorf("Expected default OIDCIssuerURL to be empty, got %s", config.OIDCIssuerURL)
	}
	if config.OIDCClientID != "" {
		t.Errorf("Expected default OIDCClientID to be empty, got %s", config.OIDCClientID)
	}
	if config.OIDCInitTimeoutSecs != DefaultOIDCInitTimeoutSecs {
		t.Errorf("Expected OIDCInitTimeoutSecs to be %d, got %d", DefaultOIDCInitTimeoutSecs, config.OIDCInitTimeoutSecs)
	}
}

// setEnv is a helper function to set environment variables for tests
func setEnv(t *testing.T, key, value string) {
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("Failed to set environment variable %s: %v", key, err)
	}
}

// unsetEnv is a helper function to unset environment variables for tests
func unsetEnv(t *testing.T, vars []string) {
	for _, v := range vars {
		if err := os.Unsetenv(v); err != nil {
			t.Logf("Failed to unset environment variable %s: %v", v, err)
		}
	}
}

// TestNewConfigEnvOverrides verifies that values are propagated from environment variables as expected
func TestNewConfigEnvOverrides(t *testing.T) {
	// Server configuration
	setEnv(t, EnvPort, "9090")
	setEnv(t, EnvReadTimeout, "15s")
	setEnv(t, EnvWriteTimeout, "20s")
	setEnv(t, EnvShutdownTimeout, "45s")
	setEnv(t, EnvTrustedProxies, "192.168.1.1,10.0.0.1")

	// Auth configuration
	setEnv(t, EnvJwtSigningKey, "custom-jwt-key")
	setEnv(t, EnvJwtIssuer, "custom-issuer")
	setEnv(t, EnvJwtAudience, "custom-audience")
	setEnv(t, EnvJwtExpiration, "30m")
	setEnv(t, EnvEnableJwtRefresh, "false")
	setEnv(t, EnvJwtRefreshHorizon, "24h")
	setEnv(t, EnvJwtRefreshWindow, "5m")

	// Cookie configuration
	setEnv(t, EnvCookieName, "custom_auth")
	setEnv(t, EnvCookieSecure, "false")
	setEnv(t, EnvCookieDomain, "some.example.com")
	setEnv(t, EnvCookiePath, "/custom")
	setEnv(t, EnvCookieMaxAge, "12h")
	setEnv(t, EnvCookieHttpOnly, "false")
	setEnv(t, EnvCookieSameSite, "strict")

	// Path configuration
	setEnv(t, EnvPathRegexPattern, "^(/custom/[^/]+)(?:/.*)?$")
	setEnv(t, EnvWorkspaceNamespacePathRegex, "^/custom/([^/]+)/workspaces/[^/]+")
	setEnv(t, EnvWorkspaceNamePathRegex, "^/custom/[^/]+/workspaces/([^/]+)")

	// CSRF configuration
	setEnv(t, EnvCsrfAuthKey, "custom-csrf-key")
	setEnv(t, EnvCsrfCookieName, "custom_csrf")
	setEnv(t, EnvCsrfCookieMaxAge, "45m")
	setEnv(t, EnvCsrfCookieSecure, "false")
	setEnv(t, EnvCsrfFieldName, "custom_token")
	setEnv(t, EnvCsrfHeaderName, "X-Custom-CSRF")
	setEnv(t, EnvCsrfTrustedOrigins, "https://trusted1.com,https://trusted2.com")

	// OIDC configuration
	setEnv(t, EnvOidcUsernamePrefix, "oidc:")
	setEnv(t, EnvOidcGroupsPrefix, "oidc-group:")
	setEnv(t, EnvOIDCIssuerURL, "https://test-dex.example.com")
	setEnv(t, EnvOIDCClientID, "test-client-id")
	setEnv(t, EnvOIDCInitTimeoutSecs, "45")

	// Clean up environment variables after the test
	vars := []string{
		EnvPort, EnvReadTimeout, EnvWriteTimeout, EnvShutdownTimeout, EnvTrustedProxies,
		EnvJwtSigningKey, EnvJwtIssuer, EnvJwtAudience, EnvJwtExpiration,
		EnvEnableJwtRefresh, EnvJwtRefreshHorizon, EnvJwtRefreshWindow,
		EnvCookieName, EnvCookieSecure, EnvCookieDomain, EnvCookiePath,
		EnvCookieMaxAge, EnvCookieHttpOnly, EnvCookieSameSite,
		EnvPathRegexPattern, EnvWorkspaceNamespacePathRegex, EnvWorkspaceNamePathRegex,
		EnvCsrfAuthKey, EnvCsrfCookieName,
		EnvCsrfCookieMaxAge, EnvCsrfCookieSecure, EnvCsrfFieldName, EnvCsrfHeaderName,
		EnvCsrfTrustedOrigins,
		EnvOidcUsernamePrefix, EnvOidcGroupsPrefix,
		EnvOIDCIssuerURL, EnvOIDCClientID, EnvOIDCInitTimeoutSecs,
	}
	defer unsetEnv(t, vars)

	config, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	// Check server configuration
	checkServerConfig(t, config)

	// Check auth configuration
	checkAuthConfig(t, config)

	// Check cookie configuration
	checkCookieConfig(t, config)

	// Check path configuration
	checkPathConfig(t, config)

	// Check CSRF configuration
	checkCSRFConfig(t, config)

	// Check OIDC configuration
	checkOIDCConfig(t, config)
}

// Helper functions to split the assertions for cyclomatic complexity reduction

func checkServerConfig(t *testing.T, config *Config) {
	if config.Port != 9090 {
		t.Errorf("Expected Port to be 9090, got %d", config.Port)
	}
	if config.ReadTimeout != 15*time.Second {
		t.Errorf("Expected ReadTimeout to be 15s, got %v", config.ReadTimeout)
	}
	if config.WriteTimeout != 20*time.Second {
		t.Errorf("Expected WriteTimeout to be 20s, got %v", config.WriteTimeout)
	}
	if config.ShutdownTimeout != 45*time.Second {
		t.Errorf("Expected ShutdownTimeout to be 45s, got %v", config.ShutdownTimeout)
	}
	if len(config.TrustedProxies) != 2 || config.TrustedProxies[0] != "192.168.1.1" || config.TrustedProxies[1] != "10.0.0.1" {
		t.Errorf("Expected TrustedProxies to be [192.168.1.1 10.0.0.1], got %v", config.TrustedProxies)
	}
}

func checkAuthConfig(t *testing.T, config *Config) {
	if config.JWTSigningKey != "custom-jwt-key" {
		t.Errorf("Expected JWTSigningKey to be custom-jwt-key, got %s", config.JWTSigningKey)
	}
	if config.JWTIssuer != "custom-issuer" {
		t.Errorf("Expected JWTIssuer to be custom-issuer, got %s", config.JWTIssuer)
	}
	if config.JWTAudience != "custom-audience" {
		t.Errorf("Expected JWTAudience to be custom-audience, got %s", config.JWTAudience)
	}
	if config.JWTExpiration != 30*time.Minute {
		t.Errorf("Expected JWTExpiration to be 30m, got %v", config.JWTExpiration)
	}
	if config.JWTRefreshEnable != false {
		t.Errorf("Expected JWTRefreshEnable to be false,, got %t", config.JWTRefreshEnable)
	}
	if config.JWTRefreshHorizon != 24*time.Hour {
		t.Errorf("Expected JWTExpiration to be 24h, got %v", config.JWTRefreshHorizon)
	}
	if config.JWTRefreshWindow != 5*time.Minute {
		t.Errorf("Expected JWTExpiration to be 5m, got %v", config.JWTRefreshWindow)
	}
}

func checkCookieConfig(t *testing.T, config *Config) {
	if config.CookieName != "custom_auth" {
		t.Errorf("Expected CookieName to be custom_auth, got %s", config.CookieName)
	}
	if config.CookieSecure != false {
		t.Errorf("Expected CookieSecure to be false, got %t", config.CookieSecure)
	}
	if config.CookieDomain != "some.example.com" {
		t.Errorf("Expected CookieDomain to be some.example.com, got %s", config.CookieDomain)
	}
	if config.CookiePath != "/custom" {
		t.Errorf("Expected CookiePath to be /custom, got %s", config.CookiePath)
	}
	if config.CookieMaxAge != 12*time.Hour {
		t.Errorf("Expected CookieMaxAge to be 12h, got %v", config.CookieMaxAge)
	}
	if config.CookieHTTPOnly != false {
		t.Errorf("Expected CookieHTTPOnly to be false, got %t", config.CookieHTTPOnly)
	}
	if config.CookieSameSite != "strict" {
		t.Errorf("Expected CookieSameSite to be strict, got %s", config.CookieSameSite)
	}
}

func checkPathConfig(t *testing.T, config *Config) {
	if config.PathRegexPattern != "^(/custom/[^/]+)(?:/.*)?$" {
		t.Errorf("Expected PathRegexPattern to be ^(/custom/[^/]+)(?:/.*)?$, got %s", config.PathRegexPattern)
	}

	// Check the new regex patterns added for workspace paths
	if config.WorkspaceNamespacePathRegex != "^/custom/([^/]+)/workspaces/[^/]+" {
		t.Errorf("Expected WorkspaceNamespacePathRegex to be ^/custom/([^/]+)/workspaces/[^/]+, got %s", config.WorkspaceNamespacePathRegex)
	}

	if config.WorkspaceNamePathRegex != "^/custom/[^/]+/workspaces/([^/]+)" {
		t.Errorf("Expected WorkspaceNamePathRegex to be ^/custom/[^/]+/workspaces/([^/]+), got %s", config.WorkspaceNamePathRegex)
	}
}

func checkCSRFConfig(t *testing.T, config *Config) {
	if config.CSRFAuthKey != "custom-csrf-key" {
		t.Errorf("Expected CSRFAuthKey to be custom-csrf-key, got %s", config.CSRFAuthKey)
	}
	if config.CSRFCookieName != "custom_csrf" {
		t.Errorf("Expected CSRFCookieName to be custom_csrf, got %s", config.CSRFCookieName)
	}
	// CSRF cookies now use the same path and domain as regular cookies
	// (config.CookiePath which is "/custom" and config.CookieDomain which is "some.example.com")
	if config.CSRFCookieMaxAge != 45*time.Minute {
		t.Errorf("Expected CSRFCookieMaxAge to be 45m, got %v", config.CSRFCookieMaxAge)
	}
	if config.CSRFCookieSecure != false {
		t.Errorf("Expected CSRFCookieSecure to be false, got %t", config.CSRFCookieSecure)
	}
	if config.CSRFFieldName != "custom_token" {
		t.Errorf("Expected CSRFFieldName to be custom_token, got %s", config.CSRFFieldName)
	}
	if config.CSRFHeaderName != "X-Custom-CSRF" {
		t.Errorf("Expected CSRFHeaderName to be X-Custom-CSRF, got %s", config.CSRFHeaderName)
	}
	if len(config.CSRFTrustedOrigins) != 2 || config.CSRFTrustedOrigins[0] != "https://trusted1.com" || config.CSRFTrustedOrigins[1] != "https://trusted2.com" {
		t.Errorf("Expected CSRFTrustedOrigins to be [https://trusted1.com https://trusted2.com], got %v", config.CSRFTrustedOrigins)
	}
}

// checkOIDCConfig validates that OIDC configuration values are set as expected
func checkOIDCConfig(t *testing.T, config *Config) {
	if config.OidcUsernamePrefix != "oidc:" {
		t.Errorf("Expected OidcUsernamePrefix to be oidc:, got %s", config.OidcUsernamePrefix)
	}
	if config.OidcGroupsPrefix != "oidc-group:" {
		t.Errorf("Expected OidcGroupsPrefix to be oidc-group:, got %s", config.OidcGroupsPrefix)
	}
	if config.OIDCIssuerURL != "https://test-dex.example.com" {
		t.Errorf("Expected OIDCIssuerURL to be https://test-dex.example.com, got %s", config.OIDCIssuerURL)
	}
	if config.OIDCClientID != "test-client-id" {
		t.Errorf("Expected OIDCClientID to be test-client-id, got %s", config.OIDCClientID)
	}
	if config.OIDCInitTimeoutSecs != 45 {
		t.Errorf("Expected OIDCInitTimeoutSecs to be 45, got %d", config.OIDCInitTimeoutSecs)
	}
}

// TestOIDCInitTimeoutConfig tests that the OIDCInitTimeoutSecs configuration
// correctly handles various environment variable scenarios
func TestOIDCInitTimeoutConfig(t *testing.T) {
	testCases := []struct {
		name          string
		envValue      string
		expectedValue int
		expectError   bool
	}{
		{
			name:          "Default value when env var not set",
			envValue:      "",
			expectedValue: DefaultOIDCInitTimeoutSecs,
			expectError:   false,
		},
		{
			name:          "Valid timeout value",
			envValue:      "60",
			expectedValue: 60,
			expectError:   false,
		},
		{
			name:          "Zero timeout value",
			envValue:      "0",
			expectedValue: 0,
			expectError:   true, // Now we expect an error for zero timeout
		},
		{
			name:          "Negative timeout value",
			envValue:      "-5",
			expectedValue: 0,
			expectError:   true, // Should error on negative timeout
		},
		{
			name:        "Invalid non-numeric timeout",
			envValue:    "invalid",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set environment variable for JWT signing key (required)
			if err := os.Setenv(EnvJwtSigningKey, "test-signing-key"); err != nil {
				t.Fatalf("Failed to set environment variable %s: %v", EnvJwtSigningKey, err)
			}
			if err := os.Setenv(EnvCsrfAuthKey, "test-csrf-key"); err != nil {
				t.Fatalf("Failed to set environment variable %s: %v", EnvCsrfAuthKey, err)
			}
			defer func() {
				if err := os.Unsetenv(EnvJwtSigningKey); err != nil {
					t.Logf("Failed to unset environment variable %s: %v", EnvJwtSigningKey, err)
				}
				if err := os.Unsetenv(EnvCsrfAuthKey); err != nil {
					t.Logf("Failed to unset environment variable %s: %v", EnvCsrfAuthKey, err)
				}
			}()

			// Set test environment variable if value is provided
			if tc.envValue != "" {
				if err := os.Setenv(EnvOIDCInitTimeoutSecs, tc.envValue); err != nil {
					t.Fatalf("Failed to set environment variable %s: %v", EnvOIDCInitTimeoutSecs, err)
				}
				defer func() {
					if err := os.Unsetenv(EnvOIDCInitTimeoutSecs); err != nil {
						t.Logf("Failed to unset environment variable %s: %v", EnvOIDCInitTimeoutSecs, err)
					}
				}()
			}

			// Create config
			config, err := NewConfig()

			if tc.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Fatalf("NewConfig() error = %v", err)
			}

			// Check timeout value
			if config.OIDCInitTimeoutSecs != tc.expectedValue {
				t.Errorf("Expected OIDCInitTimeoutSecs to be %d, got %d", tc.expectedValue, config.OIDCInitTimeoutSecs)
			}
		})
	}
}

// TestApplyOidcConfig tests that the applyOidcConfig function
// correctly applies environment variables to the config.
func TestApplyOidcConfig(t *testing.T) {
	testCases := []struct {
		name             string
		envOidcUsername  string
		envOidcGroups    string
		expectedUsername string
		expectedGroups   string
	}{
		{
			name:             "Default values when env vars not set",
			envOidcUsername:  "",
			envOidcGroups:    "",
			expectedUsername: DefaultOidcUsernamePrefix,
			expectedGroups:   DefaultOidcGroupsPrefix,
		},
		{
			name:             "Only username prefix set",
			envOidcUsername:  "gitlab:",
			envOidcGroups:    "",
			expectedUsername: "gitlab:",
			expectedGroups:   DefaultOidcGroupsPrefix,
		},
		{
			name:             "Only groups prefix set",
			envOidcUsername:  "",
			envOidcGroups:    "azure-ad:",
			expectedUsername: DefaultOidcUsernamePrefix,
			expectedGroups:   "azure-ad:",
		},
		{
			name:             "Both prefixes set",
			envOidcUsername:  "custom-user:",
			envOidcGroups:    "custom-group:",
			expectedUsername: "custom-user:",
			expectedGroups:   "custom-group:",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Set environment variables if values are provided
			if tc.envOidcUsername != "" {
				if err := os.Setenv(EnvOidcUsernamePrefix, tc.envOidcUsername); err != nil {
					t.Fatalf("Failed to set environment variable %s: %v", EnvOidcUsernamePrefix, err)
				}
			}
			if tc.envOidcGroups != "" {
				if err := os.Setenv(EnvOidcGroupsPrefix, tc.envOidcGroups); err != nil {
					t.Fatalf("Failed to set environment variable %s: %v", EnvOidcGroupsPrefix, err)
				}
			}

			// Create a config with default values
			config := createDefaultConfig()

			// Set default OIDC values (these would be set by createDefaultConfig in the real code)
			config.OidcUsernamePrefix = DefaultOidcUsernamePrefix
			config.OidcGroupsPrefix = DefaultOidcGroupsPrefix

			// Apply OIDC configuration
			if err := applyOidcConfig(config); err != nil {
				t.Fatalf("Failed to apply OIDC configuration: %v", err)
			}

			// Check values
			if config.OidcUsernamePrefix != tc.expectedUsername {
				t.Errorf("Expected OidcUsernamePrefix to be %s, got %s", tc.expectedUsername, config.OidcUsernamePrefix)
			}
			if config.OidcGroupsPrefix != tc.expectedGroups {
				t.Errorf("Expected OidcGroupsPrefix to be %s, got %s", tc.expectedGroups, config.OidcGroupsPrefix)
			}

			// Clean up environment variables
			if tc.envOidcUsername != "" {
				if err := os.Unsetenv(EnvOidcUsernamePrefix); err != nil {
					t.Logf("Failed to unset environment variable %s: %v", EnvOidcUsernamePrefix, err)
				}
			}
			if tc.envOidcGroups != "" {
				if err := os.Unsetenv(EnvOidcGroupsPrefix); err != nil {
					t.Logf("Failed to unset environment variable %s: %v", EnvOidcGroupsPrefix, err)
				}
			}
		})
	}
}

// TestOIDCVerifierInitConfig tests that the NewOIDCVerifier function properly validates config
func TestOIDCVerifierInitConfig(t *testing.T) {
	testCases := []struct {
		name         string
		issuerURL    string
		clientID     string
		clientSecret string
		expectError  bool
	}{
		{
			name:         "Missing issuer URL",
			issuerURL:    "",
			clientID:     "test-client",
			clientSecret: "test-secret",
			expectError:  true,
		},
		{
			name:         "Missing client ID",
			issuerURL:    "https://dex.example.com",
			clientID:     "",
			clientSecret: "test-secret",
			expectError:  true,
		},
		{
			name:         "Empty client secret",
			issuerURL:    "https://dex.example.com",
			clientID:     "test-client",
			clientSecret: "",
			expectError:  false, // Client secret is allowed to be empty
		},
		{
			name:         "Valid config",
			issuerURL:    "https://dex.example.com",
			clientID:     "test-client",
			clientSecret: "test-secret",
			expectError:  false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a dummy logger for testing
			logger := slog.Default()

			// Create a config with test values
			config := &Config{
				OIDCIssuerURL:       tc.issuerURL,
				OIDCClientID:        tc.clientID,
				OIDCInitTimeoutSecs: 1, // Short timeout for tests
			}

			// Note: We no longer need to skip initialization since our refactoring
			// moved the actual provider initialization to the Start() method

			_, err := NewOIDCVerifier(config, logger)

			// Check if error matches expectation
			if tc.expectError && err == nil {
				t.Errorf("Expected error but got none")
			} else if !tc.expectError && err != nil {
				// Only report error if it's not related to context cancellation
				if !strings.Contains(err.Error(), "context canceled") {
					t.Errorf("Did not expect error but got: %v", err)
				}
			}
		})
	}
}
