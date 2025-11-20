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
	"os"
	"testing"
)

// TestApplyPathConfig verifies that path config settings are applied correctly
func TestApplyPathConfig(t *testing.T) {
	// Test that defaults are applied
	t.Run("Defaults applied when not set", func(t *testing.T) {
		config := &Config{
			// Initialize with empty values
			PathRegexPattern:            "",
			WorkspaceNamespacePathRegex: "",
			WorkspaceNamePathRegex:      "",
		}

		err := applyPathConfig(config)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if config.PathRegexPattern != DefaultPathRegexPattern {
			t.Errorf("Expected PathRegexPattern to be set to default %q, got %q",
				DefaultPathRegexPattern, config.PathRegexPattern)
		}

		if config.WorkspaceNamespacePathRegex != DefaultWorkspaceNamespacePathRegex {
			t.Errorf("Expected WorkspaceNamespacePathRegex to be set to default %q, got %q",
				DefaultWorkspaceNamespacePathRegex, config.WorkspaceNamespacePathRegex)
		}

		if config.WorkspaceNamePathRegex != DefaultWorkspaceNamePathRegex {
			t.Errorf("Expected WorkspaceNamePathRegex to be set to default %q, got %q",
				DefaultWorkspaceNamePathRegex, config.WorkspaceNamePathRegex)
		}
	})

	// Test that env vars override defaults
	t.Run("Environment variables override defaults", func(t *testing.T) {
		// Save original env vars to restore them later
		origPathRegex := os.Getenv(EnvPathRegexPattern)
		origNamespaceRegex := os.Getenv(EnvWorkspaceNamespacePathRegex)
		origNameRegex := os.Getenv(EnvWorkspaceNamePathRegex)
		defer func() {
			// nolint: errcheck
			_ = os.Setenv(EnvPathRegexPattern, origPathRegex)
			// nolint: errcheck
			_ = os.Setenv(EnvWorkspaceNamespacePathRegex, origNamespaceRegex)
			// nolint: errcheck
			_ = os.Setenv(EnvWorkspaceNamePathRegex, origNameRegex)
		}()

		// Set test values
		testPathRegex := `^(/custom/[^/]+/[^/]+)(?:/.*)?$`
		testNamespaceRegex := `^/custom/([^/]+)/workspaces/[^/]+`
		testNameRegex := `^/custom/[^/]+/workspaces/([^/]+)`
		// nolint: errcheck
		_ = os.Setenv(EnvPathRegexPattern, testPathRegex)
		// nolint: errcheck
		_ = os.Setenv(EnvWorkspaceNamespacePathRegex, testNamespaceRegex)
		// nolint: errcheck
		_ = os.Setenv(EnvWorkspaceNamePathRegex, testNameRegex)

		config := &Config{
			// Initialize with defaults
			PathRegexPattern:            DefaultPathRegexPattern,
			WorkspaceNamespacePathRegex: DefaultWorkspaceNamespacePathRegex,
			WorkspaceNamePathRegex:      DefaultWorkspaceNamePathRegex,
		}

		err := applyPathConfig(config)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if config.PathRegexPattern != testPathRegex {
			t.Errorf("Expected PathRegexPattern to be %q, got %q",
				testPathRegex, config.PathRegexPattern)
		}

		if config.WorkspaceNamespacePathRegex != testNamespaceRegex {
			t.Errorf("Expected WorkspaceNamespacePathRegex to be %q, got %q",
				testNamespaceRegex, config.WorkspaceNamespacePathRegex)
		}

		if config.WorkspaceNamePathRegex != testNameRegex {
			t.Errorf("Expected WorkspaceNamePathRegex to be %q, got %q",
				testNameRegex, config.WorkspaceNamePathRegex)
		}
	})

	// Test invalid regex pattern for PathRegexPattern
	t.Run("Invalid PathRegexPattern", func(t *testing.T) {
		// Save original env vars to restore them later
		origPathRegex := os.Getenv(EnvPathRegexPattern)
		defer func() {
			// nolint: errcheck
			_ = os.Setenv(EnvPathRegexPattern, origPathRegex)
		}()

		// Set invalid regex
		// nolint: errcheck
		_ = os.Setenv(EnvPathRegexPattern, "(unclosed parenthesis")

		config := &Config{}
		err := applyPathConfig(config)
		if err == nil {
			t.Fatal("Expected error for invalid PathRegexPattern, got nil")
		}
	})

	// Test invalid regex pattern for WorkspaceNamespacePathRegex
	t.Run("Invalid WorkspaceNamespacePathRegex", func(t *testing.T) {
		// Save original env vars to restore them later
		origNamespaceRegex := os.Getenv(EnvWorkspaceNamespacePathRegex)
		defer func() {
			// nolint: errcheck
			_ = os.Setenv(EnvWorkspaceNamespacePathRegex, origNamespaceRegex)
		}()

		// Set invalid regex
		// nolint: errcheck
		_ = os.Setenv(EnvWorkspaceNamespacePathRegex, "(unclosed parenthesis")

		config := &Config{}
		err := applyPathConfig(config)
		if err == nil {
			t.Fatal("Expected error for invalid WorkspaceNamespacePathRegex, got nil")
		}
	})

	// Test invalid regex pattern for WorkspaceNamePathRegex
	t.Run("Invalid WorkspaceNamePathRegex", func(t *testing.T) {
		// Save original env vars to restore them later
		origNameRegex := os.Getenv(EnvWorkspaceNamePathRegex)
		defer func() {
			// nolint: errcheck
			_ = os.Setenv(EnvWorkspaceNamePathRegex, origNameRegex)
		}()

		// Set invalid regex
		// nolint: errcheck
		_ = os.Setenv(EnvWorkspaceNamePathRegex, "(unclosed parenthesis")

		config := &Config{}
		err := applyPathConfig(config)
		if err == nil {
			t.Fatal("Expected error for invalid WorkspaceNamePathRegex, got nil")
		}
	})
}

