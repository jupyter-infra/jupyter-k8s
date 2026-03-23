/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockJWTHandler implements JWTHandler for testing.
type mockJWTHandler struct {
	signFn   func(ctx context.Context, req *pluginapi.SignRequest) (*pluginapi.SignResponse, error)
	verifyFn func(ctx context.Context, req *pluginapi.VerifyRequest) (*pluginapi.VerifyResponse, error)
}

func (m *mockJWTHandler) Sign(ctx context.Context, req *pluginapi.SignRequest) (*pluginapi.SignResponse, error) {
	return m.signFn(ctx, req)
}

func (m *mockJWTHandler) Verify(ctx context.Context, req *pluginapi.VerifyRequest) (*pluginapi.VerifyResponse, error) {
	return m.verifyFn(ctx, req)
}

// mockRemoteAccessHandler implements RemoteAccessHandler for testing.
type mockRemoteAccessHandler struct {
	initializeFn          func(ctx context.Context, req *pluginapi.InitializeRequest) (*pluginapi.InitializeResponse, error)
	registerNodeAgentFn   func(ctx context.Context, req *pluginapi.RegisterNodeAgentRequest) (*pluginapi.RegisterNodeAgentResponse, error)
	deregisterNodeAgentFn func(ctx context.Context, req *pluginapi.DeregisterNodeAgentRequest) (*pluginapi.DeregisterNodeAgentResponse, error)
	createSessionFn       func(ctx context.Context, req *pluginapi.CreateSessionRequest) (*pluginapi.CreateSessionResponse, error)
}

func (m *mockRemoteAccessHandler) Initialize(ctx context.Context, req *pluginapi.InitializeRequest) (*pluginapi.InitializeResponse, error) {
	return m.initializeFn(ctx, req)
}

func (m *mockRemoteAccessHandler) RegisterNodeAgent(ctx context.Context, req *pluginapi.RegisterNodeAgentRequest) (*pluginapi.RegisterNodeAgentResponse, error) {
	return m.registerNodeAgentFn(ctx, req)
}

func (m *mockRemoteAccessHandler) DeregisterNodeAgent(ctx context.Context, req *pluginapi.DeregisterNodeAgentRequest) (*pluginapi.DeregisterNodeAgentResponse, error) {
	return m.deregisterNodeAgentFn(ctx, req)
}

func (m *mockRemoteAccessHandler) CreateSession(ctx context.Context, req *pluginapi.CreateSessionRequest) (*pluginapi.CreateSessionResponse, error) {
	return m.createSessionFn(ctx, req)
}

func newMockServer(jwt *mockJWTHandler, ra *mockRemoteAccessHandler) *Server {
	return NewServer(ServerConfig{
		JWTHandler:          jwt,
		RemoteAccessHandler: ra,
	})
}

// --- Healthz ---

