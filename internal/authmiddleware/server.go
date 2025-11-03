package authmiddleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/jwt"
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

	return &Server{
		config:        config,
		jwtManager:    jwtManager,
		cookieManager: cookieManager,
		logger:        logger,
		restClient:    restClient,
	}
}

// Start initializes and starts the HTTP server
func (s *Server) Start() error {
	// Create router
	router := http.NewServeMux()

	// Register routes
	// todo: add flag to turn off oauth
	router.HandleFunc("/auth", s.handleAuth)
	if s.config.EnableBearerAuth {
		router.HandleFunc("/bearer-auth", s.handleBearerAuth)
	}
	router.HandleFunc("/verify", s.handleVerify)
	router.HandleFunc("/health", s.handleHealth)

	// Apply CSRF protection
	handler := s.csrfProtect()(router)

	// Configure HTTP server
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      handler,
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

// csrfProtect returns middleware that applies appropriate CSRF protection based on endpoint
func (s *Server) csrfProtect() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// CSRF protection strategy:
			//
			// 1. /auth - No CSRF protection
			//    - Initial authentication endpoint where users get their first token
			//    - Users don't have CSRF tokens yet when they authenticate
			//    - Similar to "login page exception" in standard CSRF protection patterns
			//
			// 2. /health - No CSRF protection
			//    - Read-only status endpoint used by monitoring systems and load balancers
			//    - No state changes occur here, so CSRF is not applicable
			//    - Monitoring systems don't handle CSRF tokens
			//
			// 3. All other endpoints (including /verify) - Apply CSRF protection
			//    - /verify can refresh tokens, which modifies state
			//    - State-changing operations need CSRF protection
			//    - Clients should have received CSRF tokens from /auth endpoint

			// Skip CSRF protection for specific endpoints
			if r.URL.Path == "/auth" || r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			// Apply CSRF protection for all other endpoints (including /verify)
			s.cookieManager.CSRFProtect()(next).ServeHTTP(w, r)
		})
	}
}

// Handler methods are implemented in separate files:
// - serverroute_auth.go
// - serverroute_verify.go
// - serverroute_health.go