// TestExtractAppPath verifies that app paths are correctly extracted using the regex pattern
func TestExtractAppPath(t *testing.T) {
	// Use the default regex pattern for testing
	regex := DefaultPathRegexPattern

	testCases := []struct {
		name     string
		path     string
		expected string
	}{
		{
			name:     "Empty path",
			path:     "",
			expected: "",
		},
		{
			name:     "Root path",
			path:     "/",
			expected: "/",
		},
		{
			name:     "Non-workspace path",
			path:     "ns1/app",
			expected: "ns1/app",
		},
		{
			name:     "Non-workspace path with leading slash",
			path:     "/ns1/app",
			expected: "/ns1/app",
		},
		{
			name:     "Workspace path without trailing slash",
			path:     "/workspaces/ns1/app1",
			expected: "/workspaces/ns1/app1",
		},
		{
			name:     "Workspace path with trailing slash",
			path:     "/workspaces/ns1/app1/",
			expected: "/workspaces/ns1/app1",
		},
		{
			name:     "Workspace path with subpath",
			path:     "/workspaces/ns1/app1/lab",
			expected: "/workspaces/ns1/app1",
		},
		{
			name:     "Workspace path with complex subpath",
			path:     "/workspaces/ns1/app1/some/addl/path/elements",
			expected: "/workspaces/ns1/app1",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := ExtractAppPath(tc.path, regex)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}

	// Test with empty regex
	t.Run("Empty regex", func(t *testing.T) {
		path := testLabPath
		result := ExtractAppPath(path, "")
		if result != path {
			t.Errorf("Expected path %q to be returned unchanged, got %q", path, result)
		}
	})

	// Test with invalid regex
	t.Run("Invalid regex", func(t *testing.T) {
		path := testLabPath
		result := ExtractAppPath(path, "(unclosed parenthesis")
		if result != path {
			t.Errorf("Expected path %q to be returned unchanged for invalid regex, got %q", path, result)
		}
	})
}
