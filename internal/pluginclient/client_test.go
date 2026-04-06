/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *plugin.StatusError
		expected string
	}{
		{
			name:     "with message",
			err:      &plugin.StatusError{Code: 401, Message: "unauthorized"},
			expected: "plugin error (HTTP 401): unauthorized",
		},
		{
			name:     "without message",
			err:      &plugin.StatusError{Code: 500},
			expected: "plugin error (HTTP 500)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestDoPost_ErrorStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		errMsg     string
	}{
		{"bad request", http.StatusBadRequest, "invalid input"},
		{"unauthorized", http.StatusUnauthorized, "invalid token"},
		{"internal server error", http.StatusInternalServerError, "something broke"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tt.statusCode)
				_ = json.NewEncoder(w).Encode(pluginapi.ErrorResponse{Error: tt.errMsg})
			}))
			defer srv.Close()

			client := NewPluginClient(srv.URL, logr.Discard())
			_, _, err := doPost[pluginapi.SignResponse](context.Background(), client, "/test", &pluginapi.SignRequest{})
			require.Error(t, err)

			var pe *plugin.StatusError
			require.ErrorAs(t, err, &pe)
			assert.Equal(t, tt.statusCode, pe.Code)
			assert.Equal(t, tt.errMsg, pe.Message)
		})
	}
}

func TestDoPost_InvalidResponseJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	client := NewPluginClient(srv.URL, logr.Discard())
	_, _, err := doPost[pluginapi.SignResponse](context.Background(), client, "/test", &pluginapi.SignRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decode response")
}

func TestDoPost_NonJSONErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer srv.Close()

	client := NewPluginClient(srv.URL, logr.Discard())
	_, _, err := doPost[pluginapi.SignResponse](context.Background(), client, "/test", &pluginapi.SignRequest{})
	require.Error(t, err)

	var pe *plugin.StatusError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, http.StatusBadGateway, pe.Code)
	assert.Empty(t, pe.Message) // Non-JSON error body, message is empty
}

func TestDoPost_SendsPluginRequestID(t *testing.T) {
	var receivedPluginID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPluginID = r.Header.Get(plugin.HeaderPluginRequestID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.SignResponse{Token: "t"})
	}))
	defer srv.Close()

	client := NewPluginClient(srv.URL, logr.Discard())
	_, _, err := doPost[pluginapi.SignResponse](context.Background(), client, "/test", &pluginapi.SignRequest{})
	require.NoError(t, err)
	assert.NotEmpty(t, receivedPluginID)
	assert.Len(t, receivedPluginID, 16)
}

func TestDoPost_PropagatesOriginRequestID(t *testing.T) {
	var receivedOriginID, receivedPluginID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedOriginID = r.Header.Get(plugin.HeaderOriginRequestID)
		receivedPluginID = r.Header.Get(plugin.HeaderPluginRequestID)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.SignResponse{Token: "t"})
	}))
	defer srv.Close()

	ctx := plugin.ContextWithOriginRequestID(context.Background(), "origin-abc-123")
	client := NewPluginClient(srv.URL, logr.Discard())
	_, _, err := doPost[pluginapi.SignResponse](ctx, client, "/test", &pluginapi.SignRequest{})
	require.NoError(t, err)
	assert.Equal(t, "origin-abc-123", receivedOriginID)
	assert.NotEmpty(t, receivedPluginID)
	assert.NotEqual(t, receivedOriginID, receivedPluginID)
}

func TestDoPost_NoOriginHeader_WhenNotInContext(t *testing.T) {
	var hasOriginHeader bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hasOriginHeader = r.Header.Get(plugin.HeaderOriginRequestID) != ""
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.SignResponse{Token: "t"})
	}))
	defer srv.Close()

	client := NewPluginClient(srv.URL, logr.Discard())
	_, _, err := doPost[pluginapi.SignResponse](context.Background(), client, "/test", &pluginapi.SignRequest{})
	require.NoError(t, err)
	assert.False(t, hasOriginHeader)
}

// newTestClient creates a PluginClient with fast retry settings for tests.
func newTestClient(baseURL string) *PluginClient {
	c := NewPluginClient(baseURL, logr.Discard())
	c.retryCount = 2
	c.retryDelay = 10 * time.Millisecond // fast retries for tests
	c.callTimeout = 2 * time.Second
	return c
}

func TestDoPost_RetriesOnConnectionRefused(t *testing.T) {
	// Grab a fixed listener so the client address stays valid across retries.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	// Close the listener so the port is unbound — first attempts get ECONNREFUSED.
	require.NoError(t, ln.Close())

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.SignResponse{Token: "recovered"})
	})

	client := newTestClient("http://" + addr)

	// Re-bind the same port after a delay, simulating the sidecar coming up.
	srv := &http.Server{Handler: handler}
	listenErr := make(chan error, 1)
	go func() {
		time.Sleep(15 * time.Millisecond)
		newLn, err := net.Listen("tcp", addr)
		if err != nil {
			listenErr <- err
			return
		}
		listenErr <- nil
		_ = srv.Serve(newLn) // returns when listener is closed
	}()
	defer func() { _ = srv.Close() }()

	resp, _, err := doPost[pluginapi.SignResponse](context.Background(), client, "/test", &pluginapi.SignRequest{})
	require.NoError(t, <-listenErr, "failed to re-bind listener in background")
	require.NoError(t, err)
	assert.Equal(t, "recovered", resp.Token)
}

func TestDoPost_DoesNotRetryOnHTTPError(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(pluginapi.ErrorResponse{Error: "plugin internal error"})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	_, _, err := doPost[pluginapi.SignResponse](context.Background(), client, "/test", &pluginapi.SignRequest{})
	require.Error(t, err)

	// Should have made exactly 1 attempt — no retries on HTTP responses.
	assert.Equal(t, 1, attempts)

	var pe *plugin.StatusError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, http.StatusInternalServerError, pe.Code)
}

func TestDoPost_DoesNotRetryOnContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	client := newTestClient("http://127.0.0.1:1")
	_, _, err := doPost[pluginapi.SignResponse](ctx, client, "/test", &pluginapi.SignRequest{})
	require.Error(t, err)
	// Should fail immediately without retrying.
	assert.ErrorIs(t, err, context.Canceled)
}

func TestDoPost_RetriesExhausted_ReturnsLastError(t *testing.T) {
	// All attempts fail with connection refused.
	client := newTestClient("http://127.0.0.1:1")
	_, _, err := doPost[pluginapi.SignResponse](context.Background(), client, "/test", &pluginapi.SignRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "3 attempts") // retryCount(2) + 1 = 3
}

func TestDoPost_SucceedsAfterTransientFailure(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts++
		if attempts <= 1 {
			// Simulate transient failure by closing the connection.
			hj, ok := w.(http.Hijacker)
			require.True(t, ok, "ResponseWriter must support Hijacker")
			conn, _, err := hj.Hijack()
			require.NoError(t, err, "Hijack must succeed")
			require.NoError(t, conn.Close(), "connection close must succeed")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.SignResponse{Token: "recovered"})
	}))
	defer srv.Close()

	client := newTestClient(srv.URL)
	resp, _, err := doPost[pluginapi.SignResponse](context.Background(), client, "/test", &pluginapi.SignRequest{})
	require.NoError(t, err)
	assert.Equal(t, "recovered", resp.Token)
	assert.Equal(t, 2, attempts)
}
