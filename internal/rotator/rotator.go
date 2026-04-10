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

// SecretConfig defines the configuration for rotating a single secret.
type SecretConfig struct {
	SecretName   string `json:"secretName"`
	KeyPrefix    string `json:"keyPrefix"`
	KeySize      int    `json:"keySize"`
	NumberOfKeys int    `json:"numberOfKeys"`
}

// DefaultJWTConfig returns a SecretConfig with JWT defaults for single-secret mode.
func DefaultJWTConfig(secretName string, numberOfKeys int) SecretConfig {
	return SecretConfig{
		SecretName:   secretName,
		KeyPrefix:    jwt.KeyPrefix,
		KeySize:      jwt.KeySizeBytes,
		NumberOfKeys: numberOfKeys,
	}
}

// GenerateKey generates a cryptographically random signing key of the default size.
func GenerateKey() ([]byte, error) {
	return GenerateKeyWithSize(jwt.KeySizeBytes)
}

// GenerateKeyWithSize generates a cryptographically random key of the specified size.
// Minimum key size is 16 bytes (128 bits).
func GenerateKeyWithSize(size int) ([]byte, error) {
	if size < 16 {
		return nil, fmt.Errorf("key size must be at least 16 bytes, got %d", size)
	}
	key := make([]byte, size)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}

// RotateSecrets performs key rotation on multiple Kubernetes secrets.
// Each secret is configured independently with its own key prefix, size, and retention count.
func RotateSecrets(ctx context.Context, k8sClient client.Client, namespace string, configs []SecretConfig) error {
	for _, cfg := range configs {
		log.Printf("Rotating secret %s (prefix=%s, keySize=%d, numberOfKeys=%d)",
			cfg.SecretName, cfg.KeyPrefix, cfg.KeySize, cfg.NumberOfKeys)
		if err := RotateSecret(ctx, k8sClient, namespace, cfg); err != nil {
			return fmt.Errorf("failed to rotate secret %s: %w", cfg.SecretName, err)
		}
	}
	return nil
}

// keyEntry represents a signing key with its timestamp
type keyEntry struct {
	name      string
	timestamp int64
	value     []byte
}

// RotateSecret performs key rotation on a single Kubernetes secret.
func RotateSecret(ctx context.Context, k8sClient client.Client, namespace string, cfg SecretConfig) error {
	secretName, keyPrefix, keySize, numberOfKeys := cfg.SecretName, cfg.KeyPrefix, cfg.KeySize, cfg.NumberOfKeys

	if numberOfKeys < 1 {
		return fmt.Errorf("numberOfKeys must be at least 1, got %d", numberOfKeys)
	}

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

	// Parse existing keys matching the prefix
	keys := make([]keyEntry, 0, len(secret.Data))
	for name, value := range secret.Data {
		if !strings.HasPrefix(name, keyPrefix) {
			continue
		}

		timestamp, err := jwt.ParseKeyTimestampWithPrefix(name, keyPrefix)
		if err != nil {
			log.Printf("Warning: skipping malformed key %s: %v\n", name, err)
			continue
		}

		keys = append(keys, keyEntry{
			name:      name,
			timestamp: timestamp,
			value:     value,
		})
	}

	sort.Slice(keys, func(i, j int) bool {
		return keys[i].timestamp < keys[j].timestamp
	})

	newKey, err := GenerateKeyWithSize(keySize)
	if err != nil {
		return fmt.Errorf("failed to generate new key: %w", err)
	}

	now := time.Now().UTC().Unix()
	newKeyName := fmt.Sprintf("%s%d", keyPrefix, now)

	for _, k := range keys {
		if k.name == newKeyName {
			return fmt.Errorf("key with timestamp %d already exists, refusing to overwrite", now)
		}
	}

	secret.Data[newKeyName] = newKey
	keys = append(keys, keyEntry{
		name:      newKeyName,
		timestamp: now,
		value:     newKey,
	})

	sort.Slice(keys, func(i, j int) bool {
		return keys[i].timestamp < keys[j].timestamp
	})

	if len(keys) > numberOfKeys {
		keysToRemove := keys[:len(keys)-numberOfKeys]
		for _, k := range keysToRemove {
			delete(secret.Data, k.name)
		}
		log.Printf("Pruned %d old keys: %v\n", len(keysToRemove), getKeyNames(keysToRemove))
	}

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
