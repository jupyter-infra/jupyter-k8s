package authmiddleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SameSite mode constants
const (
	SameSiteStrict = "strict"
	SameSiteNone   = "none"
	SameSiteLax    = "lax"
)

// Common errors
var (
	ErrNoCookie      = errors.New("cookie not found")
	ErrInvalidCookie = errors.New("invalid cookie")
	ErrCSRFMismatch  = errors.New("CSRF token mismatch")
)

// CookieHandler exposes CookieManager interface to facilitate unit-testing
type CookieHandler interface {
	SetCookie(w http.ResponseWriter, token string, path string, domain string)
	GetCookie(r *http.Request, path string) (string, error)
	ClearCookie(w http.ResponseWriter, path string, domain string)
}

// CookieManager handles cookie operations
type CookieManager struct {
	cookieName         string
	cookieSecure       bool
	cookieDomain       string
	cookiePath         string
	cookieMaxAge       time.Duration
	cookieHTTPOnly     bool
	cookieSameSiteHttp http.SameSite
	pathRegexPattern   string // Regex pattern for path-based cookie naming
}

// NewCookieManager creates a new CookieManager
func NewCookieManager(cfg *Config) (*CookieManager, error) {
	// Parse SameSite value
	var sameSiteHttp http.SameSite
	switch strings.ToLower(cfg.CookieSameSite) {
	case SameSiteStrict:
		sameSiteHttp = http.SameSiteStrictMode
	case SameSiteNone:
		sameSiteHttp = http.SameSiteNoneMode
	case SameSiteLax:
		sameSiteHttp = http.SameSiteLaxMode
	default:
		return nil, fmt.Errorf("invalid same site value: %s", cfg.CookieSameSite)
	}

	return &CookieManager{
		cookieName:         cfg.CookieName,
		cookieSecure:       cfg.CookieSecure,
		cookieDomain:       cfg.CookieDomain,
		cookiePath:         cfg.CookiePath,
		cookieMaxAge:       cfg.CookieMaxAge,
		cookieHTTPOnly:     cfg.CookieHTTPOnly,
		cookieSameSiteHttp: sameSiteHttp,
		pathRegexPattern:   cfg.PathRegexPattern,
	}, nil
}

// SetCookie sets an auth cookie with the given token
func (m *CookieManager) SetCookie(w http.ResponseWriter, token string, path string, domain string) {
	cookieName := m.cookieName
	cookiePath := m.cookiePath

	// Extract the app path for cookie path only
	if path != "" {
		// Extract the app path using the regex pattern
		appPath := path
		if m.pathRegexPattern != "" {
			appPath = ExtractAppPath(path, m.pathRegexPattern)
		}

		// Use the extracted app path for cookie path if it's not empty
		if appPath != "" && appPath != "/" {
			cookiePath = appPath
		}
	}

	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     cookiePath,
		Domain:   domain,
		MaxAge:   int(m.cookieMaxAge.Seconds()),
		HttpOnly: m.cookieHTTPOnly,
		Secure:   m.cookieSecure,
		SameSite: m.cookieSameSiteHttp,
	}

	http.SetCookie(w, cookie)
}

// GetCookie retrieves the auth token from the cookie
func (m *CookieManager) GetCookie(r *http.Request, path string) (string, error) {
	cookieName := m.cookieName

	cookie, err := r.Cookie(cookieName)
	if err != nil {
		if err == http.ErrNoCookie {
			return "", ErrNoCookie
		}
		return "", fmt.Errorf("%w: %v", ErrInvalidCookie, err)
	}

	return cookie.Value, nil
}

// ClearCookie removes the auth cookie
func (m *CookieManager) ClearCookie(w http.ResponseWriter, path string, domain string) {
	cookieName := m.cookieName
	cookiePath := m.cookiePath

	// Extract the app path for cookie path only
	if path != "" {
		// Extract the app path using the regex pattern
		appPath := path
		if m.pathRegexPattern != "" {
			appPath = ExtractAppPath(path, m.pathRegexPattern)
		}

		// Use the extracted app path for cookie path if it's not empty
		if appPath != "" && appPath != "/" {
			cookiePath = appPath
		}
	}

	cookie := &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     cookiePath,
		Domain:   domain,
		MaxAge:   -1,
		HttpOnly: m.cookieHTTPOnly,
		Secure:   m.cookieSecure,
		SameSite: m.cookieSameSiteHttp,
	}

	http.SetCookie(w, cookie)
}
