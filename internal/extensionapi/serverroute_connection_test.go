/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-logr/logr"
	pluginapi "github.com/jupyter-infra/jupyter-k8s-plugin/api"
	"github.com/jupyter-infra/jupyter-k8s-plugin/pluginclient"
	connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/authentication/user"
	"k8s.io/apiserver/pkg/endpoints/request"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testUser = "test-user"

// mockSignerFactory for testing
type mockSignerFactory struct {
	signer *mockSigner
}

func (m *mockSignerFactory) CreateSigner(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (jwt.Signer, error) {
	return m.signer, nil
}

// mockSigner for testing
type mockSigner struct {
	token string
	// captured args from last GenerateToken call
	lastGroups []string
	lastExtra  map[string][]string
}

func (m *mockSigner) GenerateToken(username string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string, skipRefresh bool) (string, error) {
	m.lastGroups = groups
	m.lastExtra = extra
	return m.token, nil
}

func (m *mockSigner) GenerateRefreshToken(claims *jwt.Claims) (string, error) {
	return m.token, nil
}

func (m *mockSigner) ValidateToken(tokenString string) (*jwt.Claims, error) {
	return nil, nil
}

// mockTokenValidator for testing
type mockTokenValidator struct {
	claims *jwt.Claims
	err    error
}

func (m *mockTokenValidator) ValidateToken(tokenString string) (*jwt.Claims, error) {
	return m.claims, m.err
}

// badReader is a helper that always returns an error when reading
type badReader struct{}

func (e *badReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("read error")
}

func (e *badReader) Close() error {
	return nil
}

// --- generateBearerTokenURL tests ---

func TestGenerateBearerTokenURL(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: "test-strategy"},
		},
	}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-strategy", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "https://test.com/workspaces/{{.Workspace.Namespace}}/{{.Workspace.Name}}/bearer-auth",
		},
	}

	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, accessStrategy).Build()

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: "test-token"}},
		k8sClient:     fakeClient,
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	url, err := server.generateBearerTokenURL(req, workspace, accessStrategy)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	expected := "https://test.com/workspaces/default/test-workspace/bearer-auth?token=test-token"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestGenerateBearerTokenURL_SubdomainRouting(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "myworkspace", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: "subdomain-strategy"},
		},
	}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: "subdomain-strategy", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "https://{{.Workspace.Name}}-{{b32encode .Workspace.Namespace}}.example.com/bearer-auth",
		},
	}

	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, accessStrategy).Build()

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: "test-token"}},
		k8sClient:     fakeClient,
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	url, err := server.generateBearerTokenURL(req, workspace, accessStrategy)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	expected := "https://myworkspace-mrswmylvnr2a.example.com/bearer-auth?token=test-token"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestGenerateBearerTokenURL_PassesGroupsAndExtra(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: "test-strategy"},
		},
	}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-strategy", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "https://test.com/bearer-auth",
		},
	}

	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, accessStrategy).Build()

	signer := &mockSigner{token: "test-token"}
	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: signer},
		k8sClient:     fakeClient,
	}

	req := httptest.NewRequest("POST", "/test", nil)
	userInfo := &user.DefaultInfo{
		Name:   testUser,
		Groups: []string{"cluster-workspace-admin", "system:authenticated"},
		Extra: map[string][]string{
			"arn":          {"arn:aws:sts::123456:assumed-role/Admin/session"},
			"canonicalarn": {"arn:aws:iam::123456:role/Admin"},
		},
	}
	ctx := request.WithUser(req.Context(), userInfo)
	req = req.WithContext(ctx)

	_, err := server.generateBearerTokenURL(req, workspace, accessStrategy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify groups were passed to the signer
	if len(signer.lastGroups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %v", len(signer.lastGroups), signer.lastGroups)
	}
	if signer.lastGroups[0] != "cluster-workspace-admin" || signer.lastGroups[1] != "system:authenticated" {
		t.Errorf("unexpected groups: %v", signer.lastGroups)
	}

	// Verify extra was passed to the signer
	if len(signer.lastExtra) != 2 {
		t.Fatalf("expected 2 extra keys, got %d: %v", len(signer.lastExtra), signer.lastExtra)
	}
	if signer.lastExtra["arn"][0] != "arn:aws:sts::123456:assumed-role/Admin/session" {
		t.Errorf("unexpected extra arn: %v", signer.lastExtra["arn"])
	}
	if signer.lastExtra["canonicalarn"][0] != "arn:aws:iam::123456:role/Admin" {
		t.Errorf("unexpected extra canonicalarn: %v", signer.lastExtra["canonicalarn"])
	}
}

