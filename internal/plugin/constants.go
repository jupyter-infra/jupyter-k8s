/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package plugin provides shared types and constants for the plugin client/server protocol.
package plugin

// HTTP headers for request ID correlation between client and plugin.
const (
	// HeaderPluginRequestID identifies a single client→plugin HTTP call.
	HeaderPluginRequestID = "X-Plugin-Request-ID"
	// HeaderOriginRequestID traces back to the original trigger (extensionapi request, controller event).
	HeaderOriginRequestID = "X-Origin-Request-ID"
)
