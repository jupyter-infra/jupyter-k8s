/*
MIT License

Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
*/

package authmiddleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// Server represents the HTTP server for authentication middleware
type Server struct {
	config        *Config
	jwtManager    jwt.Handler
	cookieManager CookieHandler
	logger        *slog.Logger
	httpServer    *http.Server
	restClient    rest.Interface
	oidcVerifier  OIDCVerifierInterface
}

// NewServer creates a new server instance
func NewServer(config *Config, jwtManager jwt.Handler, cookieManager CookieHandler, logger *slog.Logger) *Server {
	// Initialize Kubernetes client for in-cluster use
	k8sConfig, err := rest.InClusterConfig()
	var restClient rest.Interface

	if err != nil {
		logger.Error("Failed to create Kubernetes client config", "error", err)
		// We don't exit here, as we can still function without the K8s client
		// for basic auth functionality, just not workspace access verification
		restClient = nil
	} else {
		// Create standard k8s client
		clientset, err := kubernetes.NewForConfig(k8sConfig)
		if err != nil {
			logger.Error("Failed to create Kubernetes REST client", "error", err)
			restClient = nil
		} else {
			restClient = clientset.CoreV1().RESTClient()
		}
	}

	// Initialize OIDC verifier structure (without making HTTP calls) if /auth endpoint is enabled
	var oidcVerifier OIDCVerifierInterface
	if config.EnableOAuth {
		v, err := NewOIDCVerifier(config, logger)
		if err != nil {
			logger.Error("Failed to create OIDC verifier", "error", err)
			oidcVerifier = nil
		} else {
			oidcVerifier = v
		}
	}

	return &Server{
		config:        config,
		jwtManager:    jwtManager,
		cookieManager: cookieManager,
		logger:        logger,
		restClient:    restClient,
		oidcVerifier:  oidcVerifier,
	}
}

// Start initializes and starts the HTTP server
func (s *Server) Start() error {
	// Initialize OIDC verifier if enabled
	if s.config.EnableOAuth && s.oidcVerifier != nil {
		s.logger.Info("Initializing OIDC verifier connection")
		if err := s.oidcVerifier.Start(context.Background()); err != nil {
			s.logger.Error("Failed to start OIDC verifier", "error", err)
			return fmt.Errorf("failed to start OIDC verifier: %w", err)
		}
		s.logger.Info("OIDC verifier initialized successfully")
	} else {
		s.logger.Info("OAuth disabled, skipping OIDC initialization")
	}

	// Create router
	router := http.NewServeMux()

	// Register routes
	if s.config.EnableOAuth {
		router.HandleFunc("/auth", s.handleAuth)
	}
	if s.config.EnableBearerAuth {
		router.HandleFunc("/bearer-auth", s.handleBearerAuth)
	}
	router.HandleFunc("/verify", s.handleVerify)
	router.HandleFunc("/health", s.handleHealth)

	// Configure HTTP server
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      router,
		ReadTimeout:  s.config.ReadTimeout,
		WriteTimeout: s.config.WriteTimeout,
	}

	// Channel for handling shutdown
	idleConnsClosed := make(chan struct{})

	// Setup graceful shutdown
	go s.handleShutdown(idleConnsClosed)

	// Start server
	s.logger.Info("Starting authentication middleware service", "port", s.config.Port)
	if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}

	<-idleConnsClosed
	s.logger.Info("Server stopped")
	return nil
}

// handleShutdown handles graceful server shutdown
func (s *Server) handleShutdown(idleConnsClosed chan struct{}) {
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, syscall.SIGINT, syscall.SIGTERM)
	<-sigint

	s.logger.Info("Received shutdown signal")

	// Create a deadline to wait for
	ctx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeout)
	defer cancel()

	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline
	if err := s.httpServer.Shutdown(ctx); err != nil {
		s.logger.Error("Server shutdown error", "error", err)
	}

	close(idleConnsClosed)
}

// Handler methods are implemented in separate files:
// - serverroute_auth.go
// - serverroute_verify.go
// - serverroute_health.go
