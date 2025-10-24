// Package authmiddleware provides JWT-based authentication and authorization middleware
// for Jupyter-k8s workspaces, handling user identity, cookie management, and CSRF protection.
package authmiddleware

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Environment variable names
const (
	// Server configuration
	EnvPort            = "PORT"
	EnvReadTimeout     = "READ_TIMEOUT"
	EnvWriteTimeout    = "WRITE_TIMEOUT"
	EnvShutdownTimeout = "SHUTDOWN_TIMEOUT"
	EnvTrustedProxies  = "TRUSTED_PROXIES"

	// Auth configuration
	EnvJwtSigningKey     = "JWT_SIGNING_KEY"
	EnvJwtIssuer         = "JWT_ISSUER"
	EnvJwtAudience       = "JWT_AUDIENCE"
	EnvJwtExpiration     = "JWT_EXPIRATION"
	EnvJwtRefreshWindow  = "JWT_REFRESH_WINDOW"
	EnvJwtRefreshHorizon = "JWT_REFRESH_HORIZON"
	EnvEnableBearerAuth  = "ENABLE_BEARER_URL_AUTH"

	// Cookie configuration
	EnvCookieName     = "COOKIE_NAME"
	EnvCookieSecure   = "COOKIE_SECURE"
	EnvCookieDomain   = "COOKIE_DOMAIN"
	EnvCookiePath     = "COOKIE_PATH"
	EnvCookieMaxAge   = "COOKIE_MAX_AGE"
	EnvCookieHttpOnly = "COOKIE_HTTP_ONLY"
	EnvCookieSameSite = "COOKIE_SAME_SITE"

	// Path configuration
	EnvPathRegexPattern = "PATH_REGEX_PATTERN"

	// CSRF configuration
	EnvCsrfAuthKey    = "CSRF_AUTH_KEY"
	EnvCsrfCookieName = "CSRF_COOKIE_NAME"
	// Note: CSRF cookies use the same CookiePath and CookieDomain as auth cookies
	EnvCsrfCookieMaxAge   = "CSRF_COOKIE_MAX_AGE"
	EnvCsrfCookieSecure   = "CSRF_COOKIE_SECURE"
	EnvCsrfFieldName      = "CSRF_FIELD_NAME"
	EnvCsrfHeaderName     = "CSRF_HEADER_NAME"
	EnvCsrfTrustedOrigins = "CSRF_TRUSTED_ORIGINS"
)

// Default values
const (
	// Server defaults
	DefaultPort            = 8080
	DefaultReadTimeout     = 10 * time.Second
	DefaultWriteTimeout    = 10 * time.Second
	DefaultShutdownTimeout = 30 * time.Second
	// DefaultTrustedProxies is a slice, defined in createDefaultConfig

	// Auth defaults
	DefaultJwtIssuer         = "workspaces-auth"
	DefaultJwtAudience       = "workspace-users"
	DefaultJwtExpiration     = 1 * time.Hour
	DefaultJwtRefreshWindow  = 15 * time.Minute // 25% of the default expiration
	DefaultJwtRefreshHorizon = 12 * time.Hour
	DefaultEnableBearerAuth  = false

	// Cookie defaults
	DefaultCookieName     = "workspace_auth"
	DefaultCookieSecure   = true
	DefaultCookiePath     = "/"
	DefaultCookieMaxAge   = 24 * time.Hour
	DefaultCookieHttpOnly = true
	DefaultCookieSameSite = SameSiteLax

	// Path defaults
	DefaultPathRegexPattern = `^(/workspaces/[^/]+/[^/]+)(?:/.*)?$`

	// CSRF defaults
	DefaultCsrfCookieName = "workspace_csrf"
	// Note: CSRF cookies use the same CookiePath and CookieDomain as auth cookies
	DefaultCsrfCookieMaxAge = 1 * time.Hour
	DefaultCsrfCookieSecure = true
	DefaultCsrfFieldName    = "csrf_token"
	DefaultCsrfHeaderName   = "X-CSRF-Token"
	// DefaultCsrfTrustedOrigins is a slice, defined in createDefaultConfig
)

// Config holds all configuration for the workspaces-auth service
type Config struct {
	// Server configuration
	Port            int
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
	TrustedProxies  []string

	// Auth configuration
	JWTSigningKey     string
	JWTIssuer         string
	JWTAudience       string
	JWTExpiration     time.Duration
	JWTRefreshWindow  time.Duration
	JWTRefreshHorizon time.Duration
	EnableBearerAuth  bool

	// Cookie configuration
	CookieName     string
	CookieSecure   bool
	CookieDomain   string
	CookiePath     string
	CookieMaxAge   time.Duration
	CookieHTTPOnly bool
	CookieSameSite string

	// Path configuration
	PathRegexPattern string // Regex pattern to extract app path from full path

	// CSRF configuration
	CSRFAuthKey    string
	CSRFCookieName string
	// Note: CSRF cookies use the same CookiePath and CookieDomain as auth cookies
	CSRFCookieMaxAge   time.Duration
	CSRFCookieSecure   bool
	CSRFFieldName      string
	CSRFHeaderName     string
	CSRFTrustedOrigins []string
}

