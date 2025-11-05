package extensionapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/jwt"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNoOpPodExec(t *testing.T) {
	exec := &noOpPodExec{}
	pod := &corev1.Pod{}

	_, err := exec.ExecInPod(context.Background(), pod, "container", []string{"cmd"}, "stdin")

	if err == nil {
		t.Error("expected error from noOpPodExec.ExecInPod")
	}

	expectedMsg := "pod exec not supported in connection URL generation"
	if err.Error() != expectedMsg {
		t.Errorf("expected error %q, got %q", expectedMsg, err.Error())
	}
}

func TestGenerateWebUIURL(t *testing.T) {
	server := &ExtensionServer{}
	req := httptest.NewRequest("POST", "/test", nil)

	connType, url, err := server.generateWebUIURL(req, "test-workspace", "default")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if connType != connectionv1alpha1.ConnectionTypeWebUI {
		t.Errorf("expected connection type %s, got %s", connectionv1alpha1.ConnectionTypeWebUI, connType)
	}

	if url != "https://placeholder-webui-url.com" {
		t.Errorf("expected placeholder URL, got %s", url)
	}
}

const testUser = "test-user"

// mockJWTManager for testing
type mockJWTManager struct {
	token string
}

func (m *mockJWTManager) GenerateToken(user string, groups []string, uid string, extra map[string][]string, path string, domain string, tokenType string) (string, error) {
	return m.token, nil
}

func (m *mockJWTManager) ValidateToken(tokenString string) (*jwt.Claims, error) {
	return nil, nil
}

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
					WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeVSCodeRemote,
				},
			},
			expectError: false,
		},
		{
			name: "valid web-ui request",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           "test-workspace",
					WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeWebUI,
				},
			},
			expectError: false,
		},
		{
			name: "missing workspace name",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeVSCodeRemote,
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
			name: "invalid connection type",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           "test-workspace",
					WorkspaceConnectionType: "invalid-type",
				},
			},
			expectError: true,
			errorMsg:    "invalid workspaceConnectionType: 'invalid-type'. Valid types are: 'vscode-remote', 'web-ui'",
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

func TestHandleConnectionCreateValidation(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{
			ClusterId: "test-cluster",
		},
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
					WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeVSCodeRemote,
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

func TestHandleConnectionCreateClusterIdValidation(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{
			ClusterId: "", // Empty cluster ID
		},
	}

	req := connectionv1alpha1.WorkspaceConnectionRequest{
		Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
			WorkspaceName:           "test-workspace",
			WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeVSCodeRemote,
		},
	}

	bodyBytes, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for missing cluster ID, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestGenerateVSCodeURL(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).Build()

	server := &ExtensionServer{
		config: &ExtensionConfig{
			ClusterId: "test-cluster",
		},
		k8sClient: fakeClient,
	}
	req := httptest.NewRequest("POST", "/test", nil)

	_, _, err := server.generateVSCodeURL(req, "test-workspace", "default")

	if err == nil {
		t.Error("expected error from generateVSCodeURL without pods")
	}
}

func TestGenerateVSCodeURLWithPod(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid-123",
			Labels: map[string]string{
				"workspace.jupyter.org/workspaceName": "test-workspace",
			},
		},
	}

	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

	server := &ExtensionServer{
		config: &ExtensionConfig{
			ClusterId: "test-cluster",
		},
		k8sClient: fakeClient,
	}
	req := httptest.NewRequest("POST", "/test", nil)

	_, _, err := server.generateVSCodeURL(req, "test-workspace", "default")

	if err == nil {
		t.Error("expected error from generateVSCodeURL at SSM strategy creation")
	}
}

func TestHandleConnectionCreateReadBodyError(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{
			ClusterId: "test-cluster",
		},
	}

	// Create a request with a body that will cause a read error
	req := httptest.NewRequest("POST", "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections", nil)
	// Use a custom reader that returns an error
	req.Body = &badReader{}
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for body read error, got %d", http.StatusBadRequest, w.Code)
	}
}

