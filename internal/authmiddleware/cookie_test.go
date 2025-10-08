package authmiddleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNewCookieManagerSameSite verifies that NewCookieManager configures SameSite correctly
func TestNewCookieManagerSameSite(t *testing.T) {
	testCases := []struct {
		name      string
		sameSite  string
		expectErr bool
	}{
		{
			name:      "Strict SameSite",
			sameSite:  SameSiteStrict,
			expectErr: false,
		},
		{
			name:      "Lax SameSite",
			sameSite:  SameSiteLax,
			expectErr: false,
		},
		{
			name:      "None SameSite",
			sameSite:  SameSiteNone,
			expectErr: false,
		},
		{
			name:      "Invalid SameSite",
			sameSite:  "invalid",
			expectErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			config := &Config{
				CookieName:       "test_auth",
				CookieSecure:     true,
				CookiePath:       "/",
				CookieMaxAge:     1 * time.Hour,
				CookieHTTPOnly:   true,
				CookieSameSite:   tc.sameSite,
				CSRFAuthKey:      "01234567890123456789012345678901", // 32-byte key
				CSRFCookieName:   "test_csrf",
				CSRFFieldName:    "csrf_token",
				CSRFHeaderName:   "X-CSRF-Token",
				CSRFCookieMaxAge: 1 * time.Hour,
				CSRFCookieSecure: true,
			}

			manager, err := NewCookieManager(config)

			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected error for SameSite %q but got none", tc.sameSite)
				}
				return
			}

			if err != nil {
				t.Fatalf("Failed to create cookie manager: %v", err)
			}

			if manager.csrfProtect == nil {
				t.Errorf("CSRF protect function is nil")
			}

			// We can't directly test the CSRF protect options since they're wrapped inside the function,
			// but we can verify that the manager fields are correctly set
			var expectedHttpSameSite http.SameSite
			switch tc.sameSite {
			case "strict":
				expectedHttpSameSite = http.SameSiteStrictMode
			case "none":
				expectedHttpSameSite = http.SameSiteNoneMode
			case "lax":
				expectedHttpSameSite = http.SameSiteLaxMode
			}

			if manager.cookieSameSiteHttp != expectedHttpSameSite {
				t.Errorf("Expected HTTP SameSite %v but got %v", expectedHttpSameSite, manager.cookieSameSiteHttp)
			}
		})
	}
}

// TestDynamicCookiePath tests that the cookie path is set based on the request path
func TestDynamicCookiePath(t *testing.T) {
	// Create a test config
	config := &Config{
		CookieName:       "test_auth",
		CookieSecure:     true,
		CookieDomain:     "example.com",
		CookiePath:       "/",
		CookieMaxAge:     1 * time.Hour,
		CookieHTTPOnly:   true,
		CookieSameSite:   SameSiteLax,
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
		MaxCookiePaths:   20,                                 // Maximum of 20 different cookie paths
		CSRFAuthKey:      "01234567890123456789012345678901", // 32-byte key
		CSRFCookieName:   "test_csrf",
		CSRFFieldName:    "csrf_token",
	}

	// Create a cookie manager
	manager, err := NewCookieManager(config)
	if err != nil {
		t.Fatalf("Failed to create cookie manager: %v", err)
	}

	testCases := []struct {
		name            string
		path            string
		expectedPath    string
		expectedAppPath string
	}{
		{
			name:            "Root path",
			path:            "/",
			expectedPath:    "/",
			expectedAppPath: "/",
		},
		{
			name:            "App path",
			path:            "/workspaces/namespace1/app1",
			expectedPath:    "/workspaces/namespace1/app1",
			expectedAppPath: "/workspaces/namespace1/app1",
		},
		{
			name:            "App subpath",
			path:            "/workspaces/namespace1/app1/lab",
			expectedPath:    "/workspaces/namespace1/app1",
			expectedAppPath: "/workspaces/namespace1/app1",
		},
		{
			name:            "Deep subpath",
			path:            "/workspaces/namespace1/app1/notebook/nb1.ipynb",
			expectedPath:    "/workspaces/namespace1/app1",
			expectedAppPath: "/workspaces/namespace1/app1",
		},
		{
			name:            "Non-matching path",
			path:            "/api/v1/status",
			expectedPath:    "/api/v1/status", // For non-matching paths, we use the path as is
			expectedAppPath: "/api/v1/status",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test with SetCookie
			w := httptest.NewRecorder()
			manager.SetCookie(w, "test-token", tc.path)
			cookies := w.Result().Cookies()
			if len(cookies) == 0 {
				t.Fatal("No cookie set")
			}
			cookie := cookies[0]

			if cookie.Path != tc.expectedPath {
				t.Errorf("Cookie path mismatch: got %q, want %q", cookie.Path, tc.expectedPath)
			}

			// Check only that the same paths for same apps get the same cookie name
			// The actual cookie name depends on the hash algorithm and is tested separately

			// Test appPath extraction
			appPath := ExtractAppPath(tc.path, config.PathRegexPattern)
			if appPath != tc.expectedAppPath {
				t.Errorf("App path mismatch: got %q, want %q", appPath, tc.expectedAppPath)
			}
		})
	}
}

