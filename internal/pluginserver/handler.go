/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package pluginserver provides HTTP server scaffolding for plugin implementations.
// Plugin authors implement the handler interfaces; the server handles routing,
// JSON encoding/decoding, health checks, and error formatting.
package pluginserver

import (
	"context"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
)

// JWTHandler defines the interface that plugin implementations must satisfy for JWT operations.
type JWTHandler interface {
	Sign(ctx context.Context, req *pluginapi.SignRequest) (*pluginapi.SignResponse, error)
	Verify(ctx context.Context, req *pluginapi.VerifyRequest) (*pluginapi.VerifyResponse, error)
}

// RemoteAccessHandler defines the interface that plugin implementations must satisfy
// for remote access operations.
type RemoteAccessHandler interface {
	Initialize(ctx context.Context, req *pluginapi.InitializeRequest) (*pluginapi.InitializeResponse, error)
	RegisterNodeAgent(ctx context.Context, req *pluginapi.RegisterNodeAgentRequest) (*pluginapi.RegisterNodeAgentResponse, error)
	DeregisterNodeAgent(ctx context.Context, req *pluginapi.DeregisterNodeAgentRequest) (*pluginapi.DeregisterNodeAgentResponse, error)
	CreateSession(ctx context.Context, req *pluginapi.CreateSessionRequest) (*pluginapi.CreateSessionResponse, error)
}
