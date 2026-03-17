/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
)

func newTestServer() *Server {
	// Handlers are unused since all routes return 501 in the shell.
	return NewServer(nil, nil, 0)
}


func TestHealthz(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestPanicRecovery(t *testing.T) {
	srv := newTestServer()
	panicHandler := srv.withMiddleware(func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	panicHandler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}

	var errResp pluginapi.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.Error != "internal server error" {
		t.Errorf("expected error %q, got %q", "internal server error", errResp.Error)
	}
}

func TestRequestID_PassedThrough(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(HeaderRequestID, "caller-id-123")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	got := w.Header().Get(HeaderRequestID)
	if got != "caller-id-123" {
		t.Errorf("expected request ID %q echoed back, got %q", "caller-id-123", got)
	}
}

func TestRequestID_GeneratedWhenMissing(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	got := w.Header().Get(HeaderRequestID)
	if got == "" {
		t.Error("expected a generated request ID, got empty string")
	}
	if len(got) != 16 { // 8 bytes = 16 hex chars
		t.Errorf("expected 16-char hex request ID, got %q (len %d)", got, len(got))
	}
}

func TestRequestID_AvailableInContext(t *testing.T) {
	srv := newTestServer()
	var capturedID string
	handler := srv.withMiddleware(func(_ http.ResponseWriter, r *http.Request) {
		capturedID = RequestIDFromContext(r.Context())
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set(HeaderRequestID, "ctx-test-456")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if capturedID != "ctx-test-456" {
		t.Errorf("expected context request ID %q, got %q", "ctx-test-456", capturedID)
	}
}

func TestRoutes_ReturnNotImplemented(t *testing.T) {
	srv := newTestServer()

	routes := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v1alpha1/jwt/sign"},
		{http.MethodPost, "/v1alpha1/jwt/verify"},
		{http.MethodPost, "/v1alpha1/remote-access/initialize"},
		{http.MethodPost, "/v1alpha1/remote-access/register-node"},
		{http.MethodPost, "/v1alpha1/remote-access/deregister-node"},
		{http.MethodPost, "/v1alpha1/remote-access/create-session"},
	}

	for _, rt := range routes {
		t.Run(rt.path, func(t *testing.T) {
			req := httptest.NewRequest(rt.method, rt.path, nil)
			w := httptest.NewRecorder()
			srv.Handler().ServeHTTP(w, req)

			if w.Code != http.StatusNotImplemented {
				t.Errorf("expected 501, got %d", w.Code)
			}

			var errResp pluginapi.ErrorResponse
			if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
				t.Fatalf("failed to decode error response: %v", err)
			}
			if errResp.Error != "not implemented" {
				t.Errorf("expected error %q, got %q", "not implemented", errResp.Error)
			}
		})
	}
}
