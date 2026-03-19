/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient

import (
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
)

// PluginSignerFactory implements jwt.SignerFactory by delegating to a plugin sidecar.
type PluginSignerFactory struct {
	client *PluginClient
}

// NewPluginSignerFactory creates a new PluginSignerFactory for the given plugin endpoint.
func NewPluginSignerFactory(baseURL string) *PluginSignerFactory {
	return &PluginSignerFactory{
		client: NewPluginClient(baseURL),
	}
}

// CreateSigner returns a PluginSigner that delegates JWT operations to the plugin.
func (f *PluginSignerFactory) CreateSigner(_ *workspacev1alpha1.WorkspaceAccessStrategy) (jwt.Signer, error) {
	return nil, ErrNotImplemented
}

// PluginSigner implements jwt.Signer by delegating to a plugin sidecar over HTTP.
type PluginSigner struct {
	client *PluginClient
}

// GenerateToken delegates token generation to the plugin via POST /v1alpha1/jwt/sign.
func (s *PluginSigner) GenerateToken(_ string, _ []string, _ string, _ map[string][]string, _, _, _ string) (string, error) {
	return "", ErrNotImplemented
}

// ValidateToken delegates token validation to the plugin via POST /v1alpha1/jwt/verify.
func (s *PluginSigner) ValidateToken(_ string) (*jwt.Claims, error) {
	return nil, ErrNotImplemented
}
