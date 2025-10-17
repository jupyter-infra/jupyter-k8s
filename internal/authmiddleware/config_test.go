package authmiddleware

import (
	"os"
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

	// CSRF configuration
	setEnv(t, EnvCsrfAuthKey, "custom-csrf-key")
	setEnv(t, EnvCsrfCookieName, "custom_csrf")
	setEnv(t, EnvCsrfCookieMaxAge, "45m")
	setEnv(t, EnvCsrfCookieSecure, "false")
	setEnv(t, EnvCsrfFieldName, "custom_token")
	setEnv(t, EnvCsrfHeaderName, "X-Custom-CSRF")
	setEnv(t, EnvCsrfTrustedOrigins, "https://trusted1.com,https://trusted2.com")

	// Clean up environment variables after the test
	vars := []string{
		EnvPort, EnvReadTimeout, EnvWriteTimeout, EnvShutdownTimeout, EnvTrustedProxies,
		EnvJwtSigningKey, EnvJwtIssuer, EnvJwtAudience, EnvJwtExpiration,
		EnvCookieName, EnvCookieSecure, EnvCookieDomain, EnvCookiePath,
		EnvCookieMaxAge, EnvCookieHttpOnly, EnvCookieSameSite,
		EnvPathRegexPattern,
		EnvCsrfAuthKey, EnvCsrfCookieName,
		EnvCsrfCookieMaxAge, EnvCsrfCookieSecure, EnvCsrfFieldName, EnvCsrfHeaderName,
		EnvCsrfTrustedOrigins,
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
