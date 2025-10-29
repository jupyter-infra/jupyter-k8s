package extensionapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	connectionv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/connection/v1alpha1"
)

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
			errorMsg:    "invalid workspaceConnectionType: invalid-type",
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
	server := &ExtensionServer{}

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

func TestHandleConnectionCreate_VSCodeMissingClusterARN(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{}, // Empty config, no EKSClusterARN
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

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, w.Code)
	}
}

func TestGenerateVSCodeURL_MissingSSMDocument(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{
			EKSClusterARN: "arn:aws:eks:us-east-1:123456789012:cluster/test",
		},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	_, _, err := server.generateVSCodeURL(req, "test-workspace", "default")

	if err == nil {
		t.Error("expected error when AWS_SSM_DOCUMENT_NAME is not set")
	}
}

func TestGenerateVSCodeURL_MissingClusterARN(t *testing.T) {
	server := &ExtensionServer{
		config: &ExtensionConfig{
			EKSClusterARN: "", // Empty cluster ARN
		},
	}

	req := httptest.NewRequest("POST", "/test", nil)
	_, _, err := server.generateVSCodeURL(req, "test-workspace", "default")

	if err == nil {
		t.Error("expected error when EKSClusterARN is empty")
	}
}

func TestHandleConnectionCreate_InvalidConnectionType(t *testing.T) {
	server := &ExtensionServer{}

	req := connectionv1alpha1.WorkspaceConnectionRequest{
		Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
			WorkspaceName:           "test-workspace",
			WorkspaceConnectionType: "invalid-type",
		},
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/apis/connection.workspace.jupyter.org/v1alpha1/namespaces/default/workspaceconnectionrequests", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	server.HandleConnectionCreate(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}
