package extensionapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleConnectionCreate(t *testing.T) {
	config := &Config{
		ClusterARN: "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster",
	}

	tests := []struct {
		name           string
		method         string
		path           string
		body           interface{}
		expectedStatus int
		expectedError  string
	}{
		{
			name:           "missing spaceName",
			method:         "POST",
			path:           "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections",
			body:           map[string]interface{}{"spec": map[string]interface{}{"ide": "vscode"}},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "spaceName is required",
		},
		{
			name:           "missing spec",
			method:         "POST",
			path:           "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections",
			body:           map[string]interface{}{"metadata": map[string]interface{}{"name": "test"}},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Missing spec",
		},
		{
			name:           "invalid JSON",
			method:         "POST",
			path:           "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
			expectedError:  "Invalid JSON",
		},
		{
			name:           "empty spaceName",
			method:         "POST",
			path:           "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections",
			body:           map[string]interface{}{"spec": map[string]interface{}{"spaceName": "", "ide": "vscode"}},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "spaceName is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader io.Reader
			if str, ok := tt.body.(string); ok {
				bodyReader = strings.NewReader(str)
			} else {
				bodyBytes, _ := json.Marshal(tt.body)
				bodyReader = bytes.NewReader(bodyBytes)
			}

			req := httptest.NewRequest(tt.method, tt.path, bodyReader)
			w := httptest.NewRecorder()

			handler := handleConnectionCreate(config)
			handler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedError != "" && !strings.Contains(w.Body.String(), tt.expectedError) {
				t.Errorf("expected error containing %q, got %q", tt.expectedError, w.Body.String())
			}
		})
	}
}

func TestHandleConnectionCreateReadBodyError(t *testing.T) {
	config := &Config{ClusterARN: "test-arn"}

	req := httptest.NewRequest("POST", "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections", &errorReader{})
	w := httptest.NewRecorder()

	handler := handleConnectionCreate(config)
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

// errorReader simulates a read error
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}
