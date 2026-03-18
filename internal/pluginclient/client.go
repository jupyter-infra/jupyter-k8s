/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package pluginclient provides HTTP clients that implement core operator interfaces
// (jwt.SignerFactory, RemoteAccessStrategyInterface) by calling plugin sidecar endpoints.
package pluginclient

import (
	"errors"
	"net/http"
)

// ErrNotImplemented is returned by shell methods that are not yet implemented.
var ErrNotImplemented = errors.New("plugin client: not implemented")

// PluginClient is the shared HTTP client for communicating with a plugin sidecar.
type PluginClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewPluginClient creates a new PluginClient for the given plugin base URL.
func NewPluginClient(baseURL string) *PluginClient {
	return &PluginClient{
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}
