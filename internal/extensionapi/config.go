/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package extensionapi provides extension API server functionality.
package extensionapi

import "time"

// Default values
const (
	// Server defaults
	DefaultApiPath             = "/apis/connection.workspace.jupyter.org/v1alpha1"
	DefaultServerPort          = 7443
	DefaultCertPath            = "/tmp/extension-server/serving-certs/tls.crt"
	DefaultKeyPath             = "/tmp/extension-server/serving-certs/tls.key"
	DefaultLogLevel            = "info"
	DefaultDisableTLS          = false
	DefaultReadTimeoutSeconds  = 30
	DefaultWriteTimeoutSeconds = 120
	DefaultAllowedOrigin       = "*"

	// JWT defaults
	DefaultJwtIssuer      = "workspaces-controller"
	DefaultJwtAudience    = "workspaces-controller"
	DefaultJwtTTL         = 5 * time.Minute
	DefaultNewKeyUseDelay = 5 * time.Second
)

// ExtensionConfig contains the configuration for the extension API server
type ExtensionConfig struct {
	ApiPath             string
	ServerPort          int
	CertPath            string
	KeyPath             string
	LogLevel            string
	DisableTLS          bool
	ReadTimeoutSeconds  int
	WriteTimeoutSeconds int
	AllowedOrigin       string
	// Plugin section
	PluginEndpoints map[string]string // e.g. {"aws": "http://localhost:8080"} for plugin sidecars

	// Controller namespace (from Downward API)
	ControllerNamespace string

	// JWT signing section
	JwtIssuer      string
	JwtAudience    string
	JwtSecretName  string
	JwtTTL         time.Duration
	NewKeyUseDelay time.Duration
}

// ConfigOption is a function that modifies an ExtensionConfig
type ConfigOption func(*ExtensionConfig)

// WithDefaultApiPath sets the base api path
func WithDefaultApiPath(apiPath string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.ApiPath = apiPath
	}
}

// WithServerPort sets the server port
func WithServerPort(port int) ConfigOption {
	return func(c *ExtensionConfig) {
		c.ServerPort = port
	}
}

// WithCertPath sets the certificate path
func WithCertPath(path string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.CertPath = path
	}
}

// WithKeyPath sets the key path
func WithKeyPath(path string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.KeyPath = path
	}
}

// WithLogLevel sets the log level
func WithLogLevel(level string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.LogLevel = level
	}
}

// WithDisableTLS sets whether TLS should be disabled
func WithDisableTLS(disable bool) ConfigOption {
	return func(c *ExtensionConfig) {
		c.DisableTLS = disable
	}
}

// WithReadTimeoutSeconds sets the read timeout in seconds
func WithReadTimeoutSeconds(timeoutSeconds int) ConfigOption {
	return func(c *ExtensionConfig) {
		c.ReadTimeoutSeconds = timeoutSeconds
	}
}

// WithWriteTimeoutSeconds sets the write timeout in seconds
func WithWriteTimeoutSeconds(timeoutSeconds int) ConfigOption {
	return func(c *ExtensionConfig) {
		c.WriteTimeoutSeconds = timeoutSeconds
	}
}

// WithAllowedOrigin sets the allowed origin for CORS
func WithAllowedOrigin(origin string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.AllowedOrigin = origin
	}
}

// WithPluginEndpoints sets the plugin name→endpoint map (e.g. {"aws": "http://localhost:8080"}).
func WithPluginEndpoints(endpoints map[string]string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.PluginEndpoints = endpoints
	}
}

// WithControllerNamespace sets the namespace the controller is running in.
func WithControllerNamespace(ns string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.ControllerNamespace = ns
	}
}

// WithJwtIssuer sets the JWT issuer claim.
func WithJwtIssuer(issuer string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.JwtIssuer = issuer
	}
}

// WithJwtAudience sets the JWT audience claim.
func WithJwtAudience(audience string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.JwtAudience = audience
	}
}

// WithJwtSecretName sets the K8s Secret name for JWT signing keys.
// When set, the extension API uses StandardSignerFactory for k8s-native JWT signing.
func WithJwtSecretName(name string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.JwtSecretName = name
	}
}

// WithJwtTTL sets the JWT expiration duration.
func WithJwtTTL(ttl time.Duration) ConfigOption {
	return func(c *ExtensionConfig) {
		c.JwtTTL = ttl
	}
}

// WithNewKeyUseDelay sets the cooloff delay before using a newly rotated signing key.
func WithNewKeyUseDelay(delay time.Duration) ConfigOption {
	return func(c *ExtensionConfig) {
		c.NewKeyUseDelay = delay
	}
}

// NewConfig creates an ExtensionConfig with default values and applies
// any provided options
func NewConfig(opts ...ConfigOption) *ExtensionConfig {
	config := &ExtensionConfig{
		ApiPath:             DefaultApiPath,
		ServerPort:          DefaultServerPort,
		CertPath:            DefaultCertPath,
		KeyPath:             DefaultKeyPath,
		LogLevel:            DefaultLogLevel,
		DisableTLS:          DefaultDisableTLS,
		ReadTimeoutSeconds:  DefaultReadTimeoutSeconds,
		WriteTimeoutSeconds: DefaultWriteTimeoutSeconds,
		AllowedOrigin:       DefaultAllowedOrigin,
	}

	// Apply all options
	for _, opt := range opts {
		opt(config)
	}

	return config
}
