/*
Copyright 2024 The Jupyter-k8s Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
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
