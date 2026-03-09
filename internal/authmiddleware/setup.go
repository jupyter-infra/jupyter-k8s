/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package authmiddleware

import (
	"fmt"
	"log/slog"
	"os"

	ctrl "sigs.k8s.io/controller-runtime"
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

		if err := standardSigner.RegisterSecretWatch(
			mgr,
			cfg.JwtSecretName,
			cfg.Namespace,
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
