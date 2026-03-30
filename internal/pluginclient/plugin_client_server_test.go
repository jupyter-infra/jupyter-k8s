/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient_test

import (
	"context"
	"net/http/httptest"
	"testing"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"
	"github.com/jupyter-infra/jupyter-k8s/internal/pluginclient"
	"github.com/jupyter-infra/jupyter-k8s/internal/pluginserver"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// stubJWTHandler implements pluginserver.JWTHandler with hardcoded responses for integration testing.
type stubJWTHandler struct{}

func (h *stubJWTHandler) Sign(_ context.Context, req *pluginapi.SignRequest) (*pluginapi.SignResponse, error) {
	return &pluginapi.SignResponse{Token: "signed-" + req.User + "-" + req.TokenType}, nil
}

func (h *stubJWTHandler) Verify(_ context.Context, req *pluginapi.VerifyRequest) (*pluginapi.VerifyResponse, error) {
	if req.Token == "invalid" {
		return nil, &plugin.StatusError{Code: 401, Message: "invalid token"}
	}
	return &pluginapi.VerifyResponse{
		Claims: &pluginapi.VerifyClaims{
			Subject:   "alice",
			Groups:    []string{"admin"},
			UID:       "uid-1",
			Path:      "/ws/ns/ws1",
			Domain:    "example.com",
			TokenType: "bootstrap",
			ExpiresAt: 1700000000,
		},
	}, nil
}

// stubRemoteAccessHandler implements pluginserver.RemoteAccessHandler for integration testing.
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
	srv := pluginserver.NewServer(pluginserver.ServerConfig{
		JWTHandler:          &stubJWTHandler{},
		RemoteAccessHandler: &stubRemoteAccessHandler{},
	})
	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	return ts.URL
}

func TestIntegration_JWTSignAndVerify(t *testing.T) {
	baseURL := setupIntegrationServer(t)
	factory := pluginclient.NewPluginSignerFactory(pluginclient.NewPluginClient(baseURL, nil))

	as := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionContext: map[string]string{"kmsKeyId": "test-key"},
		},
	}
	signer, err := factory.CreateSigner(as)
	require.NoError(t, err)

	// Sign
	token, err := signer.GenerateToken("alice", []string{"admin"}, "uid-1", nil, "/ws/ns/ws1", "example.com", "bootstrap")
	require.NoError(t, err)
	assert.Equal(t, "signed-alice-bootstrap", token)

	// Verify
	claims, err := signer.ValidateToken("any-valid-token")
	require.NoError(t, err)
	assert.Equal(t, "alice", claims.User)
	assert.Equal(t, "alice", claims.Subject)
	assert.Equal(t, []string{"admin"}, claims.Groups)
	assert.Equal(t, "/ws/ns/ws1", claims.Path)
	assert.Equal(t, "example.com", claims.Domain)
	assert.Equal(t, "bootstrap", claims.TokenType)
	assert.Equal(t, int64(1700000000), claims.ExpiresAt.Unix())
}

func TestIntegration_JWTVerify_Unauthorized(t *testing.T) {
	baseURL := setupIntegrationServer(t)
	factory := pluginclient.NewPluginSignerFactory(pluginclient.NewPluginClient(baseURL, nil))
	signer, err := factory.CreateSigner(nil)
	require.NoError(t, err)

	_, err = signer.ValidateToken("invalid")
	require.Error(t, err)

	var pe *plugin.StatusError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, 401, pe.Code)
}

func TestIntegration_RemoteAccess_Initialize(t *testing.T) {
	baseURL := setupIntegrationServer(t)
	client := pluginclient.NewPluginRemoteAccessClient(pluginclient.NewPluginClient(baseURL, nil))

	err := client.Initialize(context.Background())
	require.NoError(t, err)
}

func TestIntegration_RemoteAccess_RegisterNodeAgent(t *testing.T) {
	baseURL := setupIntegrationServer(t)
	client := pluginclient.NewPluginRemoteAccessClient(pluginclient.NewPluginClient(baseURL, nil))

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
	client := pluginclient.NewPluginRemoteAccessClient(pluginclient.NewPluginClient(baseURL, nil))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{UID: types.UID("pod-uid-xyz")},
	}
	err := client.DeregisterNodeAgent(context.Background(), pod)
	require.NoError(t, err)
}

func TestIntegration_RemoteAccess_CreateSession(t *testing.T) {
	baseURL := setupIntegrationServer(t)
	client := pluginclient.NewPluginRemoteAccessClient(pluginclient.NewPluginClient(baseURL, nil))

	as := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionContext: map[string]string{"ssmDocumentName": "doc"},
		},
	}
	url, err := client.CreateSession(context.Background(), "test-ws", "test-ns", "pod-uid-123", as)
	require.NoError(t, err)
	assert.Equal(t, "vscode://connect?ws=test-ws", url)
}
