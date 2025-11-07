package aws

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	jwt5 "github.com/golang-jwt/jwt/v5"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/jwt"
)

// Error definitions
var (
	ErrInvalidClaims = jwt.ErrInvalidClaims
	ErrTokenExpired  = jwt.ErrTokenExpired
)

// KMSJWTManager handles JWT token creation and validation using AWS KMS envelope encryption
type KMSJWTManager struct {
	kmsClient   *KMSClient
	keyId       string
	issuer      string
	audience    string
	expiration  time.Duration
	keyCache    map[string][]byte    // encrypted_key_hash -> plaintext_key
	cacheExpiry map[string]time.Time // encrypted_key_hash -> expiry_time
	lastCleanup time.Time
	cacheMutex  sync.RWMutex
}

// KMSJWTConfig contains configuration for KMS JWT manager
type KMSJWTConfig struct {
	KMSClient  *KMSClient
	KeyId      string
	Issuer     string
	Audience   string
	Expiration time.Duration
}

// NewKMSJWTManager creates a new KMSJWTManager
func NewKMSJWTManager(config KMSJWTConfig) *KMSJWTManager {
	return &KMSJWTManager{
		kmsClient:   config.KMSClient,
		keyId:       config.KeyId,
		issuer:      config.Issuer,
		audience:    config.Audience,
		expiration:  config.Expiration,
		keyCache:    make(map[string][]byte),
		cacheExpiry: make(map[string]time.Time),
		lastCleanup: time.Now(),
	}
}

// GenerateToken creates a new JWT token using KMS envelope encryption
func (m *KMSJWTManager) GenerateToken(
	user string,
	groups []string,
	uid string,
	extra map[string][]string,
	path string,
	domain string,
	tokenType string,
) (string, error) {
	ctx := context.Background()
	now := time.Now().UTC()

	// Generate data key for this token
	plaintextKey, encryptedKey, err := m.kmsClient.GenerateDataKey(ctx, m.keyId)
	if err != nil {
		log.Printf("KMS: Failed to generate data key: %v", err)
		return "", fmt.Errorf("failed to generate data key: %w", err)
	}

	// Create claims
	claims := &jwt.Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(m.expiration)),
			IssuedAt:  jwt5.NewNumericDate(now),
			NotBefore: jwt5.NewNumericDate(now),
			Issuer:    m.issuer,
			Audience:  []string{m.audience},
			Subject:   user,
		},
		User:      user,
		CapsUser:  user,
		Groups:    groups,
		UID:       uid,
		Extra:     extra,
		Path:      path,
		Domain:    domain,
		TokenType: tokenType,
	}

	// TODO: Fix this weird mutation of the header - should use proper custom header struct
	// Create token with custom header containing encrypted data key
	token := jwt5.NewWithClaims(jwt5.SigningMethodHS384, claims)

	// Add encrypted data key to header (temporary approach)
	token.Header["edk"] = base64.URLEncoding.EncodeToString(encryptedKey)

	// Sign with plaintext data key
	tokenString, err := token.SignedString(plaintextKey)
	if err != nil {
		log.Printf("KMS: Failed to sign token: %v", err)
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	// Cache the plaintext key
	keyHash := m.hashKey(encryptedKey)
	m.setCachedKey(keyHash, plaintextKey)

	return tokenString, nil
}

// ValidateToken validates token using envelope decryption
func (m *KMSJWTManager) ValidateToken(tokenString string) (*jwt.Claims, error) {
	ctx := context.Background()

	// Parse token to extract header with encrypted data key
	token, err := jwt5.ParseWithClaims(tokenString, &jwt.Claims{}, func(token *jwt5.Token) (interface{}, error) {
		// Verify signing method
		if token.Method != jwt5.SigningMethodHS384 {
			return nil, fmt.Errorf("unexpected signing method: %v, expected HS384", token.Header["alg"])
		}

		// Extract encrypted data key from header
		edkStr, ok := token.Header["edk"].(string)
		if !ok {
			return nil, errors.New("missing encrypted data key in header")
		}

		encryptedKey, err := base64.URLEncoding.DecodeString(edkStr)
		if err != nil {
			return nil, fmt.Errorf("invalid encrypted data key: %w", err)
		}

		// Try cache first
		keyHash := m.hashKey(encryptedKey)
		m.cacheMutex.RLock()
		plaintextKey, cached := m.keyCache[keyHash]
		m.cacheMutex.RUnlock()

		if cached {
			return plaintextKey, nil
		}

		// Periodic cleanup of expired entries
		m.cleanupExpiredKeys()

		// Cache miss - decrypt with KMS
		plaintextKey, err = m.kmsClient.Decrypt(ctx, encryptedKey)
		if err != nil {
			log.Printf("KMS: Failed to decrypt data key: %v", err)
			return nil, fmt.Errorf("failed to decrypt data key: %w", err)
		}

		// Cache the decrypted key
		m.setCachedKey(keyHash, plaintextKey)

		return plaintextKey, nil
	})

	if err != nil {
		log.Printf("KMS: Token validation failed: %v", err)
		return nil, err
	}

	claims, ok := token.Claims.(*jwt.Claims)
	if !ok {
		return nil, ErrInvalidClaims
	}

	// Validate standard claims manually
	now := time.Now().UTC()
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(now) {
		return nil, ErrTokenExpired
	}

	return claims, nil
}

// setCachedKey stores a key in cache with TTL
func (m *KMSJWTManager) setCachedKey(keyHash string, plaintextKey []byte) {
	m.cacheMutex.Lock()
	m.keyCache[keyHash] = plaintextKey
	m.cacheExpiry[keyHash] = time.Now().Add(m.expiration * 2) // Buffer: 2x JWT expiration
	m.cacheMutex.Unlock()
}

// cleanupExpiredKeys removes all expired entries from cache
func (m *KMSJWTManager) cleanupExpiredKeys() {
	if time.Since(m.lastCleanup) <= 15*time.Minute {
		return
	}

	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()

	// Re-check after acquiring lock to prevent double cleanup
	if time.Since(m.lastCleanup) <= 15*time.Minute {
		return
	}

	now := time.Now()
	for hash, expiry := range m.cacheExpiry {
		if now.After(expiry) {
			delete(m.keyCache, hash)
			delete(m.cacheExpiry, hash)
		}
	}
	m.lastCleanup = now
}

// hashKey creates a hash of the encrypted key for cache indexing
func (m *KMSJWTManager) hashKey(encryptedKey []byte) string {
	hash := sha256.Sum256(encryptedKey)
	return base64.URLEncoding.EncodeToString(hash[:])
}
