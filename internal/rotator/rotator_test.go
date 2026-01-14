/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package rotator

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	testSecretName = "test-secret"
	testNamespace  = "test-namespace"
)

// getTestClient creates a fake controller-runtime client for testing
func getTestClient(objects ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	return fakeclient.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}

func TestGenerateKey(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	if len(key) != KeySizeBytes {
		t.Errorf("Expected key size %d, got %d", KeySizeBytes, len(key))
	}

	// Generate another key and verify it's different (extremely unlikely to collide)
	key2, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey failed on second call: %v", err)
	}

	if string(key) == string(key2) {
		t.Error("Two generated keys are identical (collision)")
	}
}

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
					t.Fatal("Expected error but got none")
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

func TestRotateSecret_NewSecret(t *testing.T) {
	ctx := context.Background()
	secretName := testSecretName

	// Create empty secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNamespace,
		},
	}
	k8sClient := getTestClient(secret)

	// Rotate secret
	err := RotateSecret(ctx, k8sClient, secretName, testNamespace, 3)
	if err != nil {
		t.Fatalf("RotateSecret failed: %v", err)
	}

	// Verify secret has one key
	updatedSecret := &corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, updatedSecret)
	if err != nil {
		t.Fatalf("Failed to get updated secret: %v", err)
	}

	keyCount := 0
	for name := range updatedSecret.Data {
		if hasPrefix(name, KeyPrefix) {
			keyCount++
		}
	}

	if keyCount != 1 {
		t.Errorf("Expected 1 key after first rotation, got %d", keyCount)
	}
}

func TestRotateSecret_AddAndPruneKeys(t *testing.T) {
	ctx := context.Background()
	secretName := testSecretName
	numberOfKeys := 3

	// Create secret with initial key
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"jwt-signing-key-1000": []byte("key1"),
		},
	}
	k8sClient := getTestClient(secret)

	// Rotate 4 times (should end up with 3 keys due to pruning)
	for i := 0; i < 4; i++ {
		time.Sleep(1 * time.Second) // Ensure different timestamps (unix timestamp precision is 1 second)
		err := RotateSecret(ctx, k8sClient, secretName, testNamespace, numberOfKeys)
		if err != nil {
			t.Fatalf("RotateSecret failed on iteration %d: %v", i, err)
		}
	}

	// Verify we have exactly numberOfKeys keys
	updatedSecret := &corev1.Secret{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, updatedSecret)
	if err != nil {
		t.Fatalf("Failed to get updated secret: %v", err)
	}

	keys := []string{}
	for name := range updatedSecret.Data {
		if hasPrefix(name, KeyPrefix) {
			keys = append(keys, name)
		}
	}

	if len(keys) != numberOfKeys {
		t.Errorf("Expected %d keys after pruning, got %d: %v", numberOfKeys, len(keys), keys)
	}

	// Verify the oldest key (timestamp 1000) was pruned
	for _, keyName := range keys {
		if keyName == "jwt-signing-key-1000" {
			t.Error("Expected oldest key to be pruned but it still exists")
		}
	}
}

func TestRotateSecret_InvalidNumberOfKeys(t *testing.T) {
	k8sClient := getTestClient()
	ctx := context.Background()

	err := RotateSecret(ctx, k8sClient, testSecretName, testNamespace, 0)
	if err == nil {
		t.Fatal("Expected error for numberOfKeys=0")
	}

	if !contains(err.Error(), "numberOfKeys must be at least 1") {
		t.Errorf("Expected error about numberOfKeys, got: %v", err)
	}
}

func TestRotateSecret_SecretNotFound(t *testing.T) {
	k8sClient := getTestClient()
	ctx := context.Background()

	err := RotateSecret(ctx, k8sClient, "nonexistent-secret", testNamespace, 3)
	if err == nil {
		t.Fatal("Expected error for nonexistent secret")
	}

	if !contains(err.Error(), "failed to get secret") {
		t.Errorf("Expected error about getting secret, got: %v", err)
	}
}

