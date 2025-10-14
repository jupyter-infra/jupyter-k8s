package authmiddleware

import (
	"fmt"
	"testing"
)

// Constants for test paths
const (
	testAppPath = "/workspaces/ns1/app1"
)

func TestPathRegexExtraction(t *testing.T) {
	testCases := []struct {
		path        string
		regex       string
		expected    string
		description string
	}{
		{
			path:        "/workspaces/namespace1/app1",
			regex:       `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
			expected:    "/workspaces/namespace1/app1",
			description: "Basic app path",
		},
		{
			path:        "/workspaces/namespace1/app1/",
			regex:       `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
			expected:    "/workspaces/namespace1/app1",
			description: "App path with trailing slash",
		},
		{
			path:        "/workspaces/namespace1/app1/lab",
			regex:       `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
			expected:    "/workspaces/namespace1/app1",
			description: "App path with lab subpath",
		},
		{
			path:        "/workspaces/namespace1/app1/notebook/nb1.ipynb",
			regex:       `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
			expected:    "/workspaces/namespace1/app1",
			description: "App path with notebook subpath",
		},
		{
			path:        "/invalid/path",
			regex:       `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
			expected:    "/invalid/path",
			description: "Invalid path not matching regex",
		},
		{
			path:        "/workspaces/namespace-very-long-name/app-with-very-long-name",
			regex:       `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
			expected:    "/workspaces/namespace-very-long-name/app-with-very-long-name",
			description: "App path with long names",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			result := ExtractAppPath(tc.path, tc.regex)
			if result != tc.expected {
				t.Errorf("Expected %q but got %q", tc.expected, result)
			}
		})
	}
}

func TestCookieNameGeneration(t *testing.T) {
	testCases := []struct {
		baseName    string
		path        string
		regex       string
		maxPaths    int
		expectSame  bool
		description string
	}{
		{
			baseName:    "wkspc_auth",
			path:        "/workspaces/ns1/app1/lab",
			regex:       `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
			maxPaths:    20,
			expectSame:  true,
			description: "Different subpaths should have same cookie name",
		},
		{
			baseName:    "wkspc_auth",
			path:        "/workspaces/ns1/app1/notebook/nb1.ipynb",
			regex:       `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
			maxPaths:    20,
			expectSame:  true,
			description: "Deep subpaths should have same cookie name",
		},
		{
			baseName:    "wkspc_auth",
			path:        "/workspaces/ns1/app2/lab",
			regex:       `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
			maxPaths:    20,
			expectSame:  false,
			description: "Different apps should have different cookie names",
		},
		{
			baseName:    "wkspc_auth",
			path:        "/workspaces/ns2/app1/lab",
			regex:       `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
			maxPaths:    20,
			expectSame:  false,
			description: "Different namespaces should have different cookie names",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			basePath := testAppPath
			baseCookieName := GetCookieName(tc.baseName, basePath, tc.regex, tc.maxPaths)
			testCookieName := GetCookieName(tc.baseName, tc.path, tc.regex, tc.maxPaths)

			if tc.expectSame {
				if baseCookieName != testCookieName {
					t.Errorf("Expected cookie names to be the same, but got %q and %q",
						baseCookieName, testCookieName)
				}
			} else {
				if baseCookieName == testCookieName {
					t.Errorf("Expected cookie names to be different, but both are %q",
						baseCookieName)
				}
			}

			// Print cookie names for inspection
			fmt.Printf("Path: %-40s Cookie: %s\n", tc.path, testCookieName)
		})
	}
}
