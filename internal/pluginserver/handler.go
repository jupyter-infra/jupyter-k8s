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
	"net/http"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"
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

var errNotImplemented = &plugin.StatusError{Code: http.StatusNotImplemented, Message: "not implemented"}

// NotImplementedJWTHandler returns 501 for all JWT operations.
// Used as the default when no JWTHandler is provided to NewServer.
type NotImplementedJWTHandler struct{}

// Sign implements JWTHandler.
func (NotImplementedJWTHandler) Sign(context.Context, *pluginapi.SignRequest) (*pluginapi.SignResponse, error) {
	return nil, errNotImplemented
}

// Verify implements JWTHandler.
func (NotImplementedJWTHandler) Verify(context.Context, *pluginapi.VerifyRequest) (*pluginapi.VerifyResponse, error) {
	return nil, errNotImplemented
}

// NotImplementedRemoteAccessHandler returns 501 for all remote access operations.
// Used as the default when no RemoteAccessHandler is provided to NewServer.
type NotImplementedRemoteAccessHandler struct{}

// Initialize implements RemoteAccessHandler.
func (NotImplementedRemoteAccessHandler) Initialize(context.Context, *pluginapi.InitializeRequest) (*pluginapi.InitializeResponse, error) {
	return nil, errNotImplemented
}

// RegisterNodeAgent implements RemoteAccessHandler.
func (NotImplementedRemoteAccessHandler) RegisterNodeAgent(context.Context, *pluginapi.RegisterNodeAgentRequest) (*pluginapi.RegisterNodeAgentResponse, error) {
	return nil, errNotImplemented
}

// DeregisterNodeAgent implements RemoteAccessHandler.
func (NotImplementedRemoteAccessHandler) DeregisterNodeAgent(context.Context, *pluginapi.DeregisterNodeAgentRequest) (*pluginapi.DeregisterNodeAgentResponse, error) {
	return nil, errNotImplemented
}

// CreateSession implements RemoteAccessHandler.
func (NotImplementedRemoteAccessHandler) CreateSession(context.Context, *pluginapi.CreateSessionRequest) (*pluginapi.CreateSessionResponse, error) {
	return nil, errNotImplemented
}
