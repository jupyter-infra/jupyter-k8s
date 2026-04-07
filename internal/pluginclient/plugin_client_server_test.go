/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/go-logr/logr"
	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/pluginclient"
	"github.com/jupyter-infra/jupyter-k8s/internal/pluginserver"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubRemoteAccessHandler implements plugin.RemoteAccessPluginApis for integration testing.
type stubRemoteAccessHandler struct{}

func (h *stubRemoteAccessHandler) Initialize(_ context.Context, _ *pluginapi.InitializeRequest) (*pluginapi.InitializeResponse, error) {
	return &pluginapi.InitializeResponse{}, nil
}

func (h *stubRemoteAccessHandler) RegisterNodeAgent(_ context.Context, req *pluginapi.RegisterNodeAgentRequest) (*pluginapi.RegisterNodeAgentResponse, error) {
	return &pluginapi.RegisterNodeAgentResponse{
		ActivationID:   "act-" + req.PodUID,
		ActivationCode: "code-" + req.PodUID,
	}, nil
}

func (h *stubRemoteAccessHandler) DeregisterNodeAgent(_ context.Context, _ *pluginapi.DeregisterNodeAgentRequest) (*pluginapi.DeregisterNodeAgentResponse, error) {
	return &pluginapi.DeregisterNodeAgentResponse{}, nil
}

func (h *stubRemoteAccessHandler) CreateSession(_ context.Context, req *pluginapi.CreateSessionRequest) (*pluginapi.CreateSessionResponse, error) {
	return &pluginapi.CreateSessionResponse{
		ConnectionURL: "vscode://connect?ws=" + req.WorkspaceName,
	}, nil
}

func setupIntegrationServer(t *testing.T) string {
	t.Helper()
	srv := pluginserver.NewPluginServer(pluginserver.ServerConfig{
		RemoteAccessHandler: &stubRemoteAccessHandler{},
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts.URL
}

func TestIntegration_RemoteAccess_Initialize(t *testing.T) {
	baseURL := setupIntegrationServer(t)
	client := pluginclient.NewPluginClient(baseURL, logr.Discard())

	_, err := client.Initialize(context.Background(), &pluginapi.InitializeRequest{})
	require.NoError(t, err)
}

func TestIntegration_RemoteAccess_RegisterNodeAgent(t *testing.T) {
	baseURL := setupIntegrationServer(t)
	client := pluginclient.NewPluginClient(baseURL, logr.Discard())

	resp, err := client.RegisterNodeAgent(context.Background(), &pluginapi.RegisterNodeAgentRequest{
		PodUID:        "pod-123",
		WorkspaceName: "ws1",
		Namespace:     "ns1",
	})
	require.NoError(t, err)
	assert.Equal(t, "act-pod-123", resp.ActivationID)
	assert.Equal(t, "code-pod-123", resp.ActivationCode)
}

func TestIntegration_RemoteAccess_DeregisterNodeAgent(t *testing.T) {
	baseURL := setupIntegrationServer(t)
	client := pluginclient.NewPluginClient(baseURL, logr.Discard())

	_, err := client.DeregisterNodeAgent(context.Background(), &pluginapi.DeregisterNodeAgentRequest{PodUID: "pod-uid-xyz"})
	require.NoError(t, err)
}

func TestIntegration_RemoteAccess_CreateSession(t *testing.T) {
	baseURL := setupIntegrationServer(t)
	client := pluginclient.NewPluginClient(baseURL, logr.Discard())

	resp, err := client.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:            "pod-uid-123",
		WorkspaceName:     "test-ws",
		Namespace:         "test-ns",
		ConnectionContext: map[string]string{"ssmDocumentName": "doc"},
	})
	require.NoError(t, err)
	assert.Equal(t, "vscode://connect?ws=test-ws", resp.ConnectionURL)
}
