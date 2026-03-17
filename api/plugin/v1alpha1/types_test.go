/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"encoding/json"
	"testing"
)

func TestSignRequestRoundTrip(t *testing.T) {
	req := SignRequest{
		User:      "alice",
		Groups:    []string{"admin", "dev"},
		UID:       "uid-123",
		Extra:     map[string][]string{"team": {"infra"}},
		Path:      "/workspaces/ns/ws",
		Domain:    "example.com",
		TokenType: "bootstrap",
		ConnectionContext: map[string]string{
			"kmsKeyId": "arn:aws:kms:us-east-1:123:key/abc",
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got SignRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.User != req.User || got.Path != req.Path || got.ConnectionContext["kmsKeyId"] != req.ConnectionContext["kmsKeyId"] {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func TestSignResponseRoundTrip(t *testing.T) {
	resp := SignResponse{Token: "mock-jwt-token.test.sig"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got SignResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Token != resp.Token {
		t.Errorf("expected token %q, got %q", resp.Token, got.Token)
	}
}

func TestVerifyRequestRoundTrip(t *testing.T) {
	req := VerifyRequest{Token: "some-token"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got VerifyRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Token != req.Token {
		t.Errorf("expected token %q, got %q", req.Token, got.Token)
	}
}

func TestVerifyResponseRoundTrip(t *testing.T) {
	resp := VerifyResponse{
		Claims: &VerifyClaims{
			Subject:   "alice",
			Groups:    []string{"admin"},
			UID:       "uid-123",
			Extra:     map[string][]string{"team": {"infra"}},
			Path:      "/workspaces/ns/ws",
			Domain:    "example.com",
			TokenType: "bootstrap",
			ExpiresAt: 1234567890,
		},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got VerifyResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Claims.Subject != "alice" || got.Claims.ExpiresAt != 1234567890 {
		t.Errorf("round-trip mismatch: got %+v", got.Claims)
	}
}

func TestRegisterNodeAgentRoundTrip(t *testing.T) {
	req := RegisterNodeAgentRequest{
		PodUID:        "pod-uid-1",
		WorkspaceName: "my-ws",
		Namespace:     "default",
		PodEventsContext: map[string]string{
			"ssmDocumentName": "MyDoc",
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got RegisterNodeAgentRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.PodUID != req.PodUID || got.PodEventsContext["ssmDocumentName"] != "MyDoc" {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func TestRegisterNodeAgentResponseRoundTrip(t *testing.T) {
	resp := RegisterNodeAgentResponse{
		ActivationID:   "act-123",
		ActivationCode: "code-456",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got RegisterNodeAgentResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ActivationID != resp.ActivationID || got.ActivationCode != resp.ActivationCode {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func TestCreateSessionRequestRoundTrip(t *testing.T) {
	req := CreateSessionRequest{
		PodUID:        "pod-uid-1",
		WorkspaceName: "my-ws",
		Namespace:     "default",
		ConnectionContext: map[string]string{
			"ssmDocumentName": "MyDoc",
		},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got CreateSessionRequest
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.PodUID != req.PodUID || got.ConnectionContext["ssmDocumentName"] != "MyDoc" {
		t.Errorf("round-trip mismatch: got %+v", got)
	}
}

func TestCreateSessionResponseRoundTrip(t *testing.T) {
	resp := CreateSessionResponse{ConnectionURL: "vscode://some-url"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got CreateSessionResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ConnectionURL != resp.ConnectionURL {
		t.Errorf("expected %q, got %q", resp.ConnectionURL, got.ConnectionURL)
	}
}

func TestErrorResponseRoundTrip(t *testing.T) {
	resp := ErrorResponse{Error: "something went wrong"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ErrorResponse
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Error != resp.Error {
		t.Errorf("expected %q, got %q", resp.Error, got.Error)
	}
}

func TestOmitEmptyFields(t *testing.T) {
	req := SignRequest{User: "alice", Path: "/ws", Domain: "d", TokenType: "bootstrap"}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	str := string(data)
	// Fields with omitempty should not appear when zero-valued
	for _, field := range []string{"extra", "connectionContext"} {
		if contains(str, field) {
			t.Errorf("expected %q to be omitted, got: %s", field, str)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
