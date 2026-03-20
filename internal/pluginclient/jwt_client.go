/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient

import (
	"context"
	"time"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"

	jwt5 "github.com/golang-jwt/jwt/v5"
)

// PluginSignerFactory implements jwt.SignerFactory by delegating to a plugin sidecar.
type PluginSignerFactory struct {
	client *PluginClient
}

// NewPluginSignerFactory creates a new PluginSignerFactory backed by the given shared client.
func NewPluginSignerFactory(client *PluginClient) *PluginSignerFactory {
	return &PluginSignerFactory{client: client}
}

// CreateSigner returns a PluginSigner that delegates JWT operations to the plugin.
func (f *PluginSignerFactory) CreateSigner(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (jwt.Signer, error) {
	var connectionContext map[string]string
	if accessStrategy != nil {
		connectionContext = accessStrategy.Spec.CreateConnectionContext
	}
	return &PluginSigner{
		client:            f.client,
		connectionContext: connectionContext,
	}, nil
}

// PluginSigner implements jwt.Signer by delegating to a plugin sidecar over HTTP.
type PluginSigner struct {
	client            *PluginClient
	connectionContext map[string]string
}

// GenerateToken delegates token generation to the plugin via POST /v1alpha1/jwt/sign.
// Uses context.Background() because the jwt.Signer interface does not accept a context.
func (s *PluginSigner) GenerateToken(user string, groups []string, uid string, extra map[string][]string, path, domain, tokenType string) (string, error) {
	req := &pluginapi.SignRequest{
		User:              user,
		Groups:            groups,
		UID:               uid,
		Extra:             extra,
		Path:              path,
		Domain:            domain,
		TokenType:         tokenType,
		ConnectionContext: s.connectionContext,
	}
	resp, _, err := doPost[pluginapi.SignResponse](context.Background(), s.client, pluginapi.RouteJWTSign.Path, req)
	if err != nil {
		return "", err
	}
	return resp.Token, nil
}

// ValidateToken delegates token validation to the plugin via POST /v1alpha1/jwt/verify.
// Uses context.Background() because the jwt.Signer interface does not accept a context.
func (s *PluginSigner) ValidateToken(tokenString string) (*jwt.Claims, error) {
	req := &pluginapi.VerifyRequest{Token: tokenString}
	resp, _, err := doPost[pluginapi.VerifyResponse](context.Background(), s.client, pluginapi.RouteJWTVerify.Path, req)
	if err != nil {
		return nil, err
	}
	if resp.Claims == nil {
		return nil, &plugin.StatusError{Code: 200, Message: "verify returned nil claims"}
	}
	return verifyClaims(resp.Claims), nil
}

func verifyClaims(vc *pluginapi.VerifyClaims) *jwt.Claims {
	return &jwt.Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			Subject:   vc.Subject,
			ExpiresAt: jwt5.NewNumericDate(time.Unix(vc.ExpiresAt, 0)),
		},
		User:      vc.Subject,
		Groups:    vc.Groups,
		UID:       vc.UID,
		Extra:     vc.Extra,
		Path:      vc.Path,
		Domain:    vc.Domain,
		TokenType: vc.TokenType,
	}
}
