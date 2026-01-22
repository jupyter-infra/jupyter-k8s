/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

// Package rotator provides JWT signing key rotation functionality for Kubernetes secrets.
package rotator

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GenerateKey generates a cryptographically random signing key
func GenerateKey() ([]byte, error) {
	key := make([]byte, jwt.KeySizeBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
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
		if !strings.HasPrefix(name, jwt.KeyPrefix) {
			continue
		}

		timestamp, err := jwt.ParseKeyTimestamp(name)
		if err != nil {
			// Log warning but continue - don't fail rotation due to malformed key
			log.Printf("Warning: skipping malformed key %s: %v\n", name, err)
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
	newKeyName := jwt.BuildKeyName(now)

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
		log.Printf("Pruned %d old keys: %v\n", len(keysToRemove), getKeyNames(keysToRemove))
	}

	// Update secret
	err = k8sClient.Update(ctx, secret)
	if err != nil {
		return fmt.Errorf("failed to update secret %s: %w", secretName, err)
	}

	remainingKeys := len(secret.Data)
	log.Printf("Successfully rotated keys in secret %s/%s: added key %s, %d keys remaining\n",
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
		if strings.HasPrefix(name, jwt.KeyPrefix) {
			_, err := jwt.ParseKeyTimestamp(name)
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
		if !strings.HasPrefix(name, jwt.KeyPrefix) {
			continue
		}

		timestamp, err := jwt.ParseKeyTimestamp(name)
		if err != nil {
			continue // Skip malformed keys
		}

		if timestamp > latestTimestamp {
			latestTimestamp = timestamp
			latestKid = strings.TrimPrefix(name, jwt.KeyPrefix)
		}
	}

	if latestKid == "" {
		return "", fmt.Errorf("no valid JWT signing keys found")
	}

	return latestKid, nil
}
