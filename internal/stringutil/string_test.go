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

package stringutil

import "testing"

func TestSanitizeUsername(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal username",
			input:    "user123",
			expected: "user123",
		},
		{
			name:     "email address",
			input:    "test@example.com",
			expected: "test@example.com",
		},
		{
			name:     "AWS IAM role ARN",
			input:    "arn:aws:iam::123456789012:role/EKSRole",
			expected: "arn:aws:iam::123456789012:role/EKSRole",
		},
		{
			name:     "newline character",
			input:    "user\nname",
			expected: "user\\nname",
		},
		{
			name:     "tab character",
			input:    "user\tname",
			expected: "user\\tname",
		},
		{
			name:     "quote character",
			input:    "user\"name",
			expected: "user\\\"name",
		},
		{
			name:     "backslash character",
			input:    "user\\name",
			expected: "user\\\\name",
		},
		{
			name:     "unicode characters",
			input:    "用户",
			expected: "用户",
		},
		{
			name:     "emoji",
			input:    "user🚀",
			expected: "user🚀",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeUsername(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeUsername(%q) = %q, expected %q", tt.input, result, tt.expected)
			}
		})
	}
}