func TestRotateSecret_MalformedKeysSkipped(t *testing.T) {
	ctx := context.Background()
	secretName := testSecretName

	// Create secret with mix of valid and malformed keys
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: testNamespace,
		},
		Data: map[string][]byte{
			"jwt-signing-key-1000":    []byte("validkey1"),
			"jwt-signing-key-2000":    []byte("validkey2"),
			"jwt-signing-key-invalid": []byte("malformed"),
			"jwt-signing-key-":        []byte("nots"),
			"other-key":               []byte("notakey"),
		},
	}
	k8sClient := getTestClient(secret)

	// Rotation should succeed and skip malformed keys
	err := RotateSecret(ctx, k8sClient, secretName, testNamespace, 3)
	if err != nil {
		t.Fatalf("RotateSecret should skip malformed keys, but failed: %v", err)
	}

	// Verify rotation happened
	updatedSecret := &corev1.Secret{}
	err = k8sClient.Get(ctx, types.NamespacedName{Name: secretName, Namespace: testNamespace}, updatedSecret)
	if err != nil {
		t.Fatalf("Failed to get updated secret: %v", err)
	}

	// Should have 3 valid keys (2 original + 1 new)
	validKeyCount := 0
	for name := range updatedSecret.Data {
		if hasPrefix(name, KeyPrefix) {
			_, err := ParseKeyTimestamp(name)
			if err == nil {
				validKeyCount++
			}
		}
	}

	if validKeyCount != 3 {
		t.Errorf("Expected 3 valid keys, got %d", validKeyCount)
	}
}

func TestValidateSecret(t *testing.T) {
	tests := []struct {
		name          string
		secretData    map[string][]byte
		expectError   bool
		errorContains string
	}{
		{
			name: "valid secret with keys",
			secretData: map[string][]byte{
				"jwt-signing-key-1000": []byte("key1"),
				"jwt-signing-key-2000": []byte("key2"),
			},
			expectError: false,
		},
		{
			name:          "secret with no data",
			secretData:    nil,
			expectError:   true,
			errorContains: "secret has no data",
		},
		{
			name: "secret with no valid keys",
			secretData: map[string][]byte{
				"other-key": []byte("notajwtkey"),
			},
			expectError:   true,
			errorContains: "secret has no valid JWT signing keys",
		},
		{
			name: "secret with invalid key format",
			secretData: map[string][]byte{
				"jwt-signing-key-invalid": []byte("badkey"),
			},
			expectError:   true,
			errorContains: "invalid key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			secretName := testSecretName

			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: testNamespace,
				},
				Data: tt.secretData,
			}
			k8sClient := getTestClient(secret)

			err := ValidateSecret(ctx, k8sClient, secretName, testNamespace)

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
			}
		})
	}
}

func TestGetLatestKeyID(t *testing.T) {
	tests := []struct {
		name          string
		secretData    map[string][]byte
		expectedKid   string
		expectError   bool
		errorContains string
	}{
		{
			name: "single key",
			secretData: map[string][]byte{
				"jwt-signing-key-1000": []byte("key1"),
			},
			expectedKid: "1000",
			expectError: false,
		},
		{
			name: "multiple keys - latest is last",
			secretData: map[string][]byte{
				"jwt-signing-key-1000": []byte("key1"),
				"jwt-signing-key-2000": []byte("key2"),
				"jwt-signing-key-3000": []byte("key3"),
			},
			expectedKid: "3000",
			expectError: false,
		},
		{
			name: "multiple keys - latest is first",
			secretData: map[string][]byte{
				"jwt-signing-key-5000": []byte("key5"),
				"jwt-signing-key-2000": []byte("key2"),
				"jwt-signing-key-1000": []byte("key1"),
			},
			expectedKid: "5000",
			expectError: false,
		},
		{
			name:          "no data",
			secretData:    nil,
			expectError:   true,
			errorContains: "secret has no data",
		},
		{
			name: "no valid keys",
			secretData: map[string][]byte{
				"other-key": []byte("notakey"),
			},
			expectError:   true,
			errorContains: "no valid JWT signing keys found",
		},
		{
			name: "mixed valid and invalid keys",
			secretData: map[string][]byte{
				"jwt-signing-key-1000":    []byte("key1"),
				"jwt-signing-key-invalid": []byte("badkey"),
				"jwt-signing-key-3000":    []byte("key3"),
			},
			expectedKid: "3000",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testSecretName,
					Namespace: testNamespace,
				},
				Data: tt.secretData,
			}

			kid, err := GetLatestKeyID(secret)

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
				if kid != tt.expectedKid {
					t.Errorf("Expected kid '%s', got '%s'", tt.expectedKid, kid)
				}
			}
		})
	}
}

func TestParseSigningKeys(t *testing.T) {
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
					Name:      testSecretName,
					Namespace: testNamespace,
				},
				Data: tt.secretData,
			}

			keys, kid, err := ParseSigningKeys(secret)

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
			key:      []byte("short"),
			expected: "c2hvcnQ=", // base64 of "short"
		},
		{
			name:     "long key - truncated",
			key:      []byte("this-is-a-very-long-key-that-should-be-truncated"),
			expected: "dGhpcy1pcy1hLXZl...", // First 16 chars + "..."
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

// Helper functions

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

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[0:len(prefix)] == prefix
}
