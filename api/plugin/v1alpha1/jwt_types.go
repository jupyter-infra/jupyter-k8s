/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package v1alpha1 defines the request/response types for the plugin HTTP interface.
// These are plain Go structs with JSON tags — not Kubernetes API objects.
// Zero external dependencies so both core operator and plugin binaries can import them.
package v1alpha1

// SignRequest is the request body for POST /v1alpha1/jwt/sign.
type SignRequest struct {
	User              string              `json:"user"`
	Groups            []string            `json:"groups"`
	UID               string              `json:"uid"`
	Extra             map[string][]string `json:"extra,omitempty"`
	Path              string              `json:"path"`
	Domain            string              `json:"domain"`
	TokenType         string              `json:"tokenType"`
	ConnectionContext map[string]string   `json:"connectionContext,omitempty"`
}

// SignResponse is the response body for POST /v1alpha1/jwt/sign.
type SignResponse struct {
	Token string `json:"token"`
}

// VerifyRequest is the request body for POST /v1alpha1/jwt/verify.
type VerifyRequest struct {
	Token string `json:"token"`
}

// VerifyResponse is the response body for POST /v1alpha1/jwt/verify.
type VerifyResponse struct {
	Claims *VerifyClaims `json:"claims"`
}

// VerifyClaims contains the decoded JWT claims returned by verify.
type VerifyClaims struct {
	Subject   string              `json:"sub"`
	Groups    []string            `json:"groups"`
	UID       string              `json:"uid"`
	Extra     map[string][]string `json:"extra,omitempty"`
	Path      string              `json:"path"`
	Domain    string              `json:"domain"`
	TokenType string              `json:"tokenType"`
	ExpiresAt int64               `json:"exp"`
}