func TestGenerateBearerTokenURL_NoContextFallsBackToEmptyGroupsAndExtra(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: "test-strategy"},
		},
	}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: "test-strategy", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "https://test.com/bearer-auth",
		},
	}

	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, accessStrategy).Build()

	signer := &mockSigner{token: "test-token"}
	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: signer},
		k8sClient:     fakeClient,
	}

	// Request with header-based user, no Kubernetes context
	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	_, err := server.generateBearerTokenURL(req, workspace, accessStrategy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Groups and extra should be nil when no k8s context
	if signer.lastGroups != nil {
		t.Errorf("expected nil groups, got %v", signer.lastGroups)
	}
	if signer.lastExtra != nil {
		t.Errorf("expected nil extra, got %v", signer.lastExtra)
	}
}

func TestGenerateBearerTokenURL_NoAccessStrategy(t *testing.T) {
	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: "test-token"}},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	_, err := server.generateBearerTokenURL(req, &workspacev1alpha1.Workspace{}, nil)

	if err == nil {
		t.Error("expected error for missing AccessStrategy, got nil")
	}
	if !strings.Contains(err.Error(), "no AccessStrategy configured") {
		t.Errorf("expected AccessStrategy error, got: %v", err)
	}
}

func TestGenerateBearerTokenURL_MissingTemplate(t *testing.T) {
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "",
		},
	}

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: "test-token"}},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	_, err := server.generateBearerTokenURL(req, &workspacev1alpha1.Workspace{}, accessStrategy)

	if err == nil {
		t.Error("expected error for missing BearerAuthURLTemplate, got nil")
	}
	if !strings.Contains(err.Error(), "BearerAuthURLTemplate not configured") {
		t.Errorf("expected template error, got: %v", err)
	}
}

// --- generatePluginConnectionURL tests ---

func TestGeneratePluginConnectionURL_Success(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default"},
	}

	// Create httptest server simulating the plugin
	pluginSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req pluginapi.CreateSessionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.PodUID != "test-uid-123" {
			t.Errorf("expected podUID test-uid-123, got %s", req.PodUID)
		}
		if req.ConnectionContext["ssmDocumentName"] != "test-document" {
			t.Errorf("expected ssmDocumentName test-document, got %s", req.ConnectionContext["ssmDocumentName"])
		}
		if req.ConnectionType != "vscode-remote" {
			t.Errorf("expected connectionType vscode-remote, got %s", req.ConnectionType)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pluginapi.CreateSessionResponse{
			ConnectionURL: "vscode://vscode-remote/ssh-remote+test-workspace/home/user",
		})
	}))
	defer pluginSrv.Close()

	server := &ExtensionServer{
		config: &ExtensionConfig{},
		pluginClients: map[string]*pluginclient.PluginClient{
			"aws": pluginclient.NewPluginClient(pluginSrv.URL, logr.Discard()),
		},
	}

	resolvedContext := map[string]string{
		"podUid":          "test-uid-123",
		"ssmDocumentName": "test-document",
	}

	req := httptest.NewRequest("POST", "/test", nil)
	url, err := server.generatePluginConnectionURL(req, workspace, "aws", "createSession", "vscode-remote", resolvedContext, "default")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if url != "vscode://vscode-remote/ssh-remote+test-workspace/home/user" {
		t.Errorf("unexpected URL: %s", url)
	}
}