// NewConfig creates a Config with values from environment variables
// or defaults if not set
func NewConfig() (*Config, error) {
	config := createDefaultConfig()

	// Apply overrides from environment variables
	if err := applyServerConfig(config); err != nil {
		return nil, err
	}

	if err := applyJWTConfig(config); err != nil {
		return nil, err
	}

	if err := applyCookieConfig(config); err != nil {
		return nil, err
	}

	if err := applyPathConfig(config); err != nil {
		return nil, err
	}

	if err := applyCSRFConfig(config); err != nil {
		return nil, err
	}

	return config, nil
}

// createDefaultConfig creates a new Config with default values
func createDefaultConfig() *Config {
	return &Config{
		// Server defaults
		Port:            DefaultPort,
		ReadTimeout:     DefaultReadTimeout,
		WriteTimeout:    DefaultWriteTimeout,
		ShutdownTimeout: DefaultShutdownTimeout,
		TrustedProxies:  []string{"127.0.0.1", "::1"}, // Default trusted proxies

		// Auth defaults
		JWTIssuer:         DefaultJwtIssuer,
		JWTAudience:       DefaultJwtAudience,
		JWTExpiration:     DefaultJwtExpiration,
		JWTRefreshWindow:  DefaultJwtRefreshWindow,
		JWTRefreshHorizon: DefaultJwtRefreshHorizon,
		EnableBearerAuth:  DefaultEnableBearerAuth,

		// Cookie defaults
		CookieName:     DefaultCookieName,
		CookieSecure:   DefaultCookieSecure,
		CookiePath:     DefaultCookiePath,
		CookieMaxAge:   DefaultCookieMaxAge,
		CookieHTTPOnly: DefaultCookieHttpOnly,
		CookieSameSite: DefaultCookieSameSite,

		// Path defaults
		// This regex extracts application path: /workspaces/<namespace>/<app-name>
		// It will ignore subpaths like /lab, /tree, /notebook/*, etc.
		PathRegexPattern: DefaultPathRegexPattern,

		// CSRF defaults
		CSRFCookieName:   DefaultCsrfCookieName,
		CSRFCookieMaxAge: DefaultCsrfCookieMaxAge,
		CSRFCookieSecure: DefaultCsrfCookieSecure,
		CSRFFieldName:    DefaultCsrfFieldName,
		CSRFHeaderName:   DefaultCsrfHeaderName,
	}
}

// applyServerConfig applies server-related environment variable overrides
func applyServerConfig(config *Config) error {
	if port := os.Getenv(EnvPort); port != "" {
		p, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvPort, err)
		}
		config.Port = p
	}

	if readTimeout := os.Getenv(EnvReadTimeout); readTimeout != "" {
		d, err := time.ParseDuration(readTimeout)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvReadTimeout, err)
		}
		config.ReadTimeout = d
	}

	if writeTimeout := os.Getenv(EnvWriteTimeout); writeTimeout != "" {
		d, err := time.ParseDuration(writeTimeout)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvWriteTimeout, err)
		}
		config.WriteTimeout = d
	}

	if shutdownTimeout := os.Getenv(EnvShutdownTimeout); shutdownTimeout != "" {
		d, err := time.ParseDuration(shutdownTimeout)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvShutdownTimeout, err)
		}
		config.ShutdownTimeout = d
	}

	if trustedProxies := os.Getenv(EnvTrustedProxies); trustedProxies != "" {
		config.TrustedProxies = splitAndTrim(trustedProxies, ",")
	}

	return nil
}

