/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient

import (
	"context"
	"errors"
	"testing"

	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// Compile-time interface conformance checks.
var (
	_ jwt.SignerFactory = (*PluginSignerFactory)(nil)
	_ jwt.Signer        = (*PluginSigner)(nil)
)

func TestPluginSignerFactory_CreateSigner_ReturnsNotImplemented(t *testing.T) {
	factory := NewPluginSignerFactory("http://localhost:8080")
	_, err := factory.CreateSigner(&workspacev1alpha1.WorkspaceAccessStrategy{})
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestPluginSigner_GenerateToken_ReturnsNotImplemented(t *testing.T) {
	signer := &PluginSigner{client: NewPluginClient("http://localhost:8080")}
	_, err := signer.GenerateToken("user", []string{"g"}, "uid", nil, "/path", "domain", "bootstrap")
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestPluginSigner_ValidateToken_ReturnsNotImplemented(t *testing.T) {
	signer := &PluginSigner{client: NewPluginClient("http://localhost:8080")}
	_, err := signer.ValidateToken("some-token")
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestPluginRemoteAccessClient_SetupContainers_ReturnsNotImplemented(t *testing.T) {
	strategy := NewPluginRemoteAccessClient("http://localhost:8080")
	err := strategy.SetupContainers(context.Background(), nil, nil, nil)
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestPluginRemoteAccessClient_DeregisterNodeAgent_ReturnsNotImplemented(t *testing.T) {
	strategy := NewPluginRemoteAccessClient("http://localhost:8080")
	err := strategy.DeregisterNodeAgent(context.Background(), nil)
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestPluginRemoteAccessClient_Initialize_ReturnsNotImplemented(t *testing.T) {
	strategy := NewPluginRemoteAccessClient("http://localhost:8080")
	err := strategy.Initialize(context.Background())
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}

func TestPluginRemoteAccessClient_CreateSession_ReturnsNotImplemented(t *testing.T) {
	strategy := NewPluginRemoteAccessClient("http://localhost:8080")
	_, err := strategy.CreateSession(context.Background(), "ws", "ns", "pod-uid", nil)
	if !errors.Is(err, ErrNotImplemented) {
		t.Errorf("expected ErrNotImplemented, got %v", err)
	}
}
