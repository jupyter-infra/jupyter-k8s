/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginserver

// Default values for server configuration.
const (
	// DefaultListenAddress is the IPv4 loopback address. This is a literal IP,
	// not a DNS name like "localhost", so it is resolved entirely by the kernel
	// network stack — no DNS lookup, no /etc/hosts override, no ambiguity
	// between IPv4 and IPv6.
	DefaultListenAddress = "127.0.0.1"
)

// ServerConfig holds all configuration for the plugin server.
type ServerConfig struct {
	// Port the server listens on. Required — no default, since multiple
	// plugin sidecars in the same pod need distinct ports.
	Port int

	// ListenAddress is the IP address the server binds to. Defaults to "127.0.0.1"
	// (localhost only), since the plugin runs as a sidecar in the same pod.
	ListenAddress string

	// JWTHandler implements JWT signing and verification.
	// If nil, all JWT endpoints return 501 Not Implemented.
	JWTHandler JWTHandler

	// RemoteAccessHandler implements remote access operations.
	// If nil, all remote access endpoints return 501 Not Implemented.
	RemoteAccessHandler RemoteAccessHandler
}

// applyDefaults fills in zero-value fields with sensible defaults.
func (c *ServerConfig) applyDefaults() {
	if c.ListenAddress == "" {
		c.ListenAddress = DefaultListenAddress
	}
}
