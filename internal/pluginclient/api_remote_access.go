/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient

import (
	"context"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
)

// Initialize delegates one-time resource creation to the plugin via POST /v1alpha1/remote-access/initialize.
func (c *PluginClient) Initialize(ctx context.Context, req *pluginapi.InitializeRequest) (*pluginapi.InitializeResponse, error) {
	resp, _, err := doPost[pluginapi.InitializeResponse](ctx, c, pluginapi.RouteRemoteAccessInit.Path, req)
	return resp, err
}

// RegisterNodeAgent calls the plugin to create an activation and returns the credentials.
func (c *PluginClient) RegisterNodeAgent(ctx context.Context, req *pluginapi.RegisterNodeAgentRequest) (*pluginapi.RegisterNodeAgentResponse, error) {
	resp, _, err := doPost[pluginapi.RegisterNodeAgentResponse](ctx, c, pluginapi.RouteRegisterNodeAgent.Path, req)
	return resp, err
}

// DeregisterNodeAgent delegates cleanup to the plugin via POST /v1alpha1/remote-access/deregister-node-agent.
func (c *PluginClient) DeregisterNodeAgent(ctx context.Context, req *pluginapi.DeregisterNodeAgentRequest) (*pluginapi.DeregisterNodeAgentResponse, error) {
	resp, _, err := doPost[pluginapi.DeregisterNodeAgentResponse](ctx, c, pluginapi.RouteDeregisterNodeAgent.Path, req)
	return resp, err
}

// CreateSession delegates session creation to the plugin via POST /v1alpha1/remote-access/create-session.
func (c *PluginClient) CreateSession(ctx context.Context, req *pluginapi.CreateSessionRequest) (*pluginapi.CreateSessionResponse, error) {
	resp, _, err := doPost[pluginapi.CreateSessionResponse](ctx, c, pluginapi.RouteCreateSession.Path, req)
	return resp, err
}
