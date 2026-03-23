/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"
)

// pluginRequestIDKey is the context key for the plugin request ID.
type pluginRequestIDKey struct{}

// PluginRequestID returns the plugin request ID from the context, or empty string.
func PluginRequestID(ctx context.Context) string {
	if id, ok := ctx.Value(pluginRequestIDKey{}).(string); ok {
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

// NewServer creates a new plugin Server from the given config.
// The server listens on localhost only, since the plugin runs as a sidecar
// in the same pod as the controller. Nil handlers default to returning
// 501 Not Implemented for all their endpoints.
func NewServer(cfg ServerConfig) *Server {
	cfg.applyDefaults()
	if cfg.JWTHandler == nil {
		cfg.JWTHandler = NotImplementedJWTHandler{}
	}
	if cfg.RemoteAccessHandler == nil {
		cfg.RemoteAccessHandler = NotImplementedRemoteAccessHandler{}
	}

	mux := http.NewServeMux()
	logger := slog.Default()
	s := &Server{
		jwtHandler:          cfg.JWTHandler,
		remoteAccessHandler: cfg.RemoteAccessHandler,
		mux:                 mux,
		logger:              logger,
		httpServer: &http.Server{
			Addr:    fmt.Sprintf("%s:%d", cfg.ListenAddress, cfg.Port),
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
	s.mux.Handle(pluginapi.RouteJWTSign.Pattern(), s.withMiddleware(s.handleJWTSign))
	s.mux.Handle(pluginapi.RouteJWTVerify.Pattern(), s.withMiddleware(s.handleJWTVerify))
	s.mux.Handle(pluginapi.RouteRemoteAccessInit.Pattern(), s.withMiddleware(s.handleRemoteAccessInit))
	s.mux.Handle(pluginapi.RouteRegisterNodeAgent.Pattern(), s.withMiddleware(s.handleRegisterNodeAgent))
	s.mux.Handle(pluginapi.RouteDeregisterNodeAgent.Pattern(), s.withMiddleware(s.handleDeregisterNodeAgent))
	s.mux.Handle(pluginapi.RouteCreateSession.Pattern(), s.withMiddleware(s.handleCreateSession))
	s.mux.Handle(pluginapi.RouteHealthz.Pattern(), s.withMiddleware(s.handleHealthz))
}

// withMiddleware wraps a handler with request ID extraction, panic recovery, and request logging.
func (s *Server) withMiddleware(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Extract or generate plugin request ID
		pluginRequestID := r.Header.Get(plugin.HeaderPluginRequestID)
		if pluginRequestID == "" {
			pluginRequestID = plugin.GenerateRequestID()
		}
		originRequestID := r.Header.Get(plugin.HeaderOriginRequestID)

		// Echo plugin request ID back in response
		w.Header().Set(plugin.HeaderPluginRequestID, pluginRequestID)

		// Add plugin request ID to request context
		ctx := context.WithValue(r.Context(), pluginRequestIDKey{}, pluginRequestID)
		r = r.WithContext(ctx)

		// Build sub-logger with both IDs
		logger := s.logger.With("pluginRequestID", pluginRequestID)
		if originRequestID != "" {
			logger = logger.With("originRequestID", originRequestID)
		}

		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"duration", time.Since(start),
					"error", rec,
					"stack", string(debug.Stack()),
				)
				writeJSON(w, http.StatusInternalServerError, pluginapi.ErrorResponse{Error: "internal server error"})
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

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct{}{})
}

// handleRequest is a generic handler that decodes the request, calls the handler function,
// and encodes the response.
func handleRequest[Req any, Resp any](w http.ResponseWriter, r *http.Request, handlerFn func(context.Context, *Req) (*Resp, error)) {
	var req Req
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, pluginapi.ErrorResponse{Error: "invalid request body: " + err.Error()})
		return
	}

	resp, err := handlerFn(r.Context(), &req)
	if err != nil {
		// Handlers return *plugin.StatusError for known errors (e.g. 400, 401) where
		// the handler deliberately chose the HTTP status code.
		// Any other error is treated as an unexpected failure and returns 500.
		if se, ok := err.(*plugin.StatusError); ok {
			writeJSON(w, se.Code, pluginapi.ErrorResponse{Error: se.Message})
			return
		}
		writeJSON(w, http.StatusInternalServerError, pluginapi.ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleJWTSign(w http.ResponseWriter, r *http.Request) {
	handleRequest(w, r, s.jwtHandler.Sign)
}

func (s *Server) handleJWTVerify(w http.ResponseWriter, r *http.Request) {
	handleRequest(w, r, s.jwtHandler.Verify)
}

func (s *Server) handleRemoteAccessInit(w http.ResponseWriter, r *http.Request) {
	handleRequest(w, r, s.remoteAccessHandler.Initialize)
}

func (s *Server) handleRegisterNodeAgent(w http.ResponseWriter, r *http.Request) {
	handleRequest(w, r, s.remoteAccessHandler.RegisterNodeAgent)
}

func (s *Server) handleDeregisterNodeAgent(w http.ResponseWriter, r *http.Request) {
	handleRequest(w, r, s.remoteAccessHandler.DeregisterNodeAgent)
}

func (s *Server) handleCreateSession(w http.ResponseWriter, r *http.Request) {
	handleRequest(w, r, s.remoteAccessHandler.CreateSession)
}
