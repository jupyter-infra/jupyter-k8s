/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
)

// SetupAuthMiddlewareWithManager sets up the authentication middleware server
// and adds it to the manager as a Runnable.
func SetupAuthMiddlewareWithManager(mgr ctrl.Manager, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Get logger from manager
	logrLogger := mgr.GetLogger().WithName("authmiddleware")

	// Create slog.Logger for server (server currently uses slog)
	slogLogger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Get controller-runtime client from manager (for testability)
	runtimeClient := mgr.GetClient()

	// Create JWT handler
	jwtHandler, standardSigner, err := NewJWTHandler(cfg, logrLogger.WithName("jwt"))
	if err != nil {
		return fmt.Errorf("failed to create JWT handler: %w", err)
	}

	// Register secret watching event handlers if using standard signing
	if standardSigner != nil {
		logrLogger.Info("Registering secret watch event handlers",
			"secret", cfg.JwtSecretName,
			"namespace", cfg.Namespace)

		if err := registerSecretWatchHandlers(
			mgr,
			cfg.JwtSecretName,
			cfg.Namespace,
			standardSigner,
			logrLogger.WithName("secret-watch"),
		); err != nil {
			return fmt.Errorf("failed to register secret watch handlers: %w", err)
		}
	}

	// Create cookie manager
	cookieManager, err := NewCookieManager(cfg)
	if err != nil {
		return fmt.Errorf("failed to create cookie manager: %w", err)
	}

	// Create HTTP server
	server := NewServer(cfg, jwtHandler, cookieManager, slogLogger)

	// Wrap server in HTTPServerRunnable
	// Pass standardSigner and secret info for initial key loading on start
	httpServerRunnable := NewHTTPServerRunnable(
		server,
		logrLogger.WithName("http-server"),
		runtimeClient,
		standardSigner,
		cfg.JwtSecretName,
		cfg.Namespace,
	)

	logrLogger.Info("Adding HTTP server to manager")
	if err := mgr.Add(httpServerRunnable); err != nil {
		return fmt.Errorf("failed to add HTTP server to manager: %w", err)
	}

	logrLogger.Info("Authentication middleware setup complete")
	return nil
}

// registerSecretWatchHandlers registers informer event handlers to watch for secret changes
// and update the StandardSigner when keys are rotated.
func registerSecretWatchHandlers(
	mgr ctrl.Manager,
	secretName string,
	namespace string,
	standardSigner *jwt.StandardSigner,
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
		// Parse signing keys from secret
		signingKeys, latestKid, err := jwt.ParseSigningKeysFromSecret(secret)
		if err != nil {
			logger.Error(err, "Failed to parse signing keys")
			return
		}

		// Update signer with new keys
		if err := standardSigner.UpdateKeys(signingKeys, latestKid); err != nil {
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
				logger.Error(fmt.Errorf("unexpected object type: %T", obj),
					"Failed to cast add event object to Secret")
				return
			}

			// Filter: only process our specific secret
			if secret.Name == secretName && secret.Namespace == namespace {
				logger.Info("Secret added event received", "secret", secret.Name, "namespace", secret.Namespace)
				updateSignerFromSecret(secret)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			secret, ok := newObj.(*corev1.Secret)
			if !ok {
				logger.Error(fmt.Errorf("unexpected object type: %T", newObj),
					"Failed to cast update event object to Secret")
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
				logger.Error(fmt.Errorf("unexpected object type: %T", obj),
					"Failed to cast delete event object to Secret")
				return
			}

			// Filter: only process our specific secret
			if secret.Name == secretName && secret.Namespace == namespace {
				logger.Error(fmt.Errorf("secret was deleted"), "Secret deleted",
					"secret", secretName,
					"namespace", namespace)
				// No action needed - secret might be recreated and we'll get an Add event
			}
		},
	})
	if err != nil {
		return fmt.Errorf("failed to add event handler to informer: %w", err)
	}

	logger.Info("Secret watch event handlers registered")
	return nil
}