// TestCookieWithDifferentPathsButSameApp tests that cookies with the same app path but different subpaths share the same cookie name
func TestCookieWithDifferentPathsButSameApp(t *testing.T) {
	// Create a test config
	config := &Config{
		CookieName:       "test_auth",
		CookieSecure:     true,
		CookiePath:       "/",
		CookieMaxAge:     1 * time.Hour,
		CookieHTTPOnly:   true,
		CookieSameSite:   SameSiteLax,
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
		MaxCookiePaths:   20,                                 // Maximum of 20 different cookie paths
		CSRFAuthKey:      "01234567890123456789012345678901", // 32-byte key
	}

	// Create a cookie manager
	manager, err := NewCookieManager(config)
	if err != nil {
		t.Fatalf("Failed to create cookie manager: %v", err)
	}

	basePath := testAppPath
	subPaths := []string{
		"/workspaces/ns1/app1/lab",
		"/workspaces/ns1/app1/tree",
		"/workspaces/ns1/app1/notebook/nb1.ipynb",
		"/workspaces/ns1/app1/console",
	}

	// First set a cookie with the base path
	baseWriter := httptest.NewRecorder()
	manager.SetCookie(baseWriter, "base-token", basePath)
	baseCookies := baseWriter.Result().Cookies()
	if len(baseCookies) == 0 {
		t.Fatal("No base cookie set")
	}
	baseCookie := baseCookies[0]

	// Now test that cookies with the same app but different subpaths share the same name and path
	for _, subPath := range subPaths {
		t.Run(subPath, func(t *testing.T) {
			subWriter := httptest.NewRecorder()
			manager.SetCookie(subWriter, "sub-token", subPath)
			subCookies := subWriter.Result().Cookies()
			if len(subCookies) == 0 {
				t.Fatal("No sub cookie set")
			}
			subCookie := subCookies[0]

			if subCookie.Name != baseCookie.Name {
				t.Errorf("Cookie name mismatch: got %q, want %q", subCookie.Name, baseCookie.Name)
			}

			if subCookie.Path != baseCookie.Path {
				t.Errorf("Cookie path mismatch: got %q, want %q", subCookie.Path, baseCookie.Path)
			}
		})
	}
}

