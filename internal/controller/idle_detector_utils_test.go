/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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
