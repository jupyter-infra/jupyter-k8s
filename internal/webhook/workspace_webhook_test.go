package webhook

import (
	"context"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestWorkspaceMutator_Handle(t *testing.T) {
	tests := []struct {
		name            string
		req             admission.Request
		expectedAllowed bool
		expectedPatch   string
		expectError     bool
	}{
		{
			name: "successful mutation with no existing annotations",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Group:   "workspaces.jupyter.org",
						Version: "v1alpha1",
						Kind:    "Workspace",
					},
					Object: runtime.RawExtension{
						Raw: []byte(`{"metadata":{"name":"test-workspace"}}`),
					},
					UserInfo: authenticationv1.UserInfo{
						Username: "test-user",
					},
				},
			},
			expectedAllowed: true,
			expectedPatch:   `[{"op":"add","path":"/metadata/annotations","value":{"creator-username":"test-user"}}]`,
		},
		{
			name: "successful mutation with existing annotations",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Group:   "workspaces.jupyter.org",
						Version: "v1alpha1",
						Kind:    "Workspace",
					},
					Object: runtime.RawExtension{
						Raw: []byte(`{"metadata":{"name":"test-workspace","annotations":{"existing":"value"}}}`),
					},
					UserInfo: authenticationv1.UserInfo{
						Username: "test-user",
					},
				},
			},
			expectedAllowed: true,
			expectedPatch:   `[{"op":"add","path":"/metadata/annotations/creator-username","value":"test-user"}]`,
		},
		{
			name: "username with special characters",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Group:   "workspaces.jupyter.org",
						Version: "v1alpha1",
						Kind:    "Workspace",
					},
					Object: runtime.RawExtension{
						Raw: []byte(`{"metadata":{"name":"test-workspace"}}`),
					},
					UserInfo: authenticationv1.UserInfo{
						Username: `user@domain.com`,
					},
				},
			},
			expectedAllowed: true,
			expectedPatch:   `[{"op":"add","path":"/metadata/annotations","value":{"creator-username":"user@domain.com"}}]`,
		},
		{
			name: "non-workspace resource should be allowed without mutation",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Group:   "apps",
						Version: "v1",
						Kind:    "Deployment",
					},
					Object: runtime.RawExtension{
						Raw: []byte(`{"metadata":{"name":"test-deployment"}}`),
					},
					UserInfo: authenticationv1.UserInfo{
						Username: "test-user",
					},
				},
			},
			expectedAllowed: true,
			expectedPatch:   "",
		},
		{
			name: "nil object should return error",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Group:   "workspaces.jupyter.org",
						Version: "v1alpha1",
						Kind:    "Workspace",
					},
					Object: runtime.RawExtension{
						Raw: nil,
					},
					UserInfo: authenticationv1.UserInfo{
						Username: "test-user",
					},
				},
			},
			expectedAllowed: false,
			expectError:     true,
		},
		{
			name: "invalid JSON should return error",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Group:   "workspaces.jupyter.org",
						Version: "v1alpha1",
						Kind:    "Workspace",
					},
					Object: runtime.RawExtension{
						Raw: []byte(`invalid json`),
					},
					UserInfo: authenticationv1.UserInfo{
						Username: "test-user",
					},
				},
			},
			expectedAllowed: false,
			expectError:     true,
		},
		{
			name: "missing metadata should return error",
			req: admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Kind: metav1.GroupVersionKind{
						Group:   "workspaces.jupyter.org",
						Version: "v1alpha1",
						Kind:    "Workspace",
					},
					Object: runtime.RawExtension{
						Raw: []byte(`{"spec":{}}`),
					},
					UserInfo: authenticationv1.UserInfo{
						Username: "test-user",
					},
				},
			},
			expectedAllowed: false,
			expectError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &WorkspaceMutator{}
			resp := m.Handle(context.Background(), tt.req)

			if resp.Allowed != tt.expectedAllowed {
				t.Errorf("expected allowed=%v, got=%v", tt.expectedAllowed, resp.Allowed)
			}

			if tt.expectError && resp.Result.Code == 200 {
				t.Error("expected error but got success")
			}

			if !tt.expectError && resp.Result.Code != 200 {
				t.Errorf("expected success but got error: %v", resp.Result.Message)
			}

			if tt.expectedPatch != "" {
				if string(resp.Patch) != tt.expectedPatch {
					t.Errorf("expected patch=%s, got=%s", tt.expectedPatch, string(resp.Patch))
				}
			}
		})
	}
}

func TestSanitizeUsername(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple username",
			input:    "testuser",
			expected: "testuser",
		},
		{
			name:     "username with email",
			input:    "user@domain.com",
			expected: "user@domain.com",
		},
		{
			name:     "username with quotes",
			input:    `user"with"quotes`,
			expected: `user\"with\"quotes`,
		},
		{
			name:     "username with backslashes",
			input:    `user\with\backslashes`,
			expected: `user\\with\\backslashes`,
		},
		{
			name:     "empty username",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeUsername(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestWorkspaceMutator_InjectDecoder(t *testing.T) {
	m := &WorkspaceMutator{}
	err := m.InjectDecoder(nil)
	if err != nil {
		t.Errorf("InjectDecoder should not return error, got: %v", err)
	}
}
