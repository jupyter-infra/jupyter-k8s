/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import "testing"

func TestIsRemoteConnectionType(t *testing.T) {
	tests := []struct {
		connectionType string
		expected       bool
	}{
		{"vscode-remote", true},
		{"kiro-remote", true},
		{"cursor-remote", true},
		{"windsurf-remote", true},
		{"my-vscode-remote", true},
		{ConnectionTypeWebUI, false},
		{ConnectionTypeWebSocket, false},
		{"invalid-type", false},
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
			if got := IsRemoteConnectionType(tt.connectionType); got != tt.expected {
				t.Errorf("IsRemoteConnectionType(%q) = %v, want %v", tt.connectionType, got, tt.expected)
			}
		})
	}
}
