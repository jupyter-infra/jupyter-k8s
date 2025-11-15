package authmiddleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
