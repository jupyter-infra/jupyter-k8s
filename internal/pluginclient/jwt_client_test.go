/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginSignerFactory_CreateSigner(t *testing.T) {
	factory := NewPluginSignerFactory(NewPluginClient("http://localhost:8080", nil))

	t.Run("with access strategy", func(t *testing.T) {
		as := &workspacev1alpha1.WorkspaceAccessStrategy{
			Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
				CreateConnectionContext: map[string]string{"kmsKeyId": "test-key"},
			},
		}
		signer, err := factory.CreateSigner(as)
		require.NoError(t, err)
		assert.NotNil(t, signer)
		ps := signer.(*PluginSigner)
		assert.Equal(t, "test-key", ps.connectionContext["kmsKeyId"])
	})

	t.Run("nil access strategy", func(t *testing.T) {
		signer, err := factory.CreateSigner(nil)
		require.NoError(t, err)
		assert.NotNil(t, signer)
	})
}

func TestPluginSigner_GenerateToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, pluginapi.RouteJWTSign.Path, r.URL.Path)

		var req pluginapi.SignRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "alice", req.User)
		assert.Equal(t, []string{"admin"}, req.Groups)
		assert.Equal(t, "uid-1", req.UID)
		assert.Equal(t, "/ws/ns/ws1", req.Path)
		assert.Equal(t, "example.com", req.Domain)
		assert.Equal(t, "bootstrap", req.TokenType)
		assert.Equal(t, "test-key", req.ConnectionContext["kmsKeyId"])

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.SignResponse{Token: "signed-token-123"})
	}))
	defer srv.Close()

	factory := NewPluginSignerFactory(NewPluginClient(srv.URL, nil))
	as := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionContext: map[string]string{"kmsKeyId": "test-key"},
		},
	}
	signer, err := factory.CreateSigner(as)
	require.NoError(t, err)

	token, err := signer.GenerateToken("alice", []string{"admin"}, "uid-1", nil, "/ws/ns/ws1", "example.com", "bootstrap")
	require.NoError(t, err)
	assert.Equal(t, "signed-token-123", token)
}

func TestPluginSigner_GenerateToken_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(pluginapi.ErrorResponse{Error: "KMS unavailable"})
	}))
	defer srv.Close()

	signer := &PluginSigner{client: NewPluginClient(srv.URL, nil)}
	_, err := signer.GenerateToken("alice", nil, "", nil, "/p", "d", "bootstrap")
	require.Error(t, err)

	var pe *plugin.StatusError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, http.StatusInternalServerError, pe.Code)
	assert.Equal(t, "KMS unavailable", pe.Message)
}

func TestPluginSigner_ValidateToken_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, pluginapi.RouteJWTVerify.Path, r.URL.Path)

		var req pluginapi.VerifyRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "test-token", req.Token)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.VerifyResponse{
			Claims: &pluginapi.VerifyClaims{
				Subject:   "alice",
				Groups:    []string{"admin"},
				UID:       "uid-1",
				Path:      "/ws/ns/ws1",
				Domain:    "example.com",
				TokenType: "bootstrap",
				ExpiresAt: 1700000000,
			},
		})
	}))
	defer srv.Close()

	signer := &PluginSigner{client: NewPluginClient(srv.URL, nil)}
	claims, err := signer.ValidateToken("test-token")
	require.NoError(t, err)
	assert.Equal(t, "alice", claims.User)
	assert.Equal(t, "alice", claims.Subject)
	assert.Equal(t, []string{"admin"}, claims.Groups)
	assert.Equal(t, "uid-1", claims.UID)
	assert.Equal(t, "/ws/ns/ws1", claims.Path)
	assert.Equal(t, "example.com", claims.Domain)
	assert.Equal(t, "bootstrap", claims.TokenType)
	assert.Equal(t, int64(1700000000), claims.ExpiresAt.Unix())
}

func TestPluginSigner_ValidateToken_Unauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(pluginapi.ErrorResponse{Error: "invalid or expired token"})
	}))
	defer srv.Close()

	signer := &PluginSigner{client: NewPluginClient(srv.URL, nil)}
	_, err := signer.ValidateToken("bad-token")
	require.Error(t, err)

	var pe *plugin.StatusError
	require.ErrorAs(t, err, &pe)
	assert.Equal(t, http.StatusUnauthorized, pe.Code)
	assert.Equal(t, "invalid or expired token", pe.Message)
}

func TestPluginSigner_ConnectionRefused(t *testing.T) {
	client := NewPluginClient("http://127.0.0.1:1", nil)
	client.retryCount = 0 // no retries — fail fast for this test
	signer := &PluginSigner{client: client}

	_, err := signer.GenerateToken("alice", nil, "", nil, "/p", "d", "bootstrap")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")

	_, err = signer.ValidateToken("token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}
