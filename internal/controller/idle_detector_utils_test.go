/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

func TestResolveIdlePath(t *testing.T) {
	tests := []struct {
		basePath    string
		httpGetPath string
		expected    string
	}{
		{"", "/api/status", "/api/status"},
		{"/", "/api/status", "/api/status"},
		{"/workspaces/ns/ws/", "/api/status", "/workspaces/ns/ws/api/status"},
		{"/workspaces/ns/ws", "/api/status", "/workspaces/ns/ws/api/status"},
		{"/workspaces/ns/ws/", "api/status", "/workspaces/ns/ws/api/status"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s+%s", tt.basePath, tt.httpGetPath), func(t *testing.T) {
			result := resolveIdlePath(tt.basePath, tt.httpGetPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildIdleProbeURL(t *testing.T) {
	tests := []struct {
		name     string
		scheme   corev1.URIScheme
		port     intstr.IntOrString
		host     string
		fullPath string
		expected string
	}{
		{
			name:     "default scheme when empty",
			port:     intstr.FromInt(8888),
			host:     "10.96.0.1",
			fullPath: "/api/status",
			expected: "http://10.96.0.1:8888/api/status",
		},
		{
			name:     "explicit https scheme",
			scheme:   corev1.URISchemeHTTPS,
			port:     intstr.FromInt(8888),
			host:     "10.96.0.1",
			fullPath: "/api/status",
			expected: "https://10.96.0.1:8888/api/status",
		},
		{
			// Path missing a leading slash must still produce a parseable URL
			// rather than "http://host:8888api/status" (which fails to parse and
			// would silently disable idle shutdown for the workspace).
			name:     "path without leading slash is normalized",
			port:     intstr.FromInt(8888),
			host:     "10.96.0.1",
			fullPath: "api/status",
			expected: "http://10.96.0.1:8888/api/status",
		},
		{
			// A workspace-controlled path beginning with "@" must NOT be able to
			// collapse the pinned host into userinfo and steer the probe elsewhere.
			// The host must remain the ClusterIP we were given.
			name:     "userinfo injection cannot override host",
			port:     intstr.FromInt(8888),
			host:     "10.96.0.1",
			fullPath: "@10.0.0.5/x",
			expected: "http://10.96.0.1:8888/@10.0.0.5/x",
		},
		{
			name:     "ipv6 host is bracketed",
			port:     intstr.FromInt(8888),
			host:     "fd00::1",
			fullPath: "/api/status",
			expected: "http://[fd00::1]:8888/api/status",
		},
		{
			// Scheme comes from the CRD as a corev1.URIScheme ("HTTP"/"HTTPS",
			// uppercase). It must be lowercased so the URL scheme is valid.
			name:     "uppercase scheme is lowercased",
			scheme:   corev1.URIScheme("HTTP"),
			port:     intstr.FromInt(8888),
			host:     "10.96.0.1",
			fullPath: "/api/status",
			expected: "http://10.96.0.1:8888/api/status",
		},
		{
			// Port may be a named service port (intstr string form); it must be
			// passed through verbatim rather than coerced to a number.
			name:     "named string port is preserved",
			port:     intstr.FromString("http-web"),
			host:     "10.96.0.1",
			fullPath: "/api/status",
			expected: "http://10.96.0.1:http-web/api/status",
		},
		{
			name:     "localhost host (podExec transport)",
			port:     intstr.FromInt(8888),
			host:     "localhost",
			fullPath: "/api/idle",
			expected: "http://localhost:8888/api/idle",
		},
		{
			// An empty resolved path must still yield a valid root URL.
			name:     "empty path becomes root",
			port:     intstr.FromInt(8888),
			host:     "10.96.0.1",
			fullPath: "",
			expected: "http://10.96.0.1:8888/",
		},
		{
			// A workspace-controlled path must not be able to smuggle a query
			// string into the probe URL; "?" is escaped inside the path segment.
			name:     "query injection in path is escaped",
			port:     intstr.FromInt(8888),
			host:     "10.96.0.1",
			fullPath: "/api/status?foo=bar",
			expected: "http://10.96.0.1:8888/api/status%3Ffoo=bar",
		},
		{
			// A protocol-relative-looking path ("//host") stays in the path and
			// cannot repoint the request at another host.
			name:     "double-slash path cannot repoint host",
			port:     intstr.FromInt(8888),
			host:     "10.96.0.1",
			fullPath: "//evil.example.com/x",
			expected: "http://10.96.0.1:8888//evil.example.com/x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &workspacev1alpha1.IdleHTTPGetAction{
				HTTPGetAction: corev1.HTTPGetAction{
					Scheme: tt.scheme,
					Port:   tt.port,
				},
			}
			result := buildIdleProbeURL(cfg, tt.host, tt.fullPath)
			assert.Equal(t, tt.expected, result)
		})
	}

	// Security invariant: regardless of the workspace-controlled path, the host
	// the request actually targets must remain the host we pinned. Parse the
	// result back and assert net/http would dial the pinned host:port.
	t.Run("host stays pinned for hostile paths", func(t *testing.T) {
		const host, port = "10.96.0.1", "8888"
		hostilePaths := []string{
			"@10.0.0.5/x",          // userinfo collapse
			"//evil.example.com/x", // protocol-relative
			"@evil.example.com",    // userinfo without path
			"api/status",           // missing leading slash
		}
		cfg := &workspacev1alpha1.IdleHTTPGetAction{
			HTTPGetAction: corev1.HTTPGetAction{Port: intstr.FromString(port)},
		}
		for _, p := range hostilePaths {
			parsed, err := url.Parse(buildIdleProbeURL(cfg, host, p))
			assert.NoError(t, err, "path %q should produce a parseable URL", p)
			assert.Equal(t, host+":"+port, parsed.Host, "path %q must not repoint the host", p)
		}
	})
}

func TestExtractJSONField(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		path    string
		want    string
		wantErr bool
	}{
		{"simple", `{"last_activity": "2024-01-01T00:00:00Z"}`, "last_activity", "2024-01-01T00:00:00Z", false},
		{"nested", `{"status": {"ts": "2024-01-01T00:00:00Z"}}`, "status.ts", "2024-01-01T00:00:00Z", false},
		{"deeply_nested", `{"a": {"b": {"c": "val"}}}`, "a.b.c", "val", false},
		{"numeric", `{"ts": 1704067200}`, "ts", "1704067200", false},
		{"numeric_float", `{"ts": 1704067200.123}`, "ts", "1704067200.123", false},
		{"default_field", `{"lastActiveTimestamp": "2024-01-01T00:00:00Z"}`, "lastActiveTimestamp", "2024-01-01T00:00:00Z", false},
		{"missing_field", `{"other": "value"}`, "last_activity", "", true},
		{"missing_nested", `{"a": {"b": "val"}}`, "a.c", "", true},
		{"not_object_at_path", `{"a": "string"}`, "a.b", "", true},
		{"invalid_json", `not json`, "field", "", true},
		{"empty_body", ``, "field", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractJSONField(tt.body, tt.path)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		format  string
		wantErr bool
	}{
		{"rfc3339_utc", "2024-01-01T00:00:00Z", "RFC3339", false},
		{"rfc3339_offset", "2024-01-01T05:30:00+05:30", "RFC3339", false},
		{"rfc3339_lowercase_z", "2024-01-01t00:00:00z", "RFC3339", false},
		{"rfc3339_fractional", "2024-01-01T00:00:00.123456Z", "RFC3339", false},
		{"unix_int", "1704067200", "unix", false},
		{"unix_float", "1704067200.5", "unix", false},
		{"unix_string_int", "1704067200", "unix", false},
		{"invalid_rfc3339", "not-a-date", "RFC3339", true},
		{"invalid_unix", "abc", "unix", true},
		{"empty_value_rfc3339", "", "RFC3339", true},
		{"empty_value_unix", "", "unix", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTimestamp(tt.value, tt.format)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
