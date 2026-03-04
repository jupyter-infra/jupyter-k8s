/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// k8sSecretJwtInitializer loads initial JWT signing keys from a K8s Secret on startup.
// It implements the controller-runtime Runnable interface.
type k8sSecretJwtInitializer struct {
	signer     *jwt.StandardSigner
	secretName string
	namespace  string
	logger     logr.Logger
}

func newK8sSecretJwtInitializer(signer *jwt.StandardSigner, secretName, namespace string, logger logr.Logger) *k8sSecretJwtInitializer {
	return &k8sSecretJwtInitializer{
		signer:     signer,
		secretName: secretName,
		namespace:  namespace,
		logger:     logger,
	}
}

func (b *k8sSecretJwtInitializer) Start(ctx context.Context) error {
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

func (b *k8sSecretJwtInitializer) NeedLeaderElection() bool {
	return false
}