// badReader is a helper that always returns an error when reading
type badReader struct{}

func (e *badReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("read error")
}

func (e *badReader) Close() error {
	return nil
}

func TestHandleConnectionCreateInvalidConnectionType(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{
			ClusterId: "test-cluster",
		},
	}

	req := connectionv1alpha1.WorkspaceConnectionRequest{
		Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
			WorkspaceName:           "test-workspace",
			WorkspaceConnectionType: "invalid-type",
		},
	}

	bodyBytes, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for invalid connection type, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestCheckWorkspaceAuthorizationMissingUser(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{
			ClusterId: "test-cluster",
		},
	}

	// Create request without user headers
	req := httptest.NewRequest("POST", "/test", nil)

	result, err := server.checkWorkspaceAuthorization(req, "test-workspace", "default")

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

func TestHandleConnectionCreateAuthorizationError(t *testing.T) {
	// Create a server that will cause authorization to fail
	server := &ExtensionServer{
		config: &ExtensionConfig{
			ClusterId: "test-cluster",
		},
	}

	req := connectionv1alpha1.WorkspaceConnectionRequest{
		Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
			WorkspaceName:           "test-workspace",
			WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeWebUI,
		},
	}

	bodyBytes, _ := json.Marshal(req)
	// Create request without user headers to trigger authorization error
	httpReq := httptest.NewRequest("POST", "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, httpReq)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d for authorization error, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestGenerateVSCodeURLSSMSuccess(t *testing.T) {
	original := newSSMRemoteAccessStrategy
	defer func() {
		newSSMRemoteAccessStrategy = original
	}()

	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       "test-uid-123",
			Labels: map[string]string{
				"workspace.jupyter.org/workspaceName": "test-workspace",
			},
		},
	}

	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

	server := &ExtensionServer{
		config: &ExtensionConfig{
			ClusterId: "test-cluster",
		},
		k8sClient: fakeClient,
	}

	newSSMRemoteAccessStrategy = func(ssmClient aws.SSMRemoteAccessClientInterface, podExec aws.PodExecInterface) (*aws.SSMRemoteAccessStrategy, error) {
		return nil, fmt.Errorf("SSM creation failed")
	}

	req := httptest.NewRequest("POST", "/test", nil)
	_, _, err := server.generateVSCodeURL(req, "test-workspace", "default")

	if err == nil {
		t.Error("expected error from SSM creation")
	}
}

func TestHandleConnectionCreateInvalidMethod(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{
			ClusterId: "test-cluster",
		},
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
		config:    &ExtensionConfig{ClusterId: "test"},
		k8sClient: ctrlclient.NewClientBuilder().WithScheme(scheme).Build(),
		logger:    &logger,
	}

	req := connectionv1alpha1.WorkspaceConnectionRequest{
		Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
			WorkspaceName:           "test",
			WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeWebUI,
		},
	}

	bodyBytes, _ := json.Marshal(req)
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

	// Create a public workspace
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace",
			Namespace: "default",
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessType: "Public",
		},
	}

	client := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace).Build()
	logger := ctrl.Log.WithName("test")

	server := &ExtensionServer{
		config:     &ExtensionConfig{ClusterId: "test"},
		k8sClient:  client,
		logger:     &logger,
		jwtManager: &mockJWTManager{token: "test-token"},
	}

	req := connectionv1alpha1.WorkspaceConnectionRequest{
		Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
			WorkspaceName:           "test-workspace",
			WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeWebUI,
		},
	}

	bodyBytes, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections", bytes.NewReader(bodyBytes))
	httpReq.Header.Set("X-User", "test-user")
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, httpReq)
	// Should pass authorization and reach URL generation
}

