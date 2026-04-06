/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginserver

import (
	"context"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
)

// NotImplementedRemoteAccessHandler returns 501 for all remote access operations.
// Used as the default when no RemoteAccessPluginApis is provided to NewServer.
type NotImplementedRemoteAccessHandler struct{}

// Initialize implements plugin.RemoteAccessPluginApis.
func (NotImplementedRemoteAccessHandler) Initialize(context.Context, *pluginapi.InitializeRequest) (*pluginapi.InitializeResponse, error) {
	return nil, errNotImplemented
}

// RegisterNodeAgent implements plugin.RemoteAccessPluginApis.
func (NotImplementedRemoteAccessHandler) RegisterNodeAgent(context.Context, *pluginapi.RegisterNodeAgentRequest) (*pluginapi.RegisterNodeAgentResponse, error) {
	return nil, errNotImplemented
}

// DeregisterNodeAgent implements plugin.RemoteAccessPluginApis.
func (NotImplementedRemoteAccessHandler) DeregisterNodeAgent(context.Context, *pluginapi.DeregisterNodeAgentRequest) (*pluginapi.DeregisterNodeAgentResponse, error) {
	return nil, errNotImplemented
}

// CreateSession implements plugin.RemoteAccessPluginApis.
func (NotImplementedRemoteAccessHandler) CreateSession(context.Context, *pluginapi.CreateSessionRequest) (*pluginapi.CreateSessionResponse, error) {
	return nil, errNotImplemented
}
