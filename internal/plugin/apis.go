/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package plugin

import (
	"context"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
)

// RemoteAccessPluginApis defines the interface for remote access operations.
// Both the plugin server (awsplugin) and plugin client (pluginclient) implement this.
type RemoteAccessPluginApis interface {
	Initialize(ctx context.Context, req *pluginapi.InitializeRequest) (*pluginapi.InitializeResponse, error)
	RegisterNodeAgent(ctx context.Context, req *pluginapi.RegisterNodeAgentRequest) (*pluginapi.RegisterNodeAgentResponse, error)
	DeregisterNodeAgent(ctx context.Context, req *pluginapi.DeregisterNodeAgentRequest) (*pluginapi.DeregisterNodeAgentResponse, error)
	CreateSession(ctx context.Context, req *pluginapi.CreateSessionRequest) (*pluginapi.CreateSessionResponse, error)
}

// JwtPluginApis defines the interface for JWT signing and verification operations.
// Both the plugin server and plugin client implement this.
type JwtPluginApis interface {
	Sign(ctx context.Context, req *pluginapi.SignRequest) (*pluginapi.SignResponse, error)
	Verify(ctx context.Context, req *pluginapi.VerifyRequest) (*pluginapi.VerifyResponse, error)
}