func TestGeneratePluginConnectionURL_PluginError(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default"},
	}

	pluginSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "SSM creation failed"})
	}))
	defer pluginSrv.Close()

	server := &ExtensionServer{
		config: &ExtensionConfig{},
		pluginClients: map[string]*pluginclient.PluginClient{
			"aws": pluginclient.NewPluginClient(pluginSrv.URL, logr.Discard()),
		},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	_, err := server.generatePluginConnectionURL(req, workspace, "aws", "createSession", "vscode-remote", map[string]string{}, "default")

	if err == nil {
		t.Error("expected error from plugin client")
	}
}

func TestGeneratePluginConnectionURL_NoPlugin(t *testing.T) {
	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		pluginClients: map[string]*pluginclient.PluginClient{},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	_, err := server.generatePluginConnectionURL(req, &workspacev1alpha1.Workspace{}, "aws", "createSession", "vscode-remote", map[string]string{}, "default")

	if err == nil {
		t.Error("expected error for missing plugin")
	}
	if !strings.Contains(err.Error(), "no plugin endpoint configured") {
		t.Errorf("expected plugin error, got: %v", err)
	}
}

func TestGeneratePluginConnectionURL_UnsupportedAction(t *testing.T) {
	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		pluginClients: map[string]*pluginclient.PluginClient{"aws": pluginclient.NewPluginClient("http://localhost:8080", logr.Discard())},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	_, err := server.generatePluginConnectionURL(req, &workspacev1alpha1.Workspace{}, "aws", "unknownAction", "vscode-remote", map[string]string{}, "default")

	if err == nil {
		t.Error("expected error for unsupported action")
	}
	if !strings.Contains(err.Error(), "unsupported plugin action") {
		t.Errorf("expected unsupported action error, got: %v", err)
	}
}

// --- resolveConnectionHandler tests ---

