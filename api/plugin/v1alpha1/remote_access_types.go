/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

// InitializeRequest is the request body for POST /v1alpha1/remote-access/initialize.
type InitializeRequest struct{}

// InitializeResponse is the response body for POST /v1alpha1/remote-access/initialize.
type InitializeResponse struct{}

// RegisterNodeRequest is the request body for POST /v1alpha1/remote-access/register-node.
type RegisterNodeRequest struct {
	PodUID           string            `json:"podUID"`
	WorkspaceName    string            `json:"workspaceName"`
	Namespace        string            `json:"namespace"`
	PodEventsContext map[string]string `json:"podEventsContext,omitempty"`
}

// RegisterNodeResponse is the response body for POST /v1alpha1/remote-access/register-node.
type RegisterNodeResponse struct {
	ActivationID   string `json:"activationId"`
	ActivationCode string `json:"activationCode"`
}

// DeregisterNodeRequest is the request body for POST /v1alpha1/remote-access/deregister-node.
type DeregisterNodeRequest struct {
	PodUID string `json:"podUID"`
}

// DeregisterNodeResponse is the response body for POST /v1alpha1/remote-access/deregister-node.
type DeregisterNodeResponse struct{}

// CreateSessionRequest is the request body for POST /v1alpha1/remote-access/create-session.
type CreateSessionRequest struct {
	PodUID            string            `json:"podUID"`
	WorkspaceName     string            `json:"workspaceName"`
	Namespace         string            `json:"namespace"`
	ConnectionContext map[string]string `json:"connectionContext,omitempty"`
}

// CreateSessionResponse is the response body for POST /v1alpha1/remote-access/create-session.
type CreateSessionResponse struct {
	ConnectionURL string `json:"connectionURL"`
}
