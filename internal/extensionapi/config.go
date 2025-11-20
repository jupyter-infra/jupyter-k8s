/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

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
	ClusterId string
	KMSKeyID  string
	Domain    string
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

// WithKMSKeyID sets the KMS key ID for JWT token encryption
func WithKMSKeyID(keyID string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.KMSKeyID = keyID
	}
}

// WithDomain sets the domain for Web UI URLs
func WithDomain(domain string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.Domain = domain
	}
}

// WithClusterId sets the cluster ID
func WithClusterId(id string) ConfigOption {
	return func(c *ExtensionConfig) {
		c.ClusterId = id
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
