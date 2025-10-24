// Package authmiddleware provides JWT-based authentication and authorization middleware
// for Jupyter-k8s workspaces, handling user identity, cookie management, and CSRF protection.
package authmiddleware

import (
	"fmt"
	"os"
	"regexp"
)

// applyPathConfig applies path-related environment variable overrides
// and ensures default values are set if not provided
func applyPathConfig(config *Config) error {
	// Set defaults if not already set
	if config.PathRegexPattern == "" {
		config.PathRegexPattern = DefaultPathRegexPattern
	}
	if config.WorkspaceNamespacePathRegex == "" {
		config.WorkspaceNamespacePathRegex = DefaultWorkspaceNamespacePathRegex
	}
	if config.WorkspaceNamePathRegex == "" {
		config.WorkspaceNamePathRegex = DefaultWorkspaceNamePathRegex
	}

	// Override with environment variables if provided
	if pathRegex := os.Getenv(EnvPathRegexPattern); pathRegex != "" {
		// Validate that the regex compiles
		_, err := regexp.Compile(pathRegex)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvPathRegexPattern, err)
		}
		config.PathRegexPattern = pathRegex
	}

	// Override workspace namespace path regex if provided
	if namespaceRegex := os.Getenv(EnvWorkspaceNamespacePathRegex); namespaceRegex != "" {
		// Validate that the regex compiles
		_, err := regexp.Compile(namespaceRegex)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvWorkspaceNamespacePathRegex, err)
		}
		config.WorkspaceNamespacePathRegex = namespaceRegex
	}

	// Override workspace name path regex if provided
	if nameRegex := os.Getenv(EnvWorkspaceNamePathRegex); nameRegex != "" {
		// Validate that the regex compiles
		_, err := regexp.Compile(nameRegex)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvWorkspaceNamePathRegex, err)
		}
		config.WorkspaceNamePathRegex = nameRegex
	}

	return nil
}

// ExtractAppPath extracts the application path from a full URL path using the configured regex pattern
// Returns the extracted path or the original path if no match is found
func ExtractAppPath(fullPath string, regexPattern string) string {
	if fullPath == "" || regexPattern == "" {
		return fullPath
	}

	// Compile the regex
	re, err := regexp.Compile(regexPattern)
	if err != nil {
		// If regex is invalid, return the original path
		return fullPath
	}

	// Find the first match
	matches := re.FindStringSubmatch(fullPath)
	if len(matches) >= 2 {
		// The first capturing group contains our app path
		return matches[1]
	}

	// If no match found, return the original path
	return fullPath
}
