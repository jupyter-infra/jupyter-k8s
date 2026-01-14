/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package rotator provides JWT signing key rotation functionality for Kubernetes secrets.
package rotator

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// KeyPrefix is the prefix for JWT signing keys in the secret
	KeyPrefix = "jwt-signing-key-"
	// KeySizeBytes is the size of generated signing keys in bytes (384 bits)
	// Must be at least 48 bytes for HS384 per RFC 7518 Section 3.2
	KeySizeBytes = 48
)

// GenerateKey generates a cryptographically random signing key
func GenerateKey() ([]byte, error) {
	key := make([]byte, KeySizeBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}

// BuildKeyName creates a key name with the given timestamp
func BuildKeyName(timestamp int64) string {
	return fmt.Sprintf("%s%d", KeyPrefix, timestamp)
}

// ParseKeyTimestamp extracts the timestamp from a key name
func ParseKeyTimestamp(keyName string) (int64, error) {
	if !strings.HasPrefix(keyName, KeyPrefix) {
		return 0, fmt.Errorf("key name %s does not have prefix %s", keyName, KeyPrefix)
	}

	timestampStr := strings.TrimPrefix(keyName, KeyPrefix)
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse timestamp from %s: %w", keyName, err)
	}

	return timestamp, nil
}

// keyEntry represents a signing key with its timestamp
type keyEntry struct {
	name      string
	timestamp int64
	value     []byte
}

// RotateSecret performs key rotation on a Kubernetes secret
// It generates a new key, adds it to the secret, and prunes old keys beyond numberOfKeys
func RotateSecret(ctx context.Context, k8sClient client.Client, secretName string, namespace string, numberOfKeys int) error {
	if numberOfKeys < 1 {
		return fmt.Errorf("numberOfKeys must be at least 1, got %d", numberOfKeys)
	}

	// Get current secret
	secret := &corev1.Secret{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, secret)
	if err != nil {
		return fmt.Errorf("failed to get secret %s: %w", secretName, err)
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}

	// Parse existing keys
	keys := make([]keyEntry, 0, len(secret.Data))
	for name, value := range secret.Data {
		if !strings.HasPrefix(name, KeyPrefix) {
			continue
		}

		timestamp, err := ParseKeyTimestamp(name)
		if err != nil {
			// Log warning but continue - don't fail rotation due to malformed key
			fmt.Printf("Warning: skipping malformed key %s: %v\n", name, err)
			continue
		}

		keys = append(keys, keyEntry{
			name:      name,
			timestamp: timestamp,
			value:     value,
		})
	}

	// Sort keys by timestamp (oldest first)
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].timestamp < keys[j].timestamp
	})

	// Generate new key
	newKey, err := GenerateKey()
	if err != nil {
		return fmt.Errorf("failed to generate new key: %w", err)
	}

	now := time.Now().UTC().Unix()
	newKeyName := BuildKeyName(now)

	// Check if key with this timestamp already exists (clock skew or very fast rotation)
	for _, k := range keys {
		if k.name == newKeyName {
			return fmt.Errorf("key with timestamp %d already exists, refusing to overwrite", now)
		}
	}

	// Add new key
	secret.Data[newKeyName] = newKey
	keys = append(keys, keyEntry{
		name:      newKeyName,
		timestamp: now,
		value:     newKey,
	})

	// Re-sort after adding new key
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].timestamp < keys[j].timestamp
	})

	// Keep only the latest numberOfKeys keys
	if len(keys) > numberOfKeys {
		keysToRemove := keys[:len(keys)-numberOfKeys]
		for _, k := range keysToRemove {
			delete(secret.Data, k.name)
		}
		fmt.Printf("Pruned %d old keys: %v\n", len(keysToRemove), getKeyNames(keysToRemove))
	}

	// Update secret
	err = k8sClient.Update(ctx, secret)
	if err != nil {
		return fmt.Errorf("failed to update secret %s: %w", secretName, err)
	}

	remainingKeys := len(secret.Data)
	fmt.Printf("Successfully rotated keys in secret %s/%s: added key %s, %d keys remaining\n",
		secret.Namespace, secretName, newKeyName, remainingKeys)

	return nil
}

// getKeyNames extracts key names from keyEntry slice for logging
func getKeyNames(keys []keyEntry) []string {
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = k.name
	}
	return names
}

// ValidateSecret checks if a secret has valid JWT signing keys
func ValidateSecret(ctx context.Context, k8sClient client.Client, secretName string, namespace string) error {
	secret := &corev1.Secret{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: namespace,
	}, secret)
	if err != nil {
		return fmt.Errorf("failed to get secret: %w", err)
	}

	if secret.Data == nil {
		return fmt.Errorf("secret has no data")
	}

	keyCount := 0
	for name := range secret.Data {
		if strings.HasPrefix(name, KeyPrefix) {
			_, err := ParseKeyTimestamp(name)
			if err != nil {
				return fmt.Errorf("invalid key %s: %w", name, err)
			}
			keyCount++
		}
	}

	if keyCount == 0 {
		return fmt.Errorf("secret has no valid JWT signing keys")
	}

	return nil
}

// GetLatestKeyID returns the kid (timestamp) of the most recent key in the secret
func GetLatestKeyID(secret *corev1.Secret) (string, error) {
	if secret.Data == nil {
		return "", fmt.Errorf("secret has no data")
	}

	var latestTimestamp int64
	var latestKid string

	for name := range secret.Data {
		if !strings.HasPrefix(name, KeyPrefix) {
			continue
		}

		timestamp, err := ParseKeyTimestamp(name)
		if err != nil {
			continue // Skip malformed keys
		}

		if timestamp > latestTimestamp {
			latestTimestamp = timestamp
			latestKid = strings.TrimPrefix(name, KeyPrefix)
		}
	}

	if latestKid == "" {
		return "", fmt.Errorf("no valid JWT signing keys found")
	}

	return latestKid, nil
}

// ParseSigningKeys extracts all JWT signing keys from a secret
func ParseSigningKeys(secret *corev1.Secret) (map[string][]byte, string, error) {
	if secret.Data == nil {
		return nil, "", fmt.Errorf("secret has no data")
	}

	signingKeys := make(map[string][]byte)
	var latestTimestamp int64
	var latestKid string

	for name, value := range secret.Data {
		if !strings.HasPrefix(name, KeyPrefix) {
			continue
		}

		timestamp, err := ParseKeyTimestamp(name)
		if err != nil {
			return nil, "", fmt.Errorf("invalid key format %s: %w", name, err)
		}

		kid := strings.TrimPrefix(name, KeyPrefix)
		signingKeys[kid] = value

		if timestamp > latestTimestamp {
			latestTimestamp = timestamp
			latestKid = kid
		}
	}

	if len(signingKeys) == 0 {
		return nil, "", fmt.Errorf("no signing keys found in secret")
	}

	return signingKeys, latestKid, nil
}

// FormatKeyForDisplay formats a key value for safe display (base64 encoded, truncated)
func FormatKeyForDisplay(key []byte) string {
	if len(key) == 0 {
		return "<empty>"
	}
	encoded := base64.StdEncoding.EncodeToString(key)
	if len(encoded) > 16 {
		return encoded[:16] + "..."
	}
	return encoded
}
