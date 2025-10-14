package authmiddleware

import (
	"fmt"
	"testing"
)

// TestModuloCookieHashing tests the modulo-based cookie hash implementation
func TestModuloCookieHashing(t *testing.T) {
	// Define the maximum number of paths
	maxPaths := 20

	// Create a map to count how many paths are mapped to each bucket
	bucketCounts := make(map[string]int)

	// Generate a set of test paths that should be distributed across the buckets
	testPaths := []string{}
	for ns := 1; ns <= 10; ns++ {
		for app := 1; app <= 10; app++ {
			testPaths = append(testPaths, fmt.Sprintf("/workspaces/ns%d/app%d", ns, app))
		}
	}

	// Count how many paths are mapped to each bucket
	for _, path := range testPaths {
		hash := HashPath(path, `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`, maxPaths)
		bucketCounts[hash]++
	}

	// Verify that we're using the modulo correctly - should have no more than maxPaths buckets
	if len(bucketCounts) > maxPaths {
		t.Errorf("Expected at most %d buckets, got %d", maxPaths, len(bucketCounts))
	}

	// Print distribution for inspection
	t.Logf("Distribution of %d paths across %d buckets:", len(testPaths), len(bucketCounts))
	for bucket, count := range bucketCounts {
		t.Logf("  %s: %d paths", bucket, count)
	}

	// Test that the same path always hashes to the same bucket regardless of subpath
	basePath := "/workspaces/test/app"
	baseHash := HashPath(basePath, `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`, maxPaths)

	subPaths := []string{
		"/workspaces/test/app/lab",
		"/workspaces/test/app/tree",
		"/workspaces/test/app/notebook/file.ipynb",
		"/workspaces/test/app/console?param=value",
	}

	for _, subPath := range subPaths {
		subHash := HashPath(subPath, `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`, maxPaths)
		if subHash != baseHash {
			t.Errorf("Expected same hash for %q and %q, got %q and %q",
				basePath, subPath, baseHash, subHash)
		}
	}
}

// TestZeroMaxCookiePaths tests behavior when maxPaths is set to 0 or negative
func TestZeroMaxCookiePaths(t *testing.T) {
	path := "/workspaces/test/app"

	// With maxPaths=0, should fall back to non-modulo based hash
	hash0 := HashPath(path, "", 0)
	hashNeg := HashPath(path, "", -5)

	// Both should be the same (no modulo applied)
	if hash0 != hashNeg {
		t.Errorf("Expected same hash for maxPaths=0 and maxPaths=-5, got %q and %q",
			hash0, hashNeg)
	}

	// Both should NOT have the "bucket" prefix
	if len(hash0) >= 6 && hash0[:6] == "bucket" {
		t.Errorf("Expected non-modulo hash without bucket prefix, got %q", hash0)
	}
}
