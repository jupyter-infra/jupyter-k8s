/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractBearerToken tests the ExtractBearerToken function with various inputs
func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		name        string
		authHeader  string
		expected    string
		expectError bool
	}{
		{
			name:        "Valid bearer token",
			authHeader:  "Bearer abc123.def456.ghi789",
			expected:    "abc123.def456.ghi789",
			expectError: false,
		},
		{
			name:        "Empty auth header",
			authHeader:  "",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Not a bearer token",
			authHeader:  "Basic dXNlcm5hbWU6cGFzc3dvcmQ=",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Bearer prefix without token",
			authHeader:  "Bearer ",
			expected:    "",
			expectError: true,
		},
		{
			name:        "Wrong case in bearer prefix",
			authHeader:  "bearer abc123.def456.ghi789",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := ExtractBearerToken(tt.authHeader)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, token)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, token)
			}
		})
	}
}

// TestGetForwardedHost tests the GetForwardedHost function
func TestGetForwardedHost(t *testing.T) {
	tests := []struct {
		name        string
		headerValue string
		expected    string
		expectError bool
	}{
		{
			name:        "Valid host header",
			headerValue: "example.com",
			expected:    "example.com",
			expectError: false,
		},
		{
			name:        "Valid host with subdomain",
			headerValue: "workspace1.example.com",
			expected:    "workspace1.example.com",
			expectError: false,
		},
		{
			name:        "Missing header",
			headerValue: "",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com", nil)
			require.NoError(t, err)

			if tt.headerValue != "" {
				req.Header.Set(HeaderForwardedHost, tt.headerValue)
			}

			host, err := GetForwardedHost(req)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, host)
				assert.Contains(t, err.Error(), "missing")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, host)
			}
		})
	}
}

// TestGetForwardedURI tests the GetForwardedURI function
func TestGetForwardedURI(t *testing.T) {
	tests := []struct {
		name        string
		headerValue string
		expected    string
		expectError bool
	}{
		{
			name:        "Valid URI",
			headerValue: "/workspace1/path/to/resource",
			expected:    "/workspace1/path/to/resource",
			expectError: false,
		},
		{
			name:        "Root path",
			headerValue: "/",
			expected:    "/",
			expectError: false,
		},
		{
			name:        "Missing header",
			headerValue: "",
			expected:    "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", "http://example.com", nil)
			require.NoError(t, err)

			if tt.headerValue != "" {
				req.Header.Set(HeaderForwardedURI, tt.headerValue)
			}

			uri, err := GetForwardedURI(req)

			if tt.expectError {
				assert.Error(t, err)
				assert.Empty(t, uri)
				assert.Contains(t, err.Error(), "missing")
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, uri)
			}
		})
	}
}

// TestExtractSubdomain tests the ExtractSubdomain function
func TestExtractSubdomain(t *testing.T) {
	tests := []struct {
		name     string
		host     string
		expected string
	}{
		{
			name:     "Full domain with subdomain",
			host:     "workspace1.example.com",
			expected: "workspace1",
		},
		{
			name:     "Multiple subdomains",
			host:     "ws.prod.example.com",
			expected: "ws",
		},
		{
			name:     "No subdomain",
			host:     "example.com",
			expected: "example",
		},
		{
			name:     "Single word host",
			host:     "localhost",
			expected: "localhost",
		},
		{
			name:     "Empty host",
			host:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractSubdomain(tt.host)
			assert.Equal(t, tt.expected, result)
		})
	}
}
