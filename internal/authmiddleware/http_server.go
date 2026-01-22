package authmiddleware

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
)

// HTTPServerRunnable wraps the Server to make it compatible with the
// controller-runtime Manager's Runnable interface.
//
// This adapter is necessary because:
// 1. controller-runtime Manager expects Runnables with Start(ctx) that respect context cancellation
// 2. Server.Start() blocks on http.ListenAndServe() and doesn't support graceful shutdown natively
// 3. We need to load initial JWT signing keys before starting the HTTP server
//
// The adapter handles:
// - Initial secret loading (for StandardSigner only, deferred until Start to avoid network calls during construction)
// - Starting the HTTP server in a goroutine
// - Graceful shutdown when context is cancelled
// - Propagating server errors back to the manager
type HTTPServerRunnable struct {
	server         *Server
	logger         logr.Logger
	runtimeClient  client.Client
	standardSigner *jwt.StandardSigner
	secretName     string
	namespace      string
}

// NewHTTPServerRunnable creates a new HTTPServerRunnable.
// If standardSigner is not nil, it will load the initial JWT signing keys before starting the server.
func NewHTTPServerRunnable(
	server *Server,
	logger logr.Logger,
	runtimeClient client.Client,
	standardSigner *jwt.StandardSigner,
	secretName string,
	namespace string,
) *HTTPServerRunnable {
	return &HTTPServerRunnable{
		server:         server,
		logger:         logger,
		runtimeClient:  runtimeClient,
		standardSigner: standardSigner,
		secretName:     secretName,
		namespace:      namespace,
	}
}

// Start implements the Runnable interface. It starts the HTTP server
// and blocks until the context is cancelled.
func (h *HTTPServerRunnable) Start(ctx context.Context) error {
	h.logger.Info("Starting HTTP server runnable")

	// Load initial JWT signing keys if using standard signing
	if h.standardSigner != nil {
		h.logger.Info("Loading initial JWT signing keys from secret",
			"secret", h.secretName,
			"namespace", h.namespace)

		// Retrieve initial secret and load keys
		if err := h.standardSigner.RetrieveInitialSecret(
			ctx,
			h.runtimeClient,
			h.secretName,
			h.namespace,
		); err != nil {
			return fmt.Errorf("failed to retrieve initial secret: %w", err)
		}

		h.logger.Info("Successfully loaded initial JWT signing keys")
	}

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := h.server.Start(); err != nil {
			errChan <- err
		}
	}()

	// Wait for either context cancellation or server error
	select {
	case <-ctx.Done():
		h.logger.Info("Context cancelled, shutting down HTTP server")
		if err := h.server.Shutdown(ctx); err != nil {
			h.logger.Error(err, "Error during HTTP server shutdown")
			return err
		}
		return nil
	case err := <-errChan:
		h.logger.Error(err, "HTTP server error")
		return err
	}
}

// NeedLeaderElection implements the Runnable interface.
// Returns false because the HTTP server should run on all replicas.
func (h *HTTPServerRunnable) NeedLeaderElection() bool {
	return false
}
