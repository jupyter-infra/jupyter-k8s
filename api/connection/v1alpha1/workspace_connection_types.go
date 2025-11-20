/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// WorkspaceConnectionAPIVersion is the API version for workspace connections
	WorkspaceConnectionAPIVersion = "connection.workspace.jupyter.org/v1alpha1"
	// WorkspaceConnectionKind is the kind for workspace connection resources
	WorkspaceConnectionKind = "WorkspaceConnection"

	// ConnectionTypeVSCodeRemote represents VSCode remote connection type
	ConnectionTypeVSCodeRemote = "vscode-remote"
	// ConnectionTypeWebUI represents web UI connection type
	ConnectionTypeWebUI = "web-ui"
)

// WorkspaceConnectionRequestSpec represents the spec of a workspace connection request
type WorkspaceConnectionRequestSpec struct {
	WorkspaceName           string `json:"workspaceName"`
	WorkspaceConnectionType string `json:"workspaceConnectionType"`
}

// WorkspaceConnectionResponseStatus represents the status of a workspace connection response
type WorkspaceConnectionResponseStatus struct {
	WorkspaceConnectionType string `json:"workspaceConnectionType"`
	WorkspaceConnectionURL  string `json:"workspaceConnectionUrl"`
}

// +kubebuilder:object:root=true

// WorkspaceConnectionRequest represents the request body for creating a workspace connection
type WorkspaceConnectionRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WorkspaceConnectionRequestSpec `json:"spec"`
}

// +kubebuilder:object:root=true

// WorkspaceConnectionResponse represents the response for a workspace connection
type WorkspaceConnectionResponse struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              WorkspaceConnectionRequestSpec    `json:"spec"`
	Status            WorkspaceConnectionResponseStatus `json:"status"`
}
