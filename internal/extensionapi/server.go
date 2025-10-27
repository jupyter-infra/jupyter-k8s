// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/client-go/kubernetes"
	v1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	setupLog = log.Log.WithName("extension-api-server")
)

// ExtensionServer represents the extension API HTTP server
type ExtensionServer struct {
	config     *ExtensionConfig
	k8sClient  client.Client
	sarClient  v1.SubjectAccessReviewInterface
	logger     *logr.Logger
	httpServer interface {
		ListenAndServe() error
		ListenAndServeTLS(certFile, keyFile string) error
		Shutdown(ctx context.Context) error
	}
	routes map[string]func(http.ResponseWriter, *http.Request)
	mux    *http.ServeMux
}

// newExtensionServer creates a new extension API server
func newExtensionServer(
	config *ExtensionConfig,
	logger *logr.Logger,
	k8sClient client.Client,
	sarClient v1.SubjectAccessReviewInterface) *ExtensionServer {
	mux := http.NewServeMux()

	server := &ExtensionServer{
		config:    config,
		logger:    logger,
		k8sClient: k8sClient,
		sarClient: sarClient,
		routes:    make(map[string]func(http.ResponseWriter, *http.Request)),
		mux:       mux,
		httpServer: &http.Server{
			Addr:         fmt.Sprintf(":%d", config.ServerPort),
			Handler:      mux,
			ReadTimeout:  time.Duration(config.ReadTimeoutSeconds) * time.Second,
			WriteTimeout: time.Duration(config.WriteTimeoutSeconds) * time.Second,
		},
	}

	return server
}

// loggerMiddleware wraps an http.Handler and adds a logger to the request context
func (s *ExtensionServer) loggerMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Create request-specific logger with path info
		reqLogger := (*s.logger).WithValues(
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)

		// Create new context with logger
		ctx := AddLoggerToContext(r.Context(), reqLogger)

		// Call next handler with the augmented context
		next(w, r.WithContext(ctx))
	}
}

// registerRoute registers a route handler
func (s *ExtensionServer) registerRoute(name string, handler func(http.ResponseWriter, *http.Request)) {
	// Store original handler in routes map
	s.routes[name] = handler

	// Ensure the path starts with a slash
	path := name
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	// Wrap handler with the logger middleware
	wrappedHandler := s.loggerMiddleware(handler)

	// Register the wrapped handler
	s.mux.HandleFunc(path, wrappedHandler)
}

// registerNamespacedRoutes registers multiple route handlers for resources with namespaces in the URL path
// It efficiently handles paths like "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/{namespace}/{resource}"
// by registering a single handler for the namespaced path prefix and routing to the appropriate handler
func (s *ExtensionServer) registerNamespacedRoutes(resourceHandlers map[string]func(http.ResponseWriter, *http.Request)) {
	// Store all the resource handlers in the routes map for reference
	basePattern := s.config.ApiPath
	namespacedPathPrefix := basePattern + "/namespaces/"

	// For each resource, store it in the routes map with a descriptive pattern
	for resource, handler := range resourceHandlers {
		pattern := namespacedPathPrefix + "*/" + resource
		s.routes[pattern] = handler
		setupLog.Info("Added namespaced route", "resource", resource, "pattern", pattern)
	}

	// Create a single wrapped handler that will route to the appropriate resource handler
	wrappedHandler := s.loggerMiddleware(func(w http.ResponseWriter, r *http.Request) {
		// Extract namespace from path
		namespace, err := GetNamespaceFromPath(r.URL.Path)
		if err != nil {
			WriteError(w, http.StatusBadRequest, "Invalid or missing namespace in path")
			return
		}

		// Extract the resource name from the path
		// The path format is /apis/group/version/namespaces/namespace/resource
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 6 {
			http.NotFound(w, r)
			return
		}

		// The resource should be the last part of the path
		resource := parts[len(parts)-1]

		// Find the handler for this resource
		if handler, ok := resourceHandlers[resource]; ok {
			setupLog.Info("Handling namespaced request",
				"path", r.URL.Path,
				"namespace", namespace,
				"resource", resource)
			handler(w, r)
		} else {
			http.NotFound(w, r)
		}
	})

	// Register the single wrapped handler for the namespaced path prefix
	s.mux.HandleFunc(namespacedPathPrefix, wrappedHandler)
	setupLog.Info("Registered namespaced routes handler", "pathPrefix", namespacedPathPrefix)
}

// registerAllRoutes register the actual routes to the server
func (s *ExtensionServer) registerAllRoutes() {
	// Register health check route
	s.registerRoute("/health", s.handleHealth)

	// Register API discovery route
	s.registerRoute(s.config.ApiPath, s.handleDiscovery)

	// Register all namespaced routes
	s.registerNamespacedRoutes(map[string]func(http.ResponseWriter, *http.Request){
		"connections":            s.HandleConnectionCreate,
		"connectionaccessreview": s.handleConnectionAccessReview,
	})
}

// Start starts the extension API server and implements the controller-runtime's Runnable interface
func (s *ExtensionServer) Start(ctx context.Context) error {
	setupLog.Info("Starting extension API server", "port", s.config.ServerPort, "disableTLS", s.config.DisableTLS)

	// Error channel to capture server errors
	errChan := make(chan error, 1)

	// Start the server in a goroutine
	go func() {
		var err error

		if s.config.DisableTLS {
			// Start HTTP server
			setupLog.Info("Starting server with HTTP (TLS disabled)")
			err = s.httpServer.ListenAndServe()
		} else {
			// Configure TLS
			// We don't need to set TLSConfig as it's handled by ListenAndServeTLS

			// Start HTTPS server
			setupLog.Info("Starting server with HTTPS", "certPath", s.config.CertPath, "keyPath", s.config.KeyPath)
			err = s.httpServer.ListenAndServeTLS(s.config.CertPath, s.config.KeyPath)
		}

		if err != nil && err != http.ErrServerClosed {
			setupLog.Error(err, "Extension API server failed")
			errChan <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		// Context was canceled, gracefully shutdown the server
		setupLog.Info("Shutting down extension API server due to context cancellation")
		return s.Stop(context.Background())
	case err := <-errChan:
		// Server encountered an error
		return fmt.Errorf("extension API server failed: %w", err)
	}
}

// NeedLeaderElection implements the LeaderElectionRunnable interface
// This indicates this runnable doesn't need to be a leader to run
func (s *ExtensionServer) NeedLeaderElection() bool {
	return false
}

// Stop stops the extension API server
func (s *ExtensionServer) Stop(ctx context.Context) error {
	setupLog.Info("Shutting down extension API server")
	return s.httpServer.Shutdown(ctx)
}

// SetupExtensionAPIServerWithManager sets up the extension API server and adds it to the manager
func SetupExtensionAPIServerWithManager(mgr ctrl.Manager, config *ExtensionConfig) error {
	// Use the config or create a default config
	if config == nil {
		config = NewConfig()
	}

	// Retrieve the logger
	logger := mgr.GetLogger().WithName("extension-api")

	// Retrieve the k8s client
	k8sClient := mgr.GetClient()

	// Retrieve the sar client
	clientSet, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		return fmt.Errorf("failed to instantiate the sar client: %w", err)
	}
	sarClient := clientSet.AuthorizationV1().SubjectAccessReviews()

	// Create server with config
	server := newExtensionServer(config, &logger, k8sClient, sarClient)
	server.registerAllRoutes()

	// Add the server as a runnable to the manager
	if err := mgr.Add(server); err != nil {
		return fmt.Errorf("failed to add extension API server to manager: %w", err)
	}

	return nil
}
