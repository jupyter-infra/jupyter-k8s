/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SecretInitializer loads initial JWT signing keys from a K8s Secret on startup.
// It implements the controller-runtime Runnable interface.
type SecretInitializer struct {
	signer     *StandardSigner
	secretName string
	namespace  string
	logger     logr.Logger
}

// NewSecretInitializer creates a new SecretInitializer that loads initial JWT signing keys
// from the specified K8s Secret when started.
func NewSecretInitializer(signer *StandardSigner, secretName, namespace string, logger logr.Logger) *SecretInitializer {
	return &SecretInitializer{
		signer:     signer,
		secretName: secretName,
		namespace:  namespace,
		logger:     logger,
	}
}

// Start loads the initial JWT signing keys from the K8s Secret. Implements the controller-runtime Runnable interface.
func (b *SecretInitializer) Start(ctx context.Context) error {
	b.logger.Info("Loading initial JWT signing keys",
		"secret", b.secretName,
		"namespace", b.namespace)

	// Use a direct client (not cached) to ensure we get the latest secret
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get rest config: %w", err)
	}

	directClient, err := client.New(cfg, client.Options{})
	if err != nil {
		return fmt.Errorf("failed to create direct client: %w", err)
	}

	if err := b.signer.RetrieveInitialSecret(ctx, directClient, b.secretName, b.namespace); err != nil {
		return fmt.Errorf("failed to load initial JWT signing keys: %w", err)
	}

	b.logger.Info("Successfully loaded initial JWT signing keys")
	return nil
}

// NeedLeaderElection returns false because secret initialization should run on all replicas.
func (b *SecretInitializer) NeedLeaderElection() bool {
	return false
}