func TestResolveConnectionHandler(t *testing.T) {
	tests := []struct {
		name           string
		accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
		connectionType string
		expectedPlugin string
		expectedAction string
		expectedFound  bool
	}{
		{
			name:           "nil access strategy",
			accessStrategy: nil,
			connectionType: "web-ui",
			expectedFound:  false,
		},
		{
			name: "found in handler map",
			accessStrategy: &workspacev1alpha1.WorkspaceAccessStrategy{
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					CreateConnectionHandlerMap: map[string]string{
						"vscode-remote": "aws:createSession",
					},
					CreateConnectionHandler: "k8s-native",
				},
			},
			connectionType: "vscode-remote",
			expectedPlugin: "aws",
			expectedAction: "createSession",
			expectedFound:  true,
		},
		{
			name: "falls back to default handler",
			accessStrategy: &workspacev1alpha1.WorkspaceAccessStrategy{
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					CreateConnectionHandlerMap: map[string]string{
						"vscode-remote": "aws:createSession",
					},
					CreateConnectionHandler: "k8s-native",
				},
			},
			connectionType: "web-ui",
			expectedPlugin: "k8s-native",
			expectedAction: "",
			expectedFound:  true,
		},
		{
			name: "no handler configured",
			accessStrategy: &workspacev1alpha1.WorkspaceAccessStrategy{
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{},
			},
			connectionType: "web-ui",
			expectedFound:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin, action, found := resolveConnectionHandler(tt.accessStrategy, tt.connectionType)
			if found != tt.expectedFound {
				t.Errorf("expected found=%v, got %v", tt.expectedFound, found)
			}
			if plugin != tt.expectedPlugin {
				t.Errorf("expected plugin=%q, got %q", tt.expectedPlugin, plugin)
			}
			if action != tt.expectedAction {
				t.Errorf("expected action=%q, got %q", tt.expectedAction, action)
			}
		})
	}
}

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
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "default"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: "as"},
				},
				Status: workspacev1alpha1.WorkspaceStatus{
					Conditions: []metav1.Condition{{Type: "Available", Status: metav1.ConditionFalse}},
				},
			},
			accessStrategy: &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{Name: "as", Namespace: "default"},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					CreateConnectionHandler: "k8s-native",
					BearerAuthURLTemplate:   "https://example.com",
				},
			},
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      "workspace is not available",
		},
		{
			name: "validation passes",
			workspace: &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "default"},
				Spec: workspacev1alpha1.WorkspaceSpec{
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{Name: "as"},
				},
				Status: workspacev1alpha1.WorkspaceStatus{
					Conditions: []metav1.Condition{{Type: "Available", Status: metav1.ConditionTrue}},
				},
			},
			accessStrategy: &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{Name: "as", Namespace: "default"},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					CreateConnectionHandler: "k8s-native",
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
			path:           "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections",
			body:           nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid path",
			method:         "POST",
			path:           "/invalid/path",
			body:           nil,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid JSON",
			method:         "POST",
			path:           "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "missing workspace name",
			method: "POST",
			path:   "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections",
			body: connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceConnectionType: "vscode-remote",
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

	req := httptest.NewRequest("POST", "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections", nil)
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
			WorkspaceName:           "test-workspace",
			WorkspaceConnectionType: "invalid-type",
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections", bytes.NewReader(bodyBytes))
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
			WorkspaceName:           "test-workspace",
			WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeWebUI,
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections", bytes.NewReader(bodyBytes))
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

	httpReq := httptest.NewRequest("GET", "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections", nil)
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
	httpReq := httptest.NewRequest("POST", "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections", bytes.NewReader(bodyBytes))
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
		ObjectMeta: metav1.ObjectMeta{Name: "test-workspace", Namespace: "default"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessType: "Public",
		},
	}

	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace).Build()
	logger := ctrl.Log.WithName("test")

	server := &ExtensionServer{
		config:        &ExtensionConfig{},
		k8sClient:     fakeClient,
		logger:        &logger,
		signerFactory: &mockSignerFactory{signer: &mockSigner{token: "test-token"}},
	}

	reqBody := connectionv1alpha1.WorkspaceConnectionRequest{
		Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
			WorkspaceName:           "test-workspace",
			WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeWebUI,
		},
	}

	bodyBytes, _ := json.Marshal(reqBody)
	httpReq := httptest.NewRequest("POST", "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections", bytes.NewReader(bodyBytes))
	httpReq.Header.Set("X-User", "test-user")
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, httpReq)
	// Should pass authorization and reach validation
}

// --- Other helper tests ---

func TestValidateWorkspaceConnectionRequest(t *testing.T) {
	tests := []struct {
		name        string
		req         *connectionv1alpha1.WorkspaceConnectionRequest
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid vscode request",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           "test-workspace",
					WorkspaceConnectionType: "vscode-remote",
				},
			},
		},
		{
			name: "valid web-ui request",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           "test-workspace",
					WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeWebUI,
				},
			},
		},
		{
			name: "valid kiro-remote request",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           "test-workspace",
					WorkspaceConnectionType: "kiro-remote",
				},
			},
		},
		{
			name: "valid cursor-remote request",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           "test-workspace",
					WorkspaceConnectionType: "cursor-remote",
				},
			},
		},
		{
			name: "valid unknown *-remote request",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           "test-workspace",
					WorkspaceConnectionType: "windsurf-remote",
				},
			},
		},
		{
			name: "missing workspace name",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceConnectionType: "vscode-remote",
				},
			},
			expectError: true,
			errorMsg:    "workspaceName is required",
		},
		{
			name: "missing connection type",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName: "test-workspace",
				},
			},
			expectError: true,
			errorMsg:    "workspaceConnectionType is required",
		},
		{
			name: "invalid connection type - no -remote suffix",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           "test-workspace",
					WorkspaceConnectionType: "invalid-type",
				},
			},
			expectError: true,
			errorMsg:    "invalid workspaceConnectionType: 'invalid-type'. Must be 'web-ui' or follow the '{ide}-remote' pattern (e.g. 'vscode-remote', 'kiro-remote', 'cursor-remote')",
		},
		{
			name: "invalid connection type - bare remote",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           "test-workspace",
					WorkspaceConnectionType: "-remote",
				},
			},
			expectError: true,
			errorMsg:    "invalid workspaceConnectionType: '-remote'. Must be 'web-ui' or follow the '{ide}-remote' pattern (e.g. 'vscode-remote', 'kiro-remote', 'cursor-remote')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWorkspaceConnectionRequest(tt.req)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				} else if err.Error() != tt.errorMsg {
					t.Errorf("expected error %q, got %q", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestCheckWorkspaceAuthorizationMissingUser(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{},
	}

	req := httptest.NewRequest("POST", "/test", nil)

	_, result, err := server.checkWorkspaceAuthorization(req, "test-workspace", "default")

	if err == nil {
		t.Error("expected error when user headers are missing")
	}
	if result != nil {
		t.Error("expected nil result when user headers are missing")
	}
	expectedMsg := "user not found in request headers"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}

func TestGetUserFromHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	username := GetUser(req)
	if username != testUser {
		t.Errorf("expected %s, got %s", testUser, username)
	}
}

func TestHasWebUIEnabled(t *testing.T) {
	tests := []struct {
		name           string
		accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
		expected       bool
	}{
		{
			name:           "nil access strategy",
			accessStrategy: nil,
			expected:       false,
		},
		{
			name: "empty BearerAuthURLTemplate",
			accessStrategy: &workspacev1alpha1.WorkspaceAccessStrategy{
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					BearerAuthURLTemplate: "",
				},
			},
			expected: false,
		},
		{
			name: "BearerAuthURLTemplate configured",
			accessStrategy: &workspacev1alpha1.WorkspaceAccessStrategy{
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					BearerAuthURLTemplate: "https://example.com/bearer-auth",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasWebUIEnabled(tt.accessStrategy)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsWorkspaceAvailable(t *testing.T) {
	tests := []struct {
		name      string
		workspace *workspacev1alpha1.Workspace
		expected  bool
	}{
		{
			name: "no conditions",
			workspace: &workspacev1alpha1.Workspace{
				Status: workspacev1alpha1.WorkspaceStatus{
					Conditions: []metav1.Condition{},
				},
			},
			expected: false,
		},
		{
			name: "Available condition is True",
			workspace: &workspacev1alpha1.Workspace{
				Status: workspacev1alpha1.WorkspaceStatus{
					Conditions: []metav1.Condition{
						{Type: "Available", Status: metav1.ConditionTrue},
					},
				},
			},
			expected: true,
		},
		{
			name: "Available condition is False",
			workspace: &workspacev1alpha1.Workspace{
				Status: workspacev1alpha1.WorkspaceStatus{
					Conditions: []metav1.Condition{
						{Type: "Available", Status: metav1.ConditionFalse},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWorkspaceAvailable(tt.workspace)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsRemoteConnectionType(t *testing.T) {
	tests := []struct {
		connectionType string
		expected       bool
	}{
		{"vscode-remote", true},
		{"kiro-remote", true},
		{"cursor-remote", true},
		{"windsurf-remote", true},
		{"my-vscode-remote", true},
		{"web-ui", false},
		{"invalid-type", false},
		{"-remote", false},
		{"remote", false},
		{"", false},
		{"my--vscode-remote", false},
		{"vscode-remote-ssh", false},
		{"-vscode-remote", false},
		{"vscode remote", false},
		{"vscode_remote", false},
	}

	for _, tt := range tests {
		t.Run(tt.connectionType, func(t *testing.T) {
			result := isRemoteConnectionType(tt.connectionType)
			if result != tt.expected {
				t.Errorf("isRemoteConnectionType(%q) = %v, want %v", tt.connectionType, result, tt.expected)
			}
		})
	}
}