// TestSetCookieNameAndPath verifies that SetCookie sets the cookie with the expected name and path
func TestSetCookieNameAndPath(t *testing.T) {
	// Create a test config
	config := &Config{
		CookieName:       "test_auth",
		CookieSecure:     true,
		CookieDomain:     "example.com",
		CookiePath:       "/",
		CookieMaxAge:     1 * time.Hour,
		CookieHTTPOnly:   true,
		CookieSameSite:   SameSiteLax,
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
		MaxCookiePaths:   20,                                 // Maximum of 20 different cookie paths
		CSRFAuthKey:      "01234567890123456789012345678901", // 32-byte key
	}

	// Create a cookie manager
	manager, err := NewCookieManager(config)
	if err != nil {
		t.Fatalf("Failed to create cookie manager: %v", err)
	}

	testCases := []struct {
		name         string
		path         string
		expectedPath string
		token        string
	}{
		{
			name:         "Root path",
			path:         "/",
			expectedPath: "/",
			token:        "token-root",
		},
		{
			name:         "App path",
			path:         "/workspaces/namespace1/app1",
			expectedPath: "/workspaces/namespace1/app1",
			token:        "token-app",
		},
		{
			name:         "App subpath",
			path:         "/workspaces/namespace1/app1/lab",
			expectedPath: "/workspaces/namespace1/app1",
			token:        "token-subpath",
		},
		{
			name:         "Empty path",
			path:         "",
			expectedPath: "/",
			token:        "token-empty",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			manager.SetCookie(w, tc.token, tc.path)
			cookies := w.Result().Cookies()
			if len(cookies) == 0 {
				t.Fatal("No cookie set")
			}
			cookie := cookies[0]

			// Check that the cookie path is as expected
			if cookie.Path != tc.expectedPath {
				t.Errorf("Expected cookie path %q but got %q", tc.expectedPath, cookie.Path)
			}

			// Check that the cookie value is the token
			if cookie.Value != tc.token {
				t.Errorf("Expected cookie value %q but got %q", tc.token, cookie.Value)
			}

			// Check that the cookie has the correct domain
			if cookie.Domain != config.CookieDomain {
				t.Errorf("Expected cookie domain %q but got %q", config.CookieDomain, cookie.Domain)
			}

			// Check that the cookie has the correct max age
			expectedMaxAge := int(config.CookieMaxAge.Seconds())
			if cookie.MaxAge != expectedMaxAge {
				t.Errorf("Expected cookie max age %d but got %d", expectedMaxAge, cookie.MaxAge)
			}

			// Check that the cookie has the correct secure flag
			if cookie.Secure != config.CookieSecure {
				t.Errorf("Expected cookie secure %t but got %t", config.CookieSecure, cookie.Secure)
			}

			// Check that the cookie has the correct HTTP only flag
			if cookie.HttpOnly != config.CookieHTTPOnly {
				t.Errorf("Expected cookie HTTP only %t but got %t", config.CookieHTTPOnly, cookie.HttpOnly)
			}
		})
	}
}

// TestGetCookieWithPath verifies that GetCookie retrieves the cookie with the correct name based on path
func TestGetCookieWithPath(t *testing.T) {
	// Create a test config
	config := &Config{
		CookieName:       "test_auth",
		CookieSecure:     true,
		CookieDomain:     "example.com",
		CookiePath:       "/",
		CookieMaxAge:     1 * time.Hour,
		CookieHTTPOnly:   true,
		CookieSameSite:   SameSiteLax,
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
		MaxCookiePaths:   20,                                 // Maximum of 20 different cookie paths
		CSRFAuthKey:      "01234567890123456789012345678901", // 32-byte key
	}

	// Create a cookie manager
	manager, err := NewCookieManager(config)
	if err != nil {
		t.Fatalf("Failed to create cookie manager: %v", err)
	}

	testCases := []struct {
		name  string
		path  string
		token string
	}{
		{
			name:  "Root path",
			path:  "/",
			token: "token-root",
		},
		{
			name:  "App path",
			path:  "/workspaces/namespace1/app1",
			token: "token-app",
		},
		{
			name:  "App subpath",
			path:  "/workspaces/namespace1/app1/lab",
			token: "token-subpath",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// First set the cookie
			w := httptest.NewRecorder()
			manager.SetCookie(w, tc.token, tc.path)

			// Create a request with the cookie
			req := httptest.NewRequest("GET", "http://example.com"+tc.path, nil)
			for _, cookie := range w.Result().Cookies() {
				req.AddCookie(cookie)
			}

			// Now try to get the cookie
			token, err := manager.GetCookie(req, tc.path)
			if err != nil {
				t.Fatalf("Failed to get cookie: %v", err)
			}

			// Check that the token matches
			if token != tc.token {
				t.Errorf("Expected token %q but got %q", tc.token, token)
			}
		})
	}
}

// TestGetCookieNotFound verifies that GetCookie returns ErrNoCookie if the cookie is not found
func TestGetCookieNotFound(t *testing.T) {
	// Create a test config
	config := &Config{
		CookieName:       "test_auth",
		CookieSecure:     true,
		CookieDomain:     "example.com",
		CookiePath:       "/",
		CookieMaxAge:     1 * time.Hour,
		CookieHTTPOnly:   true,
		CookieSameSite:   SameSiteLax,
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
		MaxCookiePaths:   20,                                 // Maximum of 20 different cookie paths
		CSRFAuthKey:      "01234567890123456789012345678901", // 32-byte key
	}

	// Create a cookie manager
	manager, err := NewCookieManager(config)
	if err != nil {
		t.Fatalf("Failed to create cookie manager: %v", err)
	}

	// Create a request without a cookie
	req := httptest.NewRequest("GET", "http://example.com/workspaces/ns1/app1", nil)

	// Try to get the cookie
	_, err = manager.GetCookie(req, "/workspaces/ns1/app1")

	// Check that the error is ErrNoCookie
	if err != ErrNoCookie {
		t.Errorf("Expected ErrNoCookie but got %v", err)
	}
}

