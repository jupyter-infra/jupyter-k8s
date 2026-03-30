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
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func testPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ws-pod-0",
			Namespace: "test-ns",
			UID:       types.UID("pod-uid-123"),
		},
	}
}

func testAccessStrategy() *workspacev1alpha1.WorkspaceAccessStrategy {
	return &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			PodEventsHandler: "aws",
			PodEventsContext: map[string]string{
				"ssmDocumentName": "test-doc",
			},
			CreateConnectionHandler: "aws",
			CreateConnectionContext: map[string]string{
				"ssmDocumentName": "test-doc",
			},
		},
	}
}

func TestInitialize_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, pluginapi.RouteRemoteAccessInit.Path, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.InitializeResponse{})
	}))
	defer srv.Close()

	client := NewPluginRemoteAccessClient(NewPluginClient(srv.URL, nil))
	err := client.Initialize(context.Background())
	require.NoError(t, err)
}

func TestInitialize_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(pluginapi.ErrorResponse{Error: "init failed"})
	}))
	defer srv.Close()

	client := NewPluginRemoteAccessClient(NewPluginClient(srv.URL, nil))
	err := client.Initialize(context.Background())
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

	client := NewPluginRemoteAccessClient(NewPluginClient(srv.URL, nil))
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

	client := NewPluginRemoteAccessClient(NewPluginClient(srv.URL, nil))
	err := client.DeregisterNodeAgent(context.Background(), testPod())
	require.NoError(t, err)
}

func TestDeregisterNodeAgent_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(pluginapi.ErrorResponse{Error: "cleanup failed"})
	}))
	defer srv.Close()

	client := NewPluginRemoteAccessClient(NewPluginClient(srv.URL, nil))
	err := client.DeregisterNodeAgent(context.Background(), testPod())
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

	client := NewPluginRemoteAccessClient(NewPluginClient(srv.URL, nil))
	url, err := client.CreateSession(context.Background(), "test-ws", "test-ns", "pod-uid-123", testAccessStrategy())
	require.NoError(t, err)
	assert.Equal(t, "vscode://jupyter.workspace/connect?session=abc", url)
}

func TestCreateSession_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(pluginapi.ErrorResponse{Error: "session failed"})
	}))
	defer srv.Close()

	client := NewPluginRemoteAccessClient(NewPluginClient(srv.URL, nil))
	_, err := client.CreateSession(context.Background(), "test-ws", "test-ns", "pod-uid-123", testAccessStrategy())
	require.Error(t, err)
}

func TestRemoteAccess_ConnectionRefused(t *testing.T) {
	client := NewPluginRemoteAccessClient(NewPluginClient("http://127.0.0.1:1", nil))
	client.client.retryCount = 0 // no retries — fail fast for this test

	err := client.Initialize(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")

	err = client.DeregisterNodeAgent(context.Background(), testPod())
	require.Error(t, err)

	_, err = client.CreateSession(context.Background(), "ws", "ns", "uid", testAccessStrategy())
	require.Error(t, err)
}
