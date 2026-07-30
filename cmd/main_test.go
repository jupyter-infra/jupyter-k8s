/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

// Test-local constants to avoid goconst flagging repeated string literals.
const (
	pluginNameAWS      = "aws"
	pullPolicyAlwaysLc = "always"
)

func TestParseGVKWatches(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    []GVKWatch
		wantErr bool
	}{
		{
			name:  "empty string returns nil",
			input: "",
			want:  nil,
		},
		{
			name:  "single GVK",
			input: "traefik.io/v1alpha1/IngressRoute",
			want: []GVKWatch{
				{Group: "traefik.io", Version: "v1alpha1", Kind: "IngressRoute"},
			},
		},
		{
			name:  "multiple GVKs",
			input: "traefik.io/v1alpha1/IngressRoute,networking.k8s.io/v1/Ingress",
			want: []GVKWatch{
				{Group: "traefik.io", Version: "v1alpha1", Kind: "IngressRoute"},
				{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
			},
		},
		{
			name:  "core group with empty group segment",
			input: "/v1/Service",
			want: []GVKWatch{
				{Group: "", Version: "v1", Kind: "Service"},
			},
		},
		{
			name:    "too few segments",
			input:   "traefik.io/v1alpha1",
			wantErr: true,
		},
		{
			name:    "too many segments",
			input:   "traefik.io/v1alpha1/IngressRoute/extra",
			wantErr: true,
		},
		{
			name:    "one valid one invalid",
			input:   "traefik.io/v1alpha1/IngressRoute,bad",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseGVKWatches(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid GVK format")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParsePluginEndpoints(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr bool
	}{
		{
			name:  "empty string returns nil",
			input: "",
			want:  nil,
		},
		{
			name:  "single endpoint",
			input: pluginNameAWS + "=http://localhost:8080",
			want:  map[string]string{pluginNameAWS: "http://localhost:8080"},
		},
		{
			name:  "multiple endpoints",
			input: pluginNameAWS + "=http://localhost:8080,gcp=http://localhost:8081",
			want: map[string]string{
				pluginNameAWS: "http://localhost:8080",
				"gcp":         "http://localhost:8081",
			},
		},
		{
			name:  "value containing equals sign is preserved",
			input: pluginNameAWS + "=http://localhost:8080?token=abc",
			want:  map[string]string{pluginNameAWS: "http://localhost:8080?token=abc"},
		},
		{
			name:    "missing equals",
			input:   "aws",
			wantErr: true,
		},
		{
			name:    "empty name",
			input:   "=http://localhost:8080",
			wantErr: true,
		},
		{
			name:    "empty value",
			input:   "aws=",
			wantErr: true,
		},
		{
			name:    "one valid one invalid",
			input:   "aws=http://localhost:8080,bad",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePluginEndpoints(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "invalid plugin endpoint format")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetImagePullPolicy(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  corev1.PullPolicy
	}{
		{name: "always titlecase", input: "Always", want: corev1.PullAlways},
		{name: "always lowercase", input: pullPolicyAlwaysLc, want: corev1.PullAlways},
		{name: "never", input: "Never", want: corev1.PullNever},
		{name: "ifnotpresent", input: "IfNotPresent", want: corev1.PullIfNotPresent},
		{name: "empty defaults to IfNotPresent", input: "", want: corev1.PullIfNotPresent},
		{name: "unknown defaults to IfNotPresent", input: "bogus", want: corev1.PullIfNotPresent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, getImagePullPolicy(tt.input))
		})
	}
}
