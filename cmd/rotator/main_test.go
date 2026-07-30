/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestGetEnv(t *testing.T) {
	const key = "ROTATOR_TEST_STRING"

	t.Run("returns default when unset", func(t *testing.T) {
		assert.Equal(t, "fallback", getEnv(key, "fallback"))
	})

	t.Run("returns default when empty", func(t *testing.T) {
		t.Setenv(key, "")
		assert.Equal(t, "fallback", getEnv(key, "fallback"))
	})

	t.Run("returns value when set", func(t *testing.T) {
		t.Setenv(key, "custom")
		assert.Equal(t, "custom", getEnv(key, "fallback"))
	})
}

func TestGetEnvInt(t *testing.T) {
	const key = "ROTATOR_TEST_INT"

	t.Run("returns default when unset", func(t *testing.T) {
		assert.Equal(t, 7, getEnvInt(key, 7))
	})

	t.Run("returns default when empty", func(t *testing.T) {
		t.Setenv(key, "")
		assert.Equal(t, 7, getEnvInt(key, 7))
	})

	t.Run("parses integer value", func(t *testing.T) {
		t.Setenv(key, "12")
		assert.Equal(t, 12, getEnvInt(key, 7))
	})
}

func TestGetEnvBool(t *testing.T) {
	const key = "ROTATOR_TEST_BOOL"

	t.Run("returns default when unset", func(t *testing.T) {
		assert.True(t, getEnvBool(key, true))
	})

	t.Run("returns default when empty", func(t *testing.T) {
		t.Setenv(key, "")
		assert.True(t, getEnvBool(key, true))
	})

	t.Run("parses true", func(t *testing.T) {
		t.Setenv(key, "true")
		assert.True(t, getEnvBool(key, false))
	})

	t.Run("parses false", func(t *testing.T) {
		t.Setenv(key, "false")
		assert.False(t, getEnvBool(key, true))
	})
}

func TestDeriveNumberOfKeys(t *testing.T) {
	tests := []struct {
		name     string
		ttl      time.Duration
		interval time.Duration
		want     int
	}{
		{
			name:     "ttl equal to interval",
			ttl:      1 * time.Hour,
			interval: 1 * time.Hour,
			want:     2, // ceil(1) + 1
		},
		{
			name:     "ttl multiple of interval",
			ttl:      6 * time.Hour,
			interval: 1 * time.Hour,
			want:     7, // ceil(6) + 1
		},
		{
			name:     "ttl not a multiple rounds up",
			ttl:      90 * time.Minute,
			interval: 1 * time.Hour,
			want:     3, // ceil(1.5) = 2, + 1
		},
		{
			name:     "ttl smaller than interval",
			ttl:      10 * time.Minute,
			interval: 1 * time.Hour,
			want:     2, // ceil(0.16) = 1, + 1
		},
		{
			name:     "zero ttl",
			ttl:      0,
			interval: 1 * time.Hour,
			want:     1, // ceil(0) = 0, + 1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, deriveNumberOfKeys(tt.ttl, tt.interval))
		})
	}
}

func TestResolveNumberOfKeys(t *testing.T) {
	t.Run("defaults when nothing is set", func(t *testing.T) {
		assert.Equal(t, DefaultNumberOfKeys, resolveNumberOfKeys())
	})

	t.Run("explicit NUMBER_OF_KEYS takes precedence", func(t *testing.T) {
		t.Setenv(EnvNumberOfKeys, "9")
		t.Setenv(EnvTokenTTL, "6h")
		t.Setenv(EnvRotationInterval, "1h")
		assert.Equal(t, 9, resolveNumberOfKeys())
	})

	t.Run("derives from TOKEN_TTL and ROTATION_INTERVAL", func(t *testing.T) {
		t.Setenv(EnvTokenTTL, "6h")
		t.Setenv(EnvRotationInterval, "1h")
		assert.Equal(t, 7, resolveNumberOfKeys())
	})

	t.Run("defaults when only TOKEN_TTL is set", func(t *testing.T) {
		t.Setenv(EnvTokenTTL, "6h")
		assert.Equal(t, DefaultNumberOfKeys, resolveNumberOfKeys())
	})
}
