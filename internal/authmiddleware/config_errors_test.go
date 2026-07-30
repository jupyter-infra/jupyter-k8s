/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// notBoolValue is an invalid boolean literal reused across the malformed-env cases.
const notBoolValue = "notbool"

// TestNewConfig_InvalidEnvValues covers the error branches of the per-section
// config appliers: each malformed environment variable should cause NewConfig
// to fail with a message naming the offending variable.
func TestNewConfig_InvalidEnvValues(t *testing.T) {
	tests := []struct {
		name    string
		env     string
		value   string
		wantMsg string
	}{
		// Server
		{name: "invalid port", env: EnvPort, value: "not-a-number", wantMsg: EnvPort},
		{name: "invalid read timeout", env: EnvReadTimeout, value: testAbcValue, wantMsg: EnvReadTimeout},
		{name: "invalid write timeout", env: EnvWriteTimeout, value: testAbcValue, wantMsg: EnvWriteTimeout},
		{name: "invalid shutdown timeout", env: EnvShutdownTimeout, value: testAbcValue, wantMsg: EnvShutdownTimeout},

		// JWT
		{name: "invalid jwt expiration", env: EnvJwtExpiration, value: testAbcValue, wantMsg: EnvJwtExpiration},
		{name: "invalid jwt refresh window", env: EnvJwtRefreshWindow, value: testAbcValue, wantMsg: EnvJwtRefreshWindow},
		{name: "invalid jwt refresh horizon", env: EnvJwtRefreshHorizon, value: testAbcValue, wantMsg: EnvJwtRefreshHorizon},
		{name: "invalid new key use delay", env: EnvJwtNewKeyUseDelay, value: testAbcValue, wantMsg: EnvJwtNewKeyUseDelay},
		{name: "invalid enable oauth", env: EnvEnableOAuth, value: notBoolValue, wantMsg: EnvEnableOAuth},
		{name: "invalid enable bearer auth", env: EnvEnableBearerAuth, value: notBoolValue, wantMsg: EnvEnableBearerAuth},

		// Cookie
		{name: "invalid cookie secure", env: EnvCookieSecure, value: notBoolValue, wantMsg: EnvCookieSecure},
		{name: "invalid cookie max age", env: EnvCookieMaxAge, value: testAbcValue, wantMsg: EnvCookieMaxAge},
		{name: "invalid cookie http only", env: EnvCookieHttpOnly, value: notBoolValue, wantMsg: EnvCookieHttpOnly},

		// OIDC
		{name: "invalid oidc init timeout", env: EnvOIDCInitTimeoutSecs, value: testAbcValue, wantMsg: EnvOIDCInitTimeoutSecs},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(tt.env, tt.value)

			_, err := NewConfig()

			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantMsg)
		})
	}
}

func TestNewConfig_OIDCInitTimeoutMustBePositive(t *testing.T) {
	t.Setenv(EnvOIDCInitTimeoutSecs, "0")

	_, err := NewConfig()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a positive integer")
}

func TestNewConfig_RefreshWindowExceedsExpiration(t *testing.T) {
	// Refresh window larger than expiration is rejected.
	t.Setenv(EnvJwtExpiration, "10m")
	t.Setenv(EnvJwtRefreshWindow, "20m")

	_, err := NewConfig()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "refresh window")
}

func TestNewConfig_ExpirationExceedsRefreshHorizon(t *testing.T) {
	// Expiration larger than the refresh horizon is rejected. Keep the refresh
	// window under the expiration so only the horizon check trips.
	t.Setenv(EnvJwtExpiration, "5h")
	t.Setenv(EnvJwtRefreshWindow, "1m")
	t.Setenv(EnvJwtRefreshHorizon, "1h")

	_, err := NewConfig()

	require.Error(t, err)
	assert.Contains(t, err.Error(), "refresh horizon")
}

func TestNewConfig_TrustedProxiesOverride(t *testing.T) {
	// Covers the TRUSTED_PROXIES split branch in applyServerConfig. splitAndTrim
	// drops empty segments but does not strip surrounding whitespace, so the input
	// is kept space-free here.
	t.Setenv(EnvTrustedProxies, "10.0.0.1,10.0.0.2,,10.0.0.3")

	config, err := NewConfig()

	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}, config.TrustedProxies)
}
