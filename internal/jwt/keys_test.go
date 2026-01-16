/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildKeyName(t *testing.T) {
	timestamp := int64(1609459200) // 2021-01-01 00:00:00 UTC
	expected := "jwt-signing-key-1609459200"
	result := BuildKeyName(timestamp)

	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}

func TestParseKeyTimestamp(t *testing.T) {
	tests := []struct {
		name          string
		keyName       string
		expected      int64
		expectError   bool
		errorContains string
	}{
		{
			name:        "valid key name",
			keyName:     "jwt-signing-key-1609459200",
			expected:    1609459200,
			expectError: false,
		},
		{
			name:          "missing prefix",
			keyName:       "invalid-key-1609459200",
			expectError:   true,
			errorContains: "does not have prefix",
		},
		{
			name:          "invalid timestamp",
			keyName:       "jwt-signing-key-notanumber",
			expectError:   true,
			errorContains: "failed to parse timestamp",
		},
		{
			name:          "empty timestamp",
			keyName:       "jwt-signing-key-",
			expectError:   true,
			errorContains: "failed to parse timestamp",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseKeyTimestamp(tt.keyName)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("Expected %d, got %d", tt.expected, result)
				}
			}
		})
	}
}

func TestParseSigningKeysFromSecret(t *testing.T) {
	tests := []struct {
		name          string
		secretData    map[string][]byte
		expectedKeys  int
		expectedKid   string
		expectError   bool
		errorContains string
	}{
		{
			name: "single key",
			secretData: map[string][]byte{
				"jwt-signing-key-1000": []byte("key1"),
			},
			expectedKeys: 1,
			expectedKid:  "1000",
			expectError:  false,
		},
		{
			name: "multiple keys",
			secretData: map[string][]byte{
				"jwt-signing-key-1000": []byte("key1"),
				"jwt-signing-key-2000": []byte("key2"),
				"jwt-signing-key-3000": []byte("key3"),
			},
			expectedKeys: 3,
			expectedKid:  "3000",
			expectError:  false,
		},
		{
			name: "mixed keys and other data",
			secretData: map[string][]byte{
				"jwt-signing-key-1000": []byte("key1"),
				"jwt-signing-key-2000": []byte("key2"),
				"other-data":           []byte("ignored"),
			},
			expectedKeys: 2,
			expectedKid:  "2000",
			expectError:  false,
		},
		{
			name:          "no data",
			secretData:    nil,
			expectError:   true,
			errorContains: "secret has no data",
		},
		{
			name: "no signing keys",
			secretData: map[string][]byte{
				"other-key": []byte("notakey"),
			},
			expectError:   true,
			errorContains: "no signing keys found",
		},
		{
			name: "invalid key format",
			secretData: map[string][]byte{
				"jwt-signing-key-invalid": []byte("badkey"),
			},
			expectError:   true,
			errorContains: "invalid key format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-secret",
					Namespace: "test-namespace",
				},
				Data: tt.secretData,
			}

			keys, kid, err := ParseSigningKeysFromSecret(secret)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				if tt.errorContains != "" && !contains(err.Error(), tt.errorContains) {
					t.Errorf("Expected error containing '%s', got '%s'", tt.errorContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if len(keys) != tt.expectedKeys {
					t.Errorf("Expected %d keys, got %d", tt.expectedKeys, len(keys))
				}
				if kid != tt.expectedKid {
					t.Errorf("Expected kid '%s', got '%s'", tt.expectedKid, kid)
				}

				// Verify all keys are accessible by their kid
				for expectedKid := range keys {
					if _, ok := keys[expectedKid]; !ok {
						t.Errorf("Expected key with kid '%s' not found", expectedKid)
					}
				}
			}
		})
	}
}

func TestFormatKeyForDisplay(t *testing.T) {
	tests := []struct {
		name     string
		key      []byte
		expected string
	}{
		{
			name:     "empty key",
			key:      []byte{},
			expected: "<empty>",
		},
		{
			name:     "short key",
			key:      []byte("test"),
			expected: "dGVzdA==",
		},
		{
			name:     "long key truncated",
			key:      []byte("this is a very long key that should be truncated"),
			expected: "dGhpcyBpcyBhIHZl...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatKeyForDisplay(tt.key)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || hasSubstring(s, substr))
}

func hasSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
