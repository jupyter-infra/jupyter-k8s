// Package main provides the entry point for the authmiddleware service that
// handles JWT-based authentication and authorization for Jupyter-k8s workspaces.
package main

import (
	"log/slog"
	"os"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/authmiddleware"
)

func main() {
	// Initialize logger
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Load configuration
	cfg, err := authmiddleware.NewConfig()
	if err != nil {
		logger.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Create JWT manager
	jwtManager := authmiddleware.NewJWTManager(cfg)

	// Create cookie manager
	cookieManager, err := authmiddleware.NewCookieManager(cfg)
	if err != nil {
		logger.Error("Failed to create cookie manager", "error", err)
		os.Exit(1)
	}

	// Create and start server
	server := authmiddleware.NewServer(cfg, jwtManager, cookieManager, logger)
	if err := server.Start(); err != nil {
		logger.Error("Server failed", "error", err)
		os.Exit(1)
	}
}
