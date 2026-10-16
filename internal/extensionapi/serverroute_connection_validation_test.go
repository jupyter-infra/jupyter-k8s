/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

import (
	"net/http/httptest"
	"testing"

	connectionv1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/connection/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
					WorkspaceName:           testWorkspace,
					WorkspaceConnectionType: connectionTypeVSCodeRemote,
				},
			},
		},
		{
			name: "valid web-ui request",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           testWorkspace,
					WorkspaceConnectionType: connectionv1alpha1.ConnectionTypeWebUI,
				},
			},
		},
		{
			name: "valid kiro-remote request",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           testWorkspace,
					WorkspaceConnectionType: "kiro-remote",
				},
			},
		},
		{
			name: "valid cursor-remote request",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           testWorkspace,
					WorkspaceConnectionType: "cursor-remote",
				},
			},
		},
		{
			name: "valid unknown *-remote request",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           testWorkspace,
					WorkspaceConnectionType: "windsurf-remote",
				},
			},
		},
		{
			name: "missing workspace name",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceConnectionType: connectionTypeVSCodeRemote,
				},
			},
			expectError: true,
			errorMsg:    "workspaceName is required",
		},
		{
			name: "missing connection type",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName: testWorkspace,
				},
			},
			expectError: true,
			errorMsg:    "workspaceConnectionType is required",
		},
		{
			name: "invalid connection type - no -remote suffix",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           testWorkspace,
					WorkspaceConnectionType: invalidConnectionType,
				},
			},
			expectError: true,
			errorMsg:    "invalid workspaceConnectionType: 'invalid-type'. Must be 'web-ui', 'ssh-over-websocket', or follow the '{ide}-remote' pattern (e.g. 'vscode-remote', 'kiro-remote', 'cursor-remote')",
		},
		{
			name: "invalid connection type - bare remote",
			req: &connectionv1alpha1.WorkspaceConnectionRequest{
				Spec: connectionv1alpha1.WorkspaceConnectionRequestSpec{
					WorkspaceName:           testWorkspace,
					WorkspaceConnectionType: "-remote",
				},
			},
			expectError: true,
			errorMsg:    "invalid workspaceConnectionType: '-remote'. Must be 'web-ui', 'ssh-over-websocket', or follow the '{ide}-remote' pattern (e.g. 'vscode-remote', 'kiro-remote', 'cursor-remote')",
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

	_, result, err := server.checkWorkspaceAuthorization(req, testWorkspace, namespaceDefault)

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
						{Type: conditionTypeAvailable, Status: metav1.ConditionTrue},
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
						{Type: conditionTypeAvailable, Status: metav1.ConditionFalse},
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
		{connectionTypeVSCodeRemote, true},
		{"kiro-remote", true},
		{"cursor-remote", true},
		{"windsurf-remote", true},
		{"my-vscode-remote", true},
		{connectionTypeWebUI, false},
		{invalidConnectionType, false},
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
