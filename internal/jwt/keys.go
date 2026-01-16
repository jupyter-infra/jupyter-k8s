/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package jwt

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

const (
	// KeyPrefix is the prefix for JWT signing keys in the secret
	KeyPrefix = "jwt-signing-key-"
	// KeySizeBytes is the size of generated signing keys in bytes (384 bits)
	// Must be at least 48 bytes for HS384 per RFC 7518 Section 3.2
	KeySizeBytes = 48
)

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

// ParseSigningKeysFromSecret extracts all JWT signing keys from a secret
// Returns a map of kid->key, the latest kid, and any error
func ParseSigningKeysFromSecret(secret *corev1.Secret) (map[string][]byte, string, error) {
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