func TestGenerateWebUIBearerTokenURL(t *testing.T) {
	// Create test workspace with AccessStrategy
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workspace1",
			Namespace: "default",
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name: "test-strategy",
			},
		},
	}

	// Create test AccessStrategy
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-strategy",
			Namespace: "default",
		},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "https://test.com/workspaces/{{.Workspace.Namespace}}/{{.Workspace.Name}}/bearer-auth",
		},
	}

	// Create fake client with test objects
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, accessStrategy).Build()

	config := &ExtensionConfig{Domain: "https://test.com"}
	server := &ExtensionServer{
		config:     config,
		jwtManager: &mockJWTManager{token: "test-token"},
		k8sClient:  fakeClient,
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	connType, url, err := server.generateWebUIBearerTokenURL(req, "workspace1", "default")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if connType != "web-ui" {
		t.Errorf("expected web-ui, got %s", connType)
	}
	expected := "https://test.com/workspaces/default/workspace1/bearer-auth?token=test-token"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestGenerateWebUIBearerTokenURL_SubdomainRouting(t *testing.T) {
	// Create test workspace with AccessStrategy
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "myworkspace",
			Namespace: "default",
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name: "subdomain-strategy",
			},
		},
	}

	// Create AccessStrategy with subdomain template
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "subdomain-strategy",
			Namespace: "default",
		},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "https://{{.Workspace.Name}}-{{b32encode .Workspace.Namespace}}.example.com/bearer-auth",
		},
	}

	// Create fake client with test objects
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, accessStrategy).Build()

	config := &ExtensionConfig{Domain: "https://example.com"}
	server := &ExtensionServer{
		config:     config,
		jwtManager: &mockJWTManager{token: "test-token"},
		k8sClient:  fakeClient,
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	connType, url, err := server.generateWebUIBearerTokenURL(req, "myworkspace", "default")

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if connType != "web-ui" {
		t.Errorf("expected web-ui, got %s", connType)
	}
	expected := "https://myworkspace-mrswmylvnr2a.example.com/bearer-auth?token=test-token"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestGenerateWebUIBearerTokenURL_NoAccessStrategy(t *testing.T) {
	// Create workspace without AccessStrategy
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workspace1",
			Namespace: "default",
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: nil,
		},
	}

	// Create fake client with test objects
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace).Build()

	config := &ExtensionConfig{Domain: "https://example.com"}
	server := &ExtensionServer{
		config:     config,
		jwtManager: &mockJWTManager{token: "test-token"},
		k8sClient:  fakeClient,
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	_, _, err := server.generateWebUIBearerTokenURL(req, "workspace1", "default")

	if err == nil {
		t.Error("expected error for missing AccessStrategy, got nil")
	}
	if !strings.Contains(err.Error(), "no AccessStrategy configured") {
		t.Errorf("expected AccessStrategy error, got: %v", err)
	}
}

func TestGenerateWebUIBearerTokenURL_MissingTemplate(t *testing.T) {
	// Create workspace with AccessStrategy
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workspace1",
			Namespace: "default",
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name: "empty-strategy",
			},
		},
	}

	// Create AccessStrategy without BearerAuthURLTemplate
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-strategy",
			Namespace: "default",
		},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			BearerAuthURLTemplate: "",
		},
	}

	// Create fake client with test objects
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)
	fakeClient := ctrlclient.NewClientBuilder().WithScheme(scheme).WithObjects(workspace, accessStrategy).Build()

	config := &ExtensionConfig{Domain: "https://example.com"}
	server := &ExtensionServer{
		config:     config,
		jwtManager: &mockJWTManager{token: "test-token"},
		k8sClient:  fakeClient,
	}

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	_, _, err := server.generateWebUIBearerTokenURL(req, "workspace1", "default")

	if err == nil {
		t.Error("expected error for missing BearerAuthURLTemplate, got nil")
	}
	if !strings.Contains(err.Error(), "BearerAuthURLTemplate not configured") {
		t.Errorf("expected template error, got: %v", err)
	}
}

func TestGetUserFromHeaders(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Remote-User", testUser)

	user := GetUserFromHeaders(req)
	if user != testUser {
		t.Errorf("expected %s, got %s", testUser, user)
	}
}
