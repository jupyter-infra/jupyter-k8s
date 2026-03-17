/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
)

// HeaderRequestID is the HTTP header used to pass a request ID between the controller and plugin.
const HeaderRequestID = "X-Request-ID"

// requestIDKey is the context key for the request ID.
type requestIDKey struct{}

// RequestIDFromContext returns the request ID from the context, or empty string if not set.
func RequestIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey{}).(string); ok {
		return id
	}
	return ""
}

// Server is the HTTP server that routes requests to plugin handler implementations.
type Server struct {
	jwtHandler          JWTHandler
	remoteAccessHandler RemoteAccessHandler
	httpServer          *http.Server
	mux                 *http.ServeMux
	logger              *slog.Logger
}

// NewServer creates a new plugin Server with the given handlers and port.
func NewServer(jwtHandler JWTHandler, remoteAccessHandler RemoteAccessHandler, port int) *Server {
	mux := http.NewServeMux()
	logger := slog.Default()
	s := &Server{
		jwtHandler:          jwtHandler,
		remoteAccessHandler: remoteAccessHandler,
		mux:                 mux,
		logger:              logger,
		httpServer: &http.Server{
			Addr:    fmt.Sprintf(":%d", port),
			Handler: mux,
		},
	}
	s.registerRoutes()
	return s
}

// Handler returns the underlying http.Handler for testing.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	s.logger.Info("starting plugin server", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully drains in-flight requests and stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down plugin server")
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) registerRoutes() {
	s.mux.Handle("POST /v1alpha1/jwt/sign", s.withMiddleware(s.handleNotImplemented))
	s.mux.Handle("POST /v1alpha1/jwt/verify", s.withMiddleware(s.handleNotImplemented))
	s.mux.Handle("POST /v1alpha1/remote-access/initialize", s.withMiddleware(s.handleNotImplemented))
	s.mux.Handle("POST /v1alpha1/remote-access/register-node", s.withMiddleware(s.handleNotImplemented))
	s.mux.Handle("POST /v1alpha1/remote-access/deregister-node", s.withMiddleware(s.handleNotImplemented))
	s.mux.Handle("POST /v1alpha1/remote-access/create-session", s.withMiddleware(s.handleNotImplemented))
	s.mux.Handle("GET /healthz", s.withMiddleware(s.handleHealthz))
}

// withMiddleware wraps a handler with request ID extraction, panic recovery, and request logging.
func (s *Server) withMiddleware(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Extract or generate request ID
		requestID := r.Header.Get(HeaderRequestID)
		if requestID == "" {
			requestID = generateRequestID()
		}

		// Add request ID to response header and request context
		w.Header().Set(HeaderRequestID, requestID)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		r = r.WithContext(ctx)

		logger := s.logger.With("requestID", requestID)

		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"duration", time.Since(start),
					"error", rec,
					"stack", string(debug.Stack()),
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(pluginapi.ErrorResponse{Error: "internal server error"})
			}
		}()

		next.ServeHTTP(w, r)

		logger.Info("request handled",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(struct{}{})
}

func (s *Server) handleNotImplemented(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented) // 501
	_ = json.NewEncoder(w).Encode(pluginapi.ErrorResponse{Error: "not implemented"})
}
