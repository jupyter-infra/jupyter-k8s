/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
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
