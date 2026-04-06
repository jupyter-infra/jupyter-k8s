/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

// InitializeRequest is the request body for POST /v1alpha1/remote-access/initialize.
type InitializeRequest struct{}

// InitializeResponse is the response body for POST /v1alpha1/remote-access/initialize.
type InitializeResponse struct{}

// RegisterNodeAgentRequest is the request body for POST /v1alpha1/remote-access/register-node-agent.
type RegisterNodeAgentRequest struct {
	PodUID           string            `json:"podUID"`
	WorkspaceName    string            `json:"workspaceName"`
	Namespace        string            `json:"namespace"`
	PodEventsContext map[string]string `json:"podEventsContext,omitempty"`
}

// RegisterNodeAgentResponse is the response body for POST /v1alpha1/remote-access/register-node-agent.
type RegisterNodeAgentResponse struct {
	ActivationID   string `json:"activationId"`
	ActivationCode string `json:"activationCode"`
}

// DeregisterNodeAgentRequest is the request body for POST /v1alpha1/remote-access/deregister-node-agent.
type DeregisterNodeAgentRequest struct {
	PodUID string `json:"podUID"`
}

// DeregisterNodeAgentResponse is the response body for POST /v1alpha1/remote-access/deregister-node-agent.
type DeregisterNodeAgentResponse struct{}

// CreateSessionRequest is the request body for POST /v1alpha1/remote-access/create-session.
type CreateSessionRequest struct {
	PodUID            string            `json:"podUID"`
	WorkspaceName     string            `json:"workspaceName"`
	Namespace         string            `json:"namespace"`
	ConnectionType    string            `json:"connectionType,omitempty"`
	ConnectionContext map[string]string `json:"connectionContext,omitempty"`
}

// CreateSessionResponse is the response body for POST /v1alpha1/remote-access/create-session.
type CreateSessionResponse struct {
	ConnectionURL string `json:"connectionURL"`
}
