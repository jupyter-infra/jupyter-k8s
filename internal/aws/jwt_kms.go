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
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/authmiddleware"
)

// Error definitions
var (
	ErrInvalidClaims = errors.New("invalid token claims")
	ErrTokenExpired  = errors.New("token has expired")
)

// KMSJWTManager handles JWT token creation and validation using AWS KMS envelope encryption
type KMSJWTManager struct {
	kmsClient  *KMSClient
	keyId      string
	issuer     string
	audience   string
	expiration time.Duration
	keyCache   map[string][]byte // encrypted_key_hash -> plaintext_key
	cacheMutex sync.RWMutex
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
		kmsClient:  config.KMSClient,
		keyId:      config.KeyId,
		issuer:     config.Issuer,
		audience:   config.Audience,
		expiration: config.Expiration,
		keyCache:   make(map[string][]byte), // TODO implement eviction of old keys
	}
}

// GenerateToken creates a new JWT token using KMS envelope encryption
func (m *KMSJWTManager) GenerateToken(user string, groups []string, path string, domain string, tokenType string) (string, error) {
	ctx := context.Background()
	now := time.Now().UTC()

	// Generate data key for this token
	plaintextKey, encryptedKey, err := m.kmsClient.GenerateDataKey(ctx, m.keyId)
	if err != nil {
		log.Printf("KMS: Failed to generate data key: %v", err)
		return "", fmt.Errorf("failed to generate data key: %w", err)
	}

	// Create claims
	claims := &authmiddleware.Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			ExpiresAt: jwt5.NewNumericDate(now.Add(m.expiration)),
			IssuedAt:  jwt5.NewNumericDate(now),
			NotBefore: jwt5.NewNumericDate(now),
			Issuer:    m.issuer,
			Audience:  []string{m.audience},
			Subject:   user,
		},
		User:      user,
		Groups:    groups,
		Path:      path,
		Domain:    domain,
		TokenType: tokenType,
	}

	// TODO: Fix this weird mutation of the header - should use proper custom header struct
	// Create token with custom header containing encrypted data key
	token := jwt5.NewWithClaims(jwt5.SigningMethodHS256, claims)

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
	m.cacheMutex.Lock()
	m.keyCache[keyHash] = plaintextKey
	m.cacheMutex.Unlock()

	return tokenString, nil
}

// ValidateToken validates token using envelope decryption
func (m *KMSJWTManager) ValidateToken(tokenString string) (*authmiddleware.Claims, error) {
	ctx := context.Background()

	// Parse token to extract header with encrypted data key
	token, err := jwt5.ParseWithClaims(tokenString, &authmiddleware.Claims{}, func(token *jwt5.Token) (interface{}, error) {
		// Verify signing method
		if _, ok := token.Method.(*jwt5.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
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

		// Cache miss - decrypt with KMS
		plaintextKey, err = m.kmsClient.Decrypt(ctx, encryptedKey)
		if err != nil {
			log.Printf("KMS: Failed to decrypt data key: %v", err)
			return nil, fmt.Errorf("failed to decrypt data key: %w", err)
		}

		// Cache the decrypted key
		m.cacheMutex.Lock()
		m.keyCache[keyHash] = plaintextKey
		m.cacheMutex.Unlock()

		return plaintextKey, nil
	})

	if err != nil {
		log.Printf("KMS: Token validation failed: %v", err)
		return nil, err
	}

	claims, ok := token.Claims.(*authmiddleware.Claims)
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

// hashKey creates a hash of the encrypted key for cache indexing
func (m *KMSJWTManager) hashKey(encryptedKey []byte) string {
	hash := sha256.Sum256(encryptedKey)
	return base64.URLEncoding.EncodeToString(hash[:])
}
