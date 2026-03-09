/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
)

// RegisterSecretWatch registers informer event handlers to watch for secret changes
// and update the StandardSigner when keys are rotated.
func (s *StandardSigner) RegisterSecretWatch(
	mgr ctrl.Manager,
	secretName string,
	namespace string,
	logger logr.Logger,
) error {
	// Get informer for Secrets from the manager's cache
	// This provides automatic retry/backoff and reconnection
	ctx := context.Background()
	informer, err := mgr.GetCache().GetInformer(ctx, &corev1.Secret{})
	if err != nil {
		return fmt.Errorf("failed to get secret informer: %w", err)
	}

	// Helper function to update signer from secret
	updateSignerFromSecret := func(secret *corev1.Secret) {
		signingKeys, latestKid, err := ParseSigningKeysFromSecret(secret)
		if err != nil {
			logger.Error(err, "Failed to parse signing keys")
			return
		}

		if err := s.UpdateKeys(signingKeys, latestKid); err != nil {
			logger.Error(err, "Failed to update signing keys")
			return
		}

		logger.Info("Successfully updated signing keys from secret",
			"keyCount", len(signingKeys),
			"latestKid", latestKid)
	}

	// Add event handler with filtering by secret name and namespace
	_, err = informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			secret, ok := obj.(*corev1.Secret)
			if !ok {
				return
			}
			// Filter: only process our specific secret
			if secret.Name == secretName && secret.Namespace == namespace {
				logger.Info("Secret added event received", "secret", secret.Name, "namespace", secret.Namespace)
				updateSignerFromSecret(secret)
			}
		},
		UpdateFunc: func(_, newObj interface{}) {
			secret, ok := newObj.(*corev1.Secret)
			if !ok {
				return
			}
			// Filter: only process our specific secret
			if secret.Name == secretName && secret.Namespace == namespace {
				logger.Info("Secret updated event received", "secret", secret.Name, "namespace", secret.Namespace)
				updateSignerFromSecret(secret)
			}
		},
		DeleteFunc: func(obj interface{}) {
			secret, ok := obj.(*corev1.Secret)
			if !ok {
				return
			}
			// Filter: only process our specific secret
			if secret.Name == secretName && secret.Namespace == namespace {
				logger.Error(fmt.Errorf("secret was deleted"), "JWT secret deleted",
					"secret", secretName,
					"namespace", namespace)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler to informer: %w", err)
	}

	logger.Info("JWT secret watch event handlers registered")
	return nil
}
