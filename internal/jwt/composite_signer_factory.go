/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

import (
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// CompositeSignerFactory dispatches to the appropriate SignerFactory based on the
// access strategy's CreateConnectionHandler field.
type CompositeSignerFactory struct {
	factories      map[string]SignerFactory
	defaultFactory SignerFactory
}

// NewCompositeSignerFactory creates a factory that routes to child factories by handler name.
// The defaultFactory is used when the access strategy is nil or has an empty handler.
func NewCompositeSignerFactory(factories map[string]SignerFactory, defaultFactory SignerFactory) *CompositeSignerFactory {
	return &CompositeSignerFactory{
		factories:      factories,
		defaultFactory: defaultFactory,
	}
}

// CreateSigner delegates to the child factory matching the access strategy's handler.
func (c *CompositeSignerFactory) CreateSigner(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (Signer, error) {
	if accessStrategy == nil || accessStrategy.Spec.CreateConnectionHandler == "" {
		return c.defaultFactory.CreateSigner(accessStrategy)
	}

	handler := accessStrategy.Spec.CreateConnectionHandler
	factory, ok := c.factories[handler]
	if !ok {
		supported := make([]string, 0, len(c.factories))
		for k := range c.factories {
			supported = append(supported, k)
		}
		return nil, fmt.Errorf("unsupported connection handler %q (available: %v)", handler, supported)
	}

	return factory.CreateSigner(accessStrategy)
}

// GetFactory returns the child factory registered for the given handler name.
func (c *CompositeSignerFactory) GetFactory(handler string) (SignerFactory, bool) {
	f, ok := c.factories[handler]
	return f, ok
}

// ValidateToken tries to validate the token against all configured signers.
// Returns the claims from the first signer that succeeds. This handles mixed
// deployments where tokens could be signed by any configured signer (AWS or k8s-native).
func (c *CompositeSignerFactory) ValidateToken(tokenString string) (*Claims, error) {
	var lastErr error
	for name, factory := range c.factories {
		signer, err := factory.CreateSigner(nil)
		if err != nil {
			lastErr = fmt.Errorf("factory %q: %w", name, err)
			continue
		}
		claims, err := signer.ValidateToken(tokenString)
		if err == nil {
			return claims, nil
		}
		lastErr = fmt.Errorf("factory %q: %w", name, err)
	}
	if lastErr != nil {
		return nil, fmt.Errorf("token validation failed: %w", lastErr)
	}
	return nil, fmt.Errorf("no signer factories configured")
}
