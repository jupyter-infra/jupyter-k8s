/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package pluginserver provides HTTP server scaffolding for plugin implementations.
// Plugin authors implement the handler interfaces; the server handles routing,
// JSON encoding/decoding, health checks, and error formatting.
package pluginserver

import (
	"os"
	"strconv"

	"github.com/go-logr/logr"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"
)

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
	// Port the server listens on. Defaults to DefaultPort (8080).
	// Can be overridden via the PLUGIN_PORT environment variable.
	Port int

	// ListenAddress is the IP address the server binds to. Defaults to "127.0.0.1"
	// (localhost only), since the plugin runs as a sidecar in the same pod.
	ListenAddress string

	// JWTHandler implements JWT signing and verification.
	// If nil, all JWT endpoints return 501 Not Implemented.
	JWTHandler plugin.JwtPluginApis

	// RemoteAccessHandler implements remote access operations.
	// If nil, all remote access endpoints return 501 Not Implemented.
	RemoteAccessHandler plugin.RemoteAccessPluginApis

	// Logger for the server. If zero-value, logs are discarded.
	Logger logr.Logger
}

// applyDefaults fills in zero-value fields with sensible defaults.
// If Port is zero, reads PLUGIN_PORT from the environment, falling back to DefaultPort.
func (c *ServerConfig) applyDefaults() {
	if c.ListenAddress == "" {
		c.ListenAddress = DefaultListenAddress
	}
	if c.Port == 0 {
		if portStr := os.Getenv(EnvPluginPort); portStr != "" {
			if p, err := strconv.Atoi(portStr); err == nil {
				c.Port = p
				return
			}
		}
		c.Port = DefaultPort
	}
}
