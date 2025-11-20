/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
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