// applyJWTConfig applies JWT-related environment variable overrides
func applyJWTConfig(config *Config) error {
	// Required JWT signing key
	if key := os.Getenv(EnvJwtSigningKey); key != "" {
		config.JWTSigningKey = key
	} else {
		return fmt.Errorf("%s environment variable must be set", EnvJwtSigningKey)
	}

	if issuer := os.Getenv(EnvJwtIssuer); issuer != "" {
		config.JWTIssuer = issuer
	}

	if audience := os.Getenv(EnvJwtAudience); audience != "" {
		config.JWTAudience = audience
	}

	if expiration := os.Getenv(EnvJwtExpiration); expiration != "" {
		d, err := time.ParseDuration(expiration)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvJwtExpiration, err)
		}
		config.JWTExpiration = d
	}

	if refreshWindow := os.Getenv(EnvJwtRefreshWindow); refreshWindow != "" {
		d, err := time.ParseDuration(refreshWindow)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvJwtRefreshWindow, err)
		}
		config.JWTRefreshWindow = d
	}

	if refreshHorizon := os.Getenv(EnvJwtRefreshHorizon); refreshHorizon != "" {
		d, err := time.ParseDuration(refreshHorizon)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvJwtRefreshHorizon, err)
		}
		config.JWTRefreshHorizon = d
	}

	if enableBearerAuth := os.Getenv(EnvEnableBearerAuth); enableBearerAuth != "" {
		enable, err := strconv.ParseBool(enableBearerAuth)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvEnableBearerAuth, err)
		}
		config.EnableBearerAuth = enable
	}

	// Validate that JWTExpiration >= JWTRefreshWindow
	if config.JWTRefreshWindow > config.JWTExpiration {
		return fmt.Errorf("JWT refresh window (%s) must be less than or equal to JWT expiration (%s)",
			config.JWTRefreshWindow, config.JWTExpiration)
	}

	if config.JWTExpiration > config.JWTRefreshHorizon {
		return fmt.Errorf("JWT refresh horizon (%s) must be greater or equal to JWT expiration (%s)",
			config.JWTRefreshWindow, config.JWTExpiration)
	}

	return nil
}

// applyCookieConfig applies cookie-related environment variable overrides
func applyCookieConfig(config *Config) error {
	if cookieName := os.Getenv(EnvCookieName); cookieName != "" {
		config.CookieName = cookieName
	}

	if cookieSecure := os.Getenv(EnvCookieSecure); cookieSecure != "" {
		secure, err := strconv.ParseBool(cookieSecure)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvCookieSecure, err)
		}
		config.CookieSecure = secure
	}

	if cookieDomain := os.Getenv(EnvCookieDomain); cookieDomain != "" {
		config.CookieDomain = cookieDomain
	}

	if cookiePath := os.Getenv(EnvCookiePath); cookiePath != "" {
		config.CookiePath = cookiePath
	}

	if cookieMaxAge := os.Getenv(EnvCookieMaxAge); cookieMaxAge != "" {
		d, err := time.ParseDuration(cookieMaxAge)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvCookieMaxAge, err)
		}
		config.CookieMaxAge = d
	}

	if cookieHTTPOnly := os.Getenv(EnvCookieHttpOnly); cookieHTTPOnly != "" {
		httpOnly, err := strconv.ParseBool(cookieHTTPOnly)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvCookieHttpOnly, err)
		}
		config.CookieHTTPOnly = httpOnly
	}

	if cookieSameSite := os.Getenv(EnvCookieSameSite); cookieSameSite != "" {
		config.CookieSameSite = cookieSameSite
	}

	return nil
}

// applyCSRFConfig applies CSRF-related environment variable overrides
func applyCSRFConfig(config *Config) error {
	// Required CSRF auth key
	if csrfAuthKey := os.Getenv(EnvCsrfAuthKey); csrfAuthKey != "" {
		config.CSRFAuthKey = csrfAuthKey
	} else {
		return fmt.Errorf("%s environment variable must be set", EnvCsrfAuthKey)
	}

	if csrfCookieName := os.Getenv(EnvCsrfCookieName); csrfCookieName != "" {
		config.CSRFCookieName = csrfCookieName
	}

	if csrfCookieMaxAge := os.Getenv(EnvCsrfCookieMaxAge); csrfCookieMaxAge != "" {
		d, err := time.ParseDuration(csrfCookieMaxAge)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvCsrfCookieMaxAge, err)
		}
		config.CSRFCookieMaxAge = d
	}

	if csrfCookieSecure := os.Getenv(EnvCsrfCookieSecure); csrfCookieSecure != "" {
		secure, err := strconv.ParseBool(csrfCookieSecure)
		if err != nil {
			return fmt.Errorf("invalid %s: %w", EnvCsrfCookieSecure, err)
		}
		config.CSRFCookieSecure = secure
	}

	if csrfFieldName := os.Getenv(EnvCsrfFieldName); csrfFieldName != "" {
		config.CSRFFieldName = csrfFieldName
	}

	if csrfHeaderName := os.Getenv(EnvCsrfHeaderName); csrfHeaderName != "" {
		config.CSRFHeaderName = csrfHeaderName
	}

	if csrfTrustedOrigins := os.Getenv(EnvCsrfTrustedOrigins); csrfTrustedOrigins != "" {
		config.CSRFTrustedOrigins = splitAndTrim(csrfTrustedOrigins, ",")
	}

	return nil
}
