/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitialize_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, pluginapi.RouteRemoteAccessInit.Path, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.InitializeResponse{})
	}))
	defer srv.Close()

	client := NewPluginClient(srv.URL, logr.Discard())
	_, err := client.Initialize(context.Background(), &pluginapi.InitializeRequest{})
	require.NoError(t, err)
}

func TestInitialize_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(pluginapi.ErrorResponse{Error: "init failed"})
	}))
	defer srv.Close()

	client := NewPluginClient(srv.URL, logr.Discard())
	_, err := client.Initialize(context.Background(), &pluginapi.InitializeRequest{})
	require.Error(t, err)

	var pe *plugin.StatusError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, http.StatusInternalServerError, pe.Code)
}

func TestRegisterNodeAgent_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, pluginapi.RouteRegisterNodeAgent.Path, r.URL.Path)

		var req pluginapi.RegisterNodeAgentRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "pod-uid-123", req.PodUID)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.RegisterNodeAgentResponse{
			ActivationID:   "act-123",
			ActivationCode: "code-456",
		})
	}))
	defer srv.Close()

	client := NewPluginClient(srv.URL, logr.Discard())
	resp, err := client.RegisterNodeAgent(context.Background(), &pluginapi.RegisterNodeAgentRequest{
		PodUID:        "pod-uid-123",
		WorkspaceName: "test-ws",
		Namespace:     "test-ns",
	})
	require.NoError(t, err)
	assert.Equal(t, "act-123", resp.ActivationID)
	assert.Equal(t, "code-456", resp.ActivationCode)
}

func TestDeregisterNodeAgent_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, pluginapi.RouteDeregisterNodeAgent.Path, r.URL.Path)

		var req pluginapi.DeregisterNodeAgentRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "pod-uid-123", req.PodUID)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.DeregisterNodeAgentResponse{})
	}))
	defer srv.Close()

	client := NewPluginClient(srv.URL, logr.Discard())
	_, err := client.DeregisterNodeAgent(context.Background(), &pluginapi.DeregisterNodeAgentRequest{PodUID: "pod-uid-123"})
	require.NoError(t, err)
}

func TestDeregisterNodeAgent_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(pluginapi.ErrorResponse{Error: "cleanup failed"})
	}))
	defer srv.Close()

	client := NewPluginClient(srv.URL, logr.Discard())
	_, err := client.DeregisterNodeAgent(context.Background(), &pluginapi.DeregisterNodeAgentRequest{PodUID: "pod-uid-123"})
	require.Error(t, err)

	var pe *plugin.StatusError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, "cleanup failed", pe.Message)
}

func TestCreateSession_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, pluginapi.RouteCreateSession.Path, r.URL.Path)

		var req pluginapi.CreateSessionRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "pod-uid-123", req.PodUID)
		assert.Equal(t, "test-ws", req.WorkspaceName)
		assert.Equal(t, "test-ns", req.Namespace)
		assert.Equal(t, "test-doc", req.ConnectionContext["ssmDocumentName"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.CreateSessionResponse{
			ConnectionURL: "vscode://jupyter.workspace/connect?session=abc",
		})
	}))
	defer srv.Close()

	client := NewPluginClient(srv.URL, logr.Discard())
	resp, err := client.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:        "pod-uid-123",
		WorkspaceName: "test-ws",
		Namespace:     "test-ns",
		ConnectionContext: map[string]string{
			"ssmDocumentName": "test-doc",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "vscode://jupyter.workspace/connect?session=abc", resp.ConnectionURL)
}

func TestCreateSession_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(pluginapi.ErrorResponse{Error: "session failed"})
	}))
	defer srv.Close()

	client := NewPluginClient(srv.URL, logr.Discard())
	_, err := client.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:        "pod-uid-123",
		WorkspaceName: "test-ws",
		Namespace:     "test-ns",
	})
	require.Error(t, err)
}

func TestRemoteAccess_ConnectionRefused(t *testing.T) {
	client := NewPluginClient("http://127.0.0.1:1", logr.Discard())
	client.retryCount = 0 // no retries — fail fast for this test

	_, err := client.Initialize(context.Background(), &pluginapi.InitializeRequest{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")

	_, err = client.DeregisterNodeAgent(context.Background(), &pluginapi.DeregisterNodeAgentRequest{PodUID: "uid"})
	require.Error(t, err)

	_, err = client.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:        "uid",
		WorkspaceName: "ws",
		Namespace:     "ns",
	})
	require.Error(t, err)
}
