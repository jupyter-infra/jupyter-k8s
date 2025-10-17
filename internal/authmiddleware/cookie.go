package authmiddleware

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/csrf"
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
	SetCookie(w http.ResponseWriter, token string, path string)
	GetCookie(r *http.Request, path string) (string, error)
	ClearCookie(w http.ResponseWriter, path string)
	CSRFProtect() func(http.Handler) http.Handler
}

// CookieManager handles cookie operations
type CookieManager struct {
	cookieName         string
	cookieSecure       bool
	cookieDomain       string
	cookiePath         string
	cookieMaxAge       time.Duration
	cookieHTTPOnly     bool
	cookieSameSite     csrf.SameSiteMode
	cookieSameSiteHttp http.SameSite
	pathRegexPattern   string // Regex pattern for path-based cookie naming
	csrfCookieName     string
	csrfCookieMaxAge   time.Duration
	csrfCookieSecure   bool
	csrfFieldName      string
	csrfHeaderName     string
	csrfTrustedOrigins []string
	csrfProtect        func(http.Handler) http.Handler
}

// NewCookieManager creates a new CookieManager
func NewCookieManager(cfg *Config) (*CookieManager, error) {
	// Parse SameSite value
	var sameSite csrf.SameSiteMode
	var sameSiteHttp http.SameSite
	switch strings.ToLower(cfg.CookieSameSite) {
	case SameSiteStrict:
		sameSite = csrf.SameSiteDefaultMode
		sameSiteHttp = http.SameSiteStrictMode
	case SameSiteNone:
		sameSite = csrf.SameSiteNoneMode
		sameSiteHttp = http.SameSiteNoneMode
	case SameSiteLax:
		sameSite = csrf.SameSiteLaxMode
		sameSiteHttp = http.SameSiteLaxMode
	default:
		return nil, fmt.Errorf("invalid same site value: %s", cfg.CookieSameSite)
	}

	// Validate CSRF auth key
	if len(cfg.CSRFAuthKey) < 32 {
		return nil, errors.New("CSRF auth key must be at least 32 bytes")
	}

	csrfOpts := []csrf.Option{
		csrf.CookieName(cfg.CSRFCookieName),
		csrf.Path(cfg.CookiePath),
		csrf.MaxAge(int(cfg.CSRFCookieMaxAge.Seconds())),
		csrf.Secure(cfg.CSRFCookieSecure),
		csrf.SameSite(sameSite),
		csrf.FieldName(cfg.CSRFFieldName),
		csrf.RequestHeader(cfg.CSRFHeaderName),
	}

	if cfg.CookieDomain != "" {
		csrfOpts = append(csrfOpts, csrf.Domain(cfg.CookieDomain)) // Use regular cookie domain
	}

	if len(cfg.CSRFTrustedOrigins) > 0 {
		csrfOpts = append(csrfOpts, csrf.TrustedOrigins(cfg.CSRFTrustedOrigins))
	}

	csrfProtect := csrf.Protect([]byte(cfg.CSRFAuthKey), csrfOpts...)

	return &CookieManager{
		cookieName:         cfg.CookieName,
		cookieSecure:       cfg.CookieSecure,
		cookieDomain:       cfg.CookieDomain,
		cookiePath:         cfg.CookiePath,
		cookieMaxAge:       cfg.CookieMaxAge,
		cookieHTTPOnly:     cfg.CookieHTTPOnly,
		cookieSameSite:     sameSite,
		cookieSameSiteHttp: sameSiteHttp,
		pathRegexPattern:   cfg.PathRegexPattern,
		csrfCookieName:     cfg.CSRFCookieName,
		csrfCookieMaxAge:   cfg.CSRFCookieMaxAge,
		csrfCookieSecure:   cfg.CSRFCookieSecure,
		csrfFieldName:      cfg.CSRFFieldName,
		csrfHeaderName:     cfg.CSRFHeaderName,
		csrfTrustedOrigins: cfg.CSRFTrustedOrigins,
		csrfProtect:        csrfProtect,
	}, nil
}

// SetCookie sets an auth cookie with the given token
func (m *CookieManager) SetCookie(w http.ResponseWriter, token string, path string) {
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
		MaxAge:   int(m.cookieMaxAge.Seconds()),
		HttpOnly: m.cookieHTTPOnly,
		Secure:   m.cookieSecure,
		SameSite: m.cookieSameSiteHttp,
	}

	if m.cookieDomain != "" {
		cookie.Domain = m.cookieDomain
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
func (m *CookieManager) ClearCookie(w http.ResponseWriter, path string) {
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
		MaxAge:   -1,
		HttpOnly: m.cookieHTTPOnly,
		Secure:   m.cookieSecure,
		SameSite: m.cookieSameSiteHttp,
	}

	if m.cookieDomain != "" {
		cookie.Domain = m.cookieDomain
	}

	http.SetCookie(w, cookie)
}

// CSRFProtect returns a middleware that protects against CSRF attacks
// Note: CSRF cookies are set with the root path ("/") to ensure they're available
// across all workspace paths. While auth cookies are path-specific, CSRF cookies
// need to be accessible for any workspace to prevent CSRF token mismatches.
func (m *CookieManager) CSRFProtect() func(http.Handler) http.Handler {
	return m.csrfProtect
}
