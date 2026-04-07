/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginserver

import (
	"context"
	"net/http"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"
)

var errNotImplemented = &plugin.StatusError{Code: http.StatusNotImplemented, Message: "not implemented"}

// NotImplementedJWTHandler returns 501 for all JWT operations.
// Used as the default when no JwtPluginApis is provided to NewServer.
type NotImplementedJWTHandler struct{}

// Sign implements plugin.JwtPluginApis.
func (NotImplementedJWTHandler) Sign(context.Context, *pluginapi.SignRequest) (*pluginapi.SignResponse, error) {
	return nil, errNotImplemented
}

// Verify implements plugin.JwtPluginApis.
func (NotImplementedJWTHandler) Verify(context.Context, *pluginapi.VerifyRequest) (*pluginapi.VerifyResponse, error) {
	return nil, errNotImplemented
}
