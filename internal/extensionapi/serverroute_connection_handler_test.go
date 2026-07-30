/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// --- validateConnection tests ---

func TestValidateConnection(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	tests := []struct {
		name               string
		workspace          *workspacev1alpha1.Workspace
		accessStrategy     *workspacev1alpha1.WorkspaceAccessStrategy
		expectedStatusCode int
		expectedError      string
	}{
		{
			name: "workspace not available",
			workspace: &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: namespaceDefault},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: "as"},
				},
				Status: workspacev1alpha1.WorkspaceStatus{
					Conditions: []metav1.Condition{{Type: conditionTypeAvailable, Status: metav1.ConditionFalse}},
				},
			},
			accessStrategy: &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{Name: "as", Namespace: namespaceDefault},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					CreateConnectionHandler: handlerK8sNative,
					BearerAuthURLTemplate:   "https://example.com",
				},
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      "workspace is not available",
		},
		{
			name: "validation passes",
			workspace: &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: namespaceDefault},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: "as"},
				},
				Status: workspacev1alpha1.WorkspaceStatus{
					Conditions: []metav1.Condition{{Type: conditionTypeAvailable, Status: metav1.ConditionTrue}},
				},
			},
			accessStrategy: &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{Name: "as", Namespace: namespaceDefault},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					CreateConnectionHandler: handlerK8sNative,
					BearerAuthURLTemplate:   "https://example.com",
					CreateConnectionContext: map[string]string{
						"staticKey": "staticValue",
					},
				},
			},
			expectedStatusCode: 0,
			expectedError:      "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			if tt.workspace != nil {
				objects = append(objects, tt.workspace)
			}
			if tt.accessStrategy != nil {
				objects = append(objects, tt.accessStrategy)
			}

			fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()

			server := &ExtensionServer{
				k8sClient: fakeClient,
			}

			logger := ctrl.Log.WithName("test")
			_, resolvedCtx, statusCode, err := server.validateConnection(tt.workspace, logger)

			if statusCode != tt.expectedStatusCode {
				t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, statusCode)
			}

			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				// Check that context was returned when validation passes
				if resolvedCtx == nil && tt.accessStrategy != nil && len(tt.accessStrategy.Spec.CreateConnectionContext) > 0 {
					t.Error("expected resolved context to be returned")
				}
			} else {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("expected error containing %q, got %q", tt.expectedError, err.Error())
				}
			}
		})
	}
}

// --- HandleConnectionCreate tests ---

func TestHandleConnectionCreateValidation(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{},
	}

	tests := []struct {
		name           string
		method         string
		path           string
		body           interface{}
		expectedStatus int
	}{
		{
			name:           "wrong method",
			method:         "GET",
			path:           connectionsPath,
			body:           nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid path",
			method:         http.MethodPost,
			path:           "/invalid/path",
			body:           nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid JSON",
			method:         http.MethodPost,
			path:           connectionsPath,
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "missing workspace name",
			method: http.MethodPost,
			path:   connectionsPath,
			body: connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceConnectionType: connectionTypeVSCodeRemote,
				},
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyBytes []byte
			if tt.body != nil {
				if str, ok := tt.body.(string); ok {
					bodyBytes = []byte(str)
				} else {
					bodyBytes, _ = json.Marshal(tt.body)
				}
			}

			req := httptest.NewRequest(tt.method, tt.path, bytes.NewReader(bodyBytes))
			w := httptest.NewRecorder()

			server.HandleConnectionCreate(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

func TestHandleConnectionCreateReadBodyError(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{},
	}

	req := httptest.NewRequest("POST", connectionsPath, nil)
	req.Body = &badReader{}
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for body read error, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleConnectionCreateInvalidConnectionType(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{},
	}

	reqBody := connectionv1alpha1.WorkspaceConnectionRequest{
		Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
			WorkspaceName:           testWorkspace,
			WorkspaceConnectionType: invalidConnectionType,
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", connectionsPath, bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for invalid connection type, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleConnectionCreateAuthorizationError(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{},
	}

	reqBody := connectionv1alpha1.WorkspaceConnectionRequest{
		Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
			WorkspaceName:           testWorkspace,
			WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeWebUI,
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", connectionsPath, bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, httpReq)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d for authorization error, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestHandleConnectionCreateInvalidMethod(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{},
	}

	httpReq := httptest.NewRequest("GET", connectionsPath, nil)
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for invalid method, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestHandleConnectionCreateWebUIPath(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	logger := ctrl.Log.WithName("test")

	server := &ExtensionServer{
		config:    &ExtensionConfig{},
		k8sClient: ctrlclient.NewClientBuilder().WithScheme(scheme).Build(),
		logger:    &logger,
	}

	reqBody := connectionv1alpha1.WorkspaceConnectionRequest{
		Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
			WorkspaceName:           "test",
			WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeWebUI,
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", connectionsPath, bytes.NewReader(bodyBytes))
	httpReq.Header.Set("X-User", "test")
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, httpReq)
	// Covers WebUI path regardless of final status
}

func TestHandleConnectionCreateWithWorkspace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = workspacev1alpha1.AddToScheme(scheme)

	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspace, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessType: AccessTypePublic,
		},
	}

	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace).Build()
	logger := ctrl.Log.WithName("test")

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		k8sClient:     fakeClient,
		logger:        &logger,
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: testToken}},
	}

	reqBody := connectionv1alpha1.WorkspaceConnectionRequest{
		Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
			WorkspaceName:           testWorkspace,
			WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeWebUI,
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", connectionsPath, bytes.NewReader(bodyBytes))
	httpReq.Header.Set("X-User", "test-user")
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, httpReq)
	// Should pass authorization and reach validation
}

