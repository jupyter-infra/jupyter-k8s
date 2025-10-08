// Package authmiddleware provides JWT-based authentication and authorization middleware
// for Jupyter-k8s workspaces, handling user identity, cookie management, and CSRF protection.
package authmiddleware

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Removed hardcoded default prefix in favor of config value

// HashPath generates a hash from the given path
func HashPath(path string, regexPattern string, maxPaths int) string {
	// Clean the path first
	cleanPath := strings.TrimSpace(path)

	// If path is empty or just a slash, use a default value
	if cleanPath == "" || cleanPath == "/" {
		return "default"
	}

	// If regex pattern is provided, extract the app path component
	if regexPattern != "" {
		cleanPath = ExtractAppPath(cleanPath, regexPattern)
	}

	// Remove trailing slash if present
	if len(cleanPath) > 0 && cleanPath[len(cleanPath)-1] == '/' {
		cleanPath = cleanPath[:len(cleanPath)-1]
	}

	// Remove leading slash if present
	if len(cleanPath) > 0 && cleanPath[0] == '/' {
		cleanPath = cleanPath[1:]
	}

	// Replace additional slashes with underscores to make a more readable hash
	cleanPath = strings.ReplaceAll(cleanPath, "/", "_")

	// If maxPaths is provided and greater than 0, use it for modulo hashing
	if maxPaths > 0 {
		// Create a numeric hash of the path
		hash := sha256.Sum256([]byte(cleanPath))
		// Convert first 8 bytes to uint64
		hashValue := uint64(0)
		for _, b := range hash[:8] {
			hashValue = (hashValue << 8) | uint64(b)
		}
		// Use modulo to limit the range
		bucketNumber := hashValue % uint64(maxPaths)
		// Return the bucket number as string
		return fmt.Sprintf("%d", bucketNumber)
	}

	// If the cleaned path is still too long, create a hash
	if len(cleanPath) > 20 {
		hash := sha256.Sum256([]byte(cleanPath))
		return hex.EncodeToString(hash[:])[:16] // Use first 16 chars of hash
	}

	return cleanPath
}

// GetCookieName generates a cookie name based on the given path, regex pattern, and max paths
func GetCookieName(baseName string, path string, regexPattern string, maxPaths int) string {
	// baseName should be passed from the config and not be empty
	if baseName == "" {
		// This is just a safety check, callers should provide a baseName
		baseName = DefaultCookieName
	}

	pathHash := HashPath(path, regexPattern, maxPaths)
	if pathHash == "default" {
		return baseName
	}

	return baseName + "_" + pathHash
}
