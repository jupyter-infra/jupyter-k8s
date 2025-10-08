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
			PathRegexPattern: "",
			MaxCookiePaths:   0,
		}

		err := applyPathConfig(config)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if config.PathRegexPattern != DefaultPathRegexPattern {
			t.Errorf("Expected PathRegexPattern to be set to default %q, got %q",
				DefaultPathRegexPattern, config.PathRegexPattern)
		}

		if config.MaxCookiePaths != DefaultMaxCookiePaths {
			t.Errorf("Expected MaxCookiePaths to be set to default %d, got %d",
				DefaultMaxCookiePaths, config.MaxCookiePaths)
		}
	})

	// Test that env vars override defaults
	t.Run("Environment variables override defaults", func(t *testing.T) {
		// Save original env vars to restore them later
		origPathRegex := os.Getenv(EnvPathRegexPattern)
		origMaxPaths := os.Getenv(EnvMaxCookiePaths)
		// nolint: errcheck
		_ = os.Setenv(EnvPathRegexPattern, origPathRegex)
		// nolint: errcheck
		_ = os.Setenv(EnvMaxCookiePaths, origMaxPaths)

		// Set test values
		testPathRegex := `^(/custom/[^/]+/[^/]+)(?:/.*)?$`
		testMaxPaths := "50"
		// nolint: errcheck
		_ = os.Setenv(EnvPathRegexPattern, testPathRegex)
		// nolint: errcheck
		_ = os.Setenv(EnvMaxCookiePaths, testMaxPaths)

		config := &Config{
			// Initialize with defaults
			PathRegexPattern: DefaultPathRegexPattern,
			MaxCookiePaths:   DefaultMaxCookiePaths,
		}

		err := applyPathConfig(config)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if config.PathRegexPattern != testPathRegex {
			t.Errorf("Expected PathRegexPattern to be %q, got %q",
				testPathRegex, config.PathRegexPattern)
		}

		expectedMaxPaths := 50
		if config.MaxCookiePaths != expectedMaxPaths {
			t.Errorf("Expected MaxCookiePaths to be %d, got %d",
				expectedMaxPaths, config.MaxCookiePaths)
		}
	})

	// Test invalid regex pattern
	t.Run("Invalid regex pattern", func(t *testing.T) {
		// Save original env var to restore it later
		origPathRegex := os.Getenv(EnvPathRegexPattern)
		// nolint: errcheck
		_ = os.Setenv(EnvPathRegexPattern, origPathRegex)

		// Set invalid regex
		// nolint: errcheck
		_ = os.Setenv(EnvPathRegexPattern, "(unclosed parenthesis")

		config := &Config{}
		err := applyPathConfig(config)
		if err == nil {
			t.Fatal("Expected error for invalid regex, got nil")
		}
	})

	// Test invalid MaxCookiePaths value
	t.Run("Invalid MaxCookiePaths value", func(t *testing.T) {
		// Save original env var to restore it later
		origMaxPaths := os.Getenv(EnvMaxCookiePaths)
		// nolint: errcheck
		_ = os.Setenv(EnvMaxCookiePaths, origMaxPaths)

		// Test non-numeric value
		// nolint: errcheck
		_ = os.Setenv(EnvMaxCookiePaths, "not-a-number")
		config := &Config{}
		err := applyPathConfig(config)
		if err == nil {
			t.Fatal("Expected error for non-numeric MaxCookiePaths, got nil")
		}

		// Test negative value
		// nolint: errcheck
		_ = os.Setenv(EnvMaxCookiePaths, "-5")
		config = &Config{}
		err = applyPathConfig(config)
		if err == nil {
			t.Fatal("Expected error for negative MaxCookiePaths, got nil")
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
