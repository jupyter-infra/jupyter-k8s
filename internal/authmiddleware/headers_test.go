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