// TestClearCookie verifies that ClearCookie sets a cookie with an empty value and negative max age
func TestClearCookie(t *testing.T) {
	// Create a test config
	config := &Config{
		CookieName:       "test_auth",
		CookieSecure:     true,
		CookieDomain:     "example.com",
		CookiePath:       "/",
		CookieMaxAge:     1 * time.Hour,
		CookieHTTPOnly:   true,
		CookieSameSite:   SameSiteLax,
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
		MaxCookiePaths:   20,                                 // Maximum of 20 different cookie paths
		CSRFAuthKey:      "01234567890123456789012345678901", // 32-byte key
	}

	// Create a cookie manager
	manager, err := NewCookieManager(config)
	if err != nil {
		t.Fatalf("Failed to create cookie manager: %v", err)
	}

	testCases := []struct {
		name string
		path string
	}{
		{
			name: "Root path",
			path: "/",
		},
		{
			name: "App path",
			path: "/workspaces/namespace1/app1",
		},
		{
			name: "App subpath",
			path: "/workspaces/namespace1/app1/lab",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// First set a cookie with a value
			w := httptest.NewRecorder()
			manager.SetCookie(w, "test-token", tc.path)

			// Now clear the cookie
			clearW := httptest.NewRecorder()
			manager.ClearCookie(clearW, tc.path)

			// Get the cleared cookie
			cookies := clearW.Result().Cookies()
			if len(cookies) == 0 {
				t.Fatal("No cookie set")
			}
			cookie := cookies[0]

			// Check that the cookie value is empty
			if cookie.Value != "" {
				t.Errorf("Expected empty cookie value but got %q", cookie.Value)
			}

			// Check that the cookie max age is negative (expires immediately)
			if cookie.MaxAge != -1 {
				t.Errorf("Expected cookie max age -1 but got %d", cookie.MaxAge)
			}

			// Check that the cookie path is correct based on the input path
			expectedPath := "/"
			if tc.path != "/" {
				appPath := ExtractAppPath(tc.path, config.PathRegexPattern)
				if appPath != "" && appPath != "/" {
					expectedPath = appPath
				}
			}

			if cookie.Path != expectedPath {
				t.Errorf("Expected cookie path %q but got %q", expectedPath, cookie.Path)
			}
		})
	}
}

// TestGetCSRFCookiePath verifies that GetCSRFCookiePath always returns the static cookie path
func TestGetCSRFCookiePath(t *testing.T) {
	// Create a test config with a custom cookie path
	customPath := "/custom-cookie-path"
	config := &Config{
		CookieName:       "test_auth",
		CookieSecure:     true,
		CookiePath:       customPath, // Set a custom path to verify it's returned
		CookieMaxAge:     1 * time.Hour,
		CookieHTTPOnly:   true,
		CookieSameSite:   SameSiteLax,
		PathRegexPattern: `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`,
		MaxCookiePaths:   20,
		CSRFAuthKey:      "01234567890123456789012345678901",
		CSRFCookieName:   "test_csrf",
		CSRFCookieMaxAge: 1 * time.Hour,
		CSRFCookieSecure: true,
		CSRFFieldName:    "csrf_token",
		CSRFHeaderName:   "X-CSRF-Token",
	}

	// Create a cookie manager
	manager, err := NewCookieManager(config)
	if err != nil {
		t.Fatalf("Failed to create cookie manager: %v", err)
	}

	testCases := []struct {
		name string
		path string
	}{
		{
			name: "Empty path",
			path: "",
		},
		{
			name: "Root path",
			path: "/",
		},
		{
			name: "App path",
			path: "/workspaces/namespace1/app1",
		},
		{
			name: "App subpath",
			path: "/workspaces/namespace1/app1/lab",
		},
		{
			name: "Non-matching path",
			path: "/api/v1/status",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Call GetCSRFCookiePath
			csrfPath := manager.GetCSRFCookiePath(tc.path)

			// Check that the returned path is always the static cookie path
			// regardless of the input path
			if csrfPath != customPath {
				t.Errorf("Expected CSRF cookie path to always be %q for any input, but got %q", customPath, csrfPath)
			}
		})
	}
}