// --- HandleConnectionCreate full success and terminal-status paths ---

func TestHandleConnectionCreate_WebUISuccess(t *testing.T) {
	ws, strategy := availableWorkspaceWithStrategy(workspacev1alpha1.WorkspaceAccessStrategySpec{
		BearerAuthURLTemplate: "https://test.com/workspaces/{{.Workspace.Namespace}}/{{.Workspace.Name}}/bearer-auth",
	})
	server := newConnectionServer(t, ws, strategy)

	w := postConnection(t, server, connectionv1alpha1.ConnectionTypeWebUI)

	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	var resp connectionv1alpha1.WorkspaceConnectionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, connectionv1alpha1.ConnectionTypeWebUI, resp.Status.WorkspaceConnectionType)
	assert.Contains(t, resp.Status.WorkspaceConnectionURL, "token="+testToken)
}

func TestHandleConnectionCreate_WebSocketSuccess(t *testing.T) {
	ws, strategy := availableWorkspaceWithStrategy(workspacev1alpha1.WorkspaceAccessStrategySpec{
		BearerAuthURLTemplate: "https://test.com/workspaces/ns/ws/bearer-auth",
	})
	server := newConnectionServer(t, ws, strategy)

	w := postConnection(t, server, connectionv1alpha1.ConnectionTypeWebSocket)

	require.Equal(t, http.StatusCreated, w.Code, "body: %s", w.Body.String())
	var resp connectionv1alpha1.WorkspaceConnectionResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, connectionv1alpha1.ConnectionTypeWebSocket, resp.Status.WorkspaceConnectionType)
	assert.Contains(t, resp.Status.WorkspaceConnectionURL, "wss://")
}

func TestHandleConnectionCreate_WebUINotEnabled(t *testing.T) {
	// Available workspace + strategy, but no BearerAuthURLTemplate → web UI disabled.
	ws, strategy := availableWorkspaceWithStrategy(workspacev1alpha1.WorkspaceAccessStrategySpec{})
	server := newConnectionServer(t, ws, strategy)

	w := postConnection(t, server, connectionv1alpha1.ConnectionTypeWebUI)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "web browser access is not enabled")
}

func TestHandleConnectionCreate_WorkspaceNotAvailable(t *testing.T) {
	// Public workspace referencing a strategy, but without an Available condition.
	strategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: strategyName, Namespace: namespaceDefault},
		Spec:       workspacev1alpha1.WorkspaceAccessStrategySpec{BearerAuthURLTemplate: bearerAuthTestURL},
	}
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: testWorkspace, Namespace: namespaceDefault},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessType:     AccessTypePublic,
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: strategyName, Namespace: namespaceDefault},
		},
	}
	server := newConnectionServer(t, ws, strategy)

	w := postConnection(t, server, connectionv1alpha1.ConnectionTypeWebUI)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "not available")
}

func TestHandleConnectionCreate_SignerFailureReturns500(t *testing.T) {
	ws, strategy := availableWorkspaceWithStrategy(workspacev1alpha1.WorkspaceAccessStrategySpec{
		BearerAuthURLTemplate: bearerAuthTestURL,
	})
	server := newConnectionServer(t, ws, strategy)
	// Force the signer factory to fail so URL generation errors out.
	server.signerFactory = &mockSignerFactory{err: errors.New("signer boom")}

	w := postConnection(t, server, connectionv1alpha1.ConnectionTypeWebUI)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestHandleConnectionCreate_UnknownRemoteTypeNotConfigured(t *testing.T) {
	// A valid *-remote type that the access strategy has no handler for → 400.
	ws, strategy := availableWorkspaceWithStrategy(workspacev1alpha1.WorkspaceAccessStrategySpec{
		BearerAuthURLTemplate: bearerAuthTestURL,
	})
	server := newConnectionServer(t, ws, strategy)

	w := postConnection(t, server, "windsurf-remote")

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "is not configured for this workspace")
}
