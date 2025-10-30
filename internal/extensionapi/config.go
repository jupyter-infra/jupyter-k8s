// Package extensionapi provides extension API server functionality.
package extensionapi

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
	// AWS section
	EKSClusterARN string
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

// WithEKSClusterARN sets the EKS cluster ARN
func WithEKSClusterARN(arn string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.EKSClusterARN = arn
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