func TestHealthz(t *testing.T) {
	srv := NewServer(ServerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

// --- Middleware ---

func TestPanicRecovery(t *testing.T) {
	srv := NewServer(ServerConfig{})
	panicHandler := srv.withMiddleware(func(_ http.ResponseWriter, _ *http.Request) {
		panic("test panic")
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	w := httptest.NewRecorder()
	panicHandler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var errResp pluginapi.ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&errResp))
	assert.Equal(t, "internal server error", errResp.Error)
}

func TestRequestID_PassedThrough(t *testing.T) {
	srv := NewServer(ServerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(plugin.HeaderPluginRequestID, "caller-id-123")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, "caller-id-123", w.Header().Get(plugin.HeaderPluginRequestID))
}

func TestRequestID_GeneratedWhenMissing(t *testing.T) {
	srv := NewServer(ServerConfig{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	got := w.Header().Get(plugin.HeaderPluginRequestID)
	assert.NotEmpty(t, got)
	assert.Len(t, got, 16) // 8 bytes = 16 hex chars
}

func TestRequestID_AvailableInContext(t *testing.T) {
	srv := NewServer(ServerConfig{})
	var capturedID string
	handler := srv.withMiddleware(func(_ http.ResponseWriter, r *http.Request) {
		capturedID = PluginRequestID(r.Context())
	})

	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Header.Set(plugin.HeaderPluginRequestID, "ctx-test-456")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	assert.Equal(t, "ctx-test-456", capturedID)
}

// --- JWT Sign ---

func TestJWTSign_Success(t *testing.T) {
	jwtH := &mockJWTHandler{
		signFn: func(_ context.Context, req *pluginapi.SignRequest) (*pluginapi.SignResponse, error) {
			assert.Equal(t, "alice", req.User)
			assert.Equal(t, "bootstrap", req.TokenType)
			return &pluginapi.SignResponse{Token: "signed-token"}, nil
		},
	}
	srv := newMockServer(jwtH, &mockRemoteAccessHandler{})

	body, _ := json.Marshal(pluginapi.SignRequest{User: "alice", TokenType: "bootstrap"})
	req := httptest.NewRequest(http.MethodPost, pluginapi.RouteJWTSign.Path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp pluginapi.SignResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "signed-token", resp.Token)
}

func TestJWTSign_HandlerError(t *testing.T) {
	jwtH := &mockJWTHandler{
		signFn: func(_ context.Context, _ *pluginapi.SignRequest) (*pluginapi.SignResponse, error) {
			return nil, fmt.Errorf("KMS unavailable")
		},
	}
	srv := newMockServer(jwtH, &mockRemoteAccessHandler{})

	body, _ := json.Marshal(pluginapi.SignRequest{User: "alice"})
	req := httptest.NewRequest(http.MethodPost, pluginapi.RouteJWTSign.Path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var errResp pluginapi.ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&errResp))
	assert.Equal(t, "KMS unavailable", errResp.Error)
}

func TestJWTSign_StatusError(t *testing.T) {
	jwtH := &mockJWTHandler{
		signFn: func(_ context.Context, _ *pluginapi.SignRequest) (*pluginapi.SignResponse, error) {
			return nil, &plugin.StatusError{Code: http.StatusBadRequest, Message: "missing user field"}
		},
	}
	srv := newMockServer(jwtH, &mockRemoteAccessHandler{})

	body, _ := json.Marshal(pluginapi.SignRequest{})
	req := httptest.NewRequest(http.MethodPost, pluginapi.RouteJWTSign.Path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var errResp pluginapi.ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&errResp))
	assert.Equal(t, "missing user field", errResp.Error)
}

func TestJWTSign_InvalidBody(t *testing.T) {
	srv := newMockServer(&mockJWTHandler{}, &mockRemoteAccessHandler{})

	req := httptest.NewRequest(http.MethodPost, pluginapi.RouteJWTSign.Path, bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	var errResp pluginapi.ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&errResp))
	assert.Contains(t, errResp.Error, "invalid request body")
}

// --- JWT Verify ---

func TestJWTVerify_Success(t *testing.T) {
	jwtH := &mockJWTHandler{
		verifyFn: func(_ context.Context, req *pluginapi.VerifyRequest) (*pluginapi.VerifyResponse, error) {
			assert.Equal(t, "test-token", req.Token)
			return &pluginapi.VerifyResponse{
				Claims: &pluginapi.VerifyClaims{
					Subject:   "alice",
					TokenType: "bootstrap",
					ExpiresAt: 1700000000,
				},
			}, nil
		},
	}
	srv := newMockServer(jwtH, &mockRemoteAccessHandler{})

	body, _ := json.Marshal(pluginapi.VerifyRequest{Token: "test-token"})
	req := httptest.NewRequest(http.MethodPost, pluginapi.RouteJWTVerify.Path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp pluginapi.VerifyResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "alice", resp.Claims.Subject)
}

func TestJWTVerify_Unauthorized(t *testing.T) {
	jwtH := &mockJWTHandler{
		verifyFn: func(_ context.Context, _ *pluginapi.VerifyRequest) (*pluginapi.VerifyResponse, error) {
			return nil, &plugin.StatusError{Code: http.StatusUnauthorized, Message: "invalid or expired token"}
		},
	}
	srv := newMockServer(jwtH, &mockRemoteAccessHandler{})

	body, _ := json.Marshal(pluginapi.VerifyRequest{Token: "bad-token"})
	req := httptest.NewRequest(http.MethodPost, pluginapi.RouteJWTVerify.Path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- Remote Access ---

func TestRemoteAccessInit_Success(t *testing.T) {
	raH := &mockRemoteAccessHandler{
		initializeFn: func(_ context.Context, _ *pluginapi.InitializeRequest) (*pluginapi.InitializeResponse, error) {
			return &pluginapi.InitializeResponse{}, nil
		},
	}
	srv := newMockServer(&mockJWTHandler{}, raH)

	body, _ := json.Marshal(pluginapi.InitializeRequest{})
	req := httptest.NewRequest(http.MethodPost, pluginapi.RouteRemoteAccessInit.Path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRegisterNodeAgent_Success(t *testing.T) {
	raH := &mockRemoteAccessHandler{
		registerNodeAgentFn: func(_ context.Context, req *pluginapi.RegisterNodeAgentRequest) (*pluginapi.RegisterNodeAgentResponse, error) {
			assert.Equal(t, "pod-uid-123", req.PodUID)
			assert.Equal(t, "test-ws", req.WorkspaceName)
			return &pluginapi.RegisterNodeAgentResponse{
				ActivationID:   "act-1",
				ActivationCode: "code-1",
			}, nil
		},
	}
	srv := newMockServer(&mockJWTHandler{}, raH)

	body, _ := json.Marshal(pluginapi.RegisterNodeAgentRequest{
		PodUID:        "pod-uid-123",
		WorkspaceName: "test-ws",
		Namespace:     "test-ns",
	})
	req := httptest.NewRequest(http.MethodPost, pluginapi.RouteRegisterNodeAgent.Path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp pluginapi.RegisterNodeAgentResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "act-1", resp.ActivationID)
	assert.Equal(t, "code-1", resp.ActivationCode)
}

func TestDeregisterNodeAgent_Success(t *testing.T) {
	raH := &mockRemoteAccessHandler{
		deregisterNodeAgentFn: func(_ context.Context, req *pluginapi.DeregisterNodeAgentRequest) (*pluginapi.DeregisterNodeAgentResponse, error) {
			assert.Equal(t, "pod-uid-123", req.PodUID)
			return &pluginapi.DeregisterNodeAgentResponse{}, nil
		},
	}
	srv := newMockServer(&mockJWTHandler{}, raH)

	body, _ := json.Marshal(pluginapi.DeregisterNodeAgentRequest{PodUID: "pod-uid-123"})
	req := httptest.NewRequest(http.MethodPost, pluginapi.RouteDeregisterNodeAgent.Path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestCreateSession_Success(t *testing.T) {
	raH := &mockRemoteAccessHandler{
		createSessionFn: func(_ context.Context, req *pluginapi.CreateSessionRequest) (*pluginapi.CreateSessionResponse, error) {
			assert.Equal(t, "pod-uid-123", req.PodUID)
			return &pluginapi.CreateSessionResponse{
				ConnectionURL: "vscode://test",
			}, nil
		},
	}
	srv := newMockServer(&mockJWTHandler{}, raH)

	body, _ := json.Marshal(pluginapi.CreateSessionRequest{
		PodUID:        "pod-uid-123",
		WorkspaceName: "ws",
		Namespace:     "ns",
	})
	req := httptest.NewRequest(http.MethodPost, pluginapi.RouteCreateSession.Path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp pluginapi.CreateSessionResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "vscode://test", resp.ConnectionURL)
}

func TestCreateSession_HandlerError(t *testing.T) {
	raH := &mockRemoteAccessHandler{
		createSessionFn: func(_ context.Context, _ *pluginapi.CreateSessionRequest) (*pluginapi.CreateSessionResponse, error) {
			return nil, fmt.Errorf("session creation failed")
		},
	}
	srv := newMockServer(&mockJWTHandler{}, raH)

	body, _ := json.Marshal(pluginapi.CreateSessionRequest{PodUID: "pod-uid-123"})
	req := httptest.NewRequest(http.MethodPost, pluginapi.RouteCreateSession.Path, bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	var errResp pluginapi.ErrorResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&errResp))
	assert.Equal(t, "session creation failed", errResp.Error)
}
