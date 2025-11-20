/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package aws

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	jwt5 "github.com/golang-jwt/jwt/v5"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
)

// MockKMSClient implements a mock KMS client for testing
type MockKMSClient struct {
	dataKey          []byte
	encryptedKey     []byte
	decryptCalled    bool
	decryptCallCount int
	decryptFunc      func(ctx context.Context, encryptedKey []byte) ([]byte, error)
}

func (m *MockKMSClient) GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return &kms.GenerateDataKeyOutput{
		Plaintext:      m.dataKey,
		CiphertextBlob: m.encryptedKey,
		KeyId:          params.KeyId,
	}, nil
}

func (m *MockKMSClient) Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	m.decryptCalled = true
	m.decryptCallCount++

	if m.decryptFunc != nil {
		plaintext, err := m.decryptFunc(ctx, params.CiphertextBlob)
		if err != nil {
			return nil, err
		}
		return &kms.DecryptOutput{
			Plaintext: plaintext,
			KeyId:     aws.String("test-key-id"),
		}, nil
	}

	return &kms.DecryptOutput{
		Plaintext: m.dataKey,
		KeyId:     aws.String("test-key-id"),
	}, nil
}

func (m *MockKMSClient) DescribeKey(ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options)) (*kms.DescribeKeyOutput, error) {
	return &kms.DescribeKeyOutput{
		KeyMetadata: &types.KeyMetadata{
			KeyId: aws.String("test-key-id"),
		},
	}, nil
}

func TestNewKMSJWTManager(t *testing.T) {
	mockKMS := &MockKMSClient{}
	kmsClient := NewKMSWrapper(mockKMS, "us-east-1")

	config := KMSJWTConfig{
		KMSClient:  kmsClient,
		KeyId:      "test-key-id",
		Issuer:     "test-issuer",
		Audience:   "test-audience",
		Expiration: 30 * time.Minute,
	}

	manager := NewKMSJWTManager(config)

	if manager.keyId != "test-key-id" {
		t.Errorf("Expected keyId %q, got %q", "test-key-id", manager.keyId)
	}

	if manager.issuer != "test-issuer" {
		t.Errorf("Expected issuer %q, got %q", "test-issuer", manager.issuer)
	}

	if manager.audience != "test-audience" {
		t.Errorf("Expected audience %q, got %q", "test-audience", manager.audience)
	}

	if manager.expiration != 30*time.Minute {
		t.Errorf("Expected expiration %v, got %v", 30*time.Minute, manager.expiration)
	}

	if manager.keyCache == nil {
		t.Error("Expected keyCache to be initialized")
	}
}

func TestKMSJWTManager_EnvelopeEncryption(t *testing.T) {
	// Create mock KMS client
	mockKMS := &MockKMSClient{
		dataKey:      []byte("test-data-key-32-bytes-long-key"),
		encryptedKey: []byte("encrypted-data-key-blob"),
	}

	// Create KMS JWT manager
	manager := &KMSJWTManager{
		kmsClient:   NewKMSWrapper(mockKMS, "us-east-1"),
		keyId:       "test-key-id",
		issuer:      "test-issuer",
		audience:    "test-audience",
		expiration:  30 * time.Minute,
		keyCache:    make(map[string][]byte),
		cacheExpiry: make(map[string]time.Time),
		lastCleanup: time.Now(),
	}

	// Test data
	user := "test-user"
	groups := []string{"users", "admins"}
	path := "/workspaces/test-ns/test-workspace"
	domain := "example.com"
	tokenType := "access"

	// Generate token
	token, err := manager.GenerateToken(user, groups, "uid123", nil, path, domain, tokenType)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	if token == "" {
		t.Fatal("Generated token is empty")
	}

	// Validate token (should use cache, not call KMS decrypt)
	mockKMS.decryptCalled = false
	claims, err := manager.ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	// Verify KMS decrypt was not called (cache hit)
	if mockKMS.decryptCalled {
		t.Error("Expected cache hit, but KMS decrypt was called")
	}

	// Verify claims
	if claims.User != user {
		t.Errorf("Expected user %q, got %q", user, claims.User)
	}

	if len(claims.Groups) != len(groups) {
		t.Errorf("Expected %d groups, got %d", len(groups), len(claims.Groups))
	}

	if claims.Path != path {
		t.Errorf("Expected path %q, got %q", path, claims.Path)
	}

	if claims.Domain != domain {
		t.Errorf("Expected domain %q, got %q", domain, claims.Domain)
	}

	if claims.TokenType != tokenType {
		t.Errorf("Expected token type %q, got %q", tokenType, claims.TokenType)
	}

	if claims.Issuer != "test-issuer" {
		t.Errorf("Expected issuer %q, got %q", "test-issuer", claims.Issuer)
	}

	if len(claims.Audience) != 1 || claims.Audience[0] != "test-audience" {
		t.Errorf("Expected audience [%q], got %v", "test-audience", claims.Audience)
	}
}

func TestKMSJWTManager_CacheMiss(t *testing.T) {
	// Create mock KMS client
	mockKMS := &MockKMSClient{
		dataKey:      []byte("test-data-key-32-bytes-long-key"),
		encryptedKey: []byte("encrypted-data-key-blob"),
	}

	// Create KMS JWT manager
	manager := &KMSJWTManager{
		kmsClient:   NewKMSWrapper(mockKMS, "us-east-1"),
		keyId:       "test-key-id",
		issuer:      "test-issuer",
		audience:    "test-audience",
		expiration:  30 * time.Minute,
		keyCache:    make(map[string][]byte),
		cacheExpiry: make(map[string]time.Time),
		lastCleanup: time.Now(),
	}

	// Generate token
	token, err := manager.GenerateToken("user", []string{"group"}, "uid123", nil, "/path", "domain", "type")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Clear cache to force KMS decrypt call
	manager.keyCache = make(map[string][]byte)
	manager.cacheExpiry = make(map[string]time.Time)
	mockKMS.decryptCalled = false

	// Validate token (should call KMS decrypt due to cache miss)
	_, err = manager.ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	// Verify KMS decrypt was called (cache miss)
	if !mockKMS.decryptCalled {
		t.Error("Expected KMS decrypt to be called on cache miss")
	}
}

func TestKMSJWTManager_CacheExpiry(t *testing.T) {
	manager := &KMSJWTManager{
		keyCache:    make(map[string][]byte),
		cacheExpiry: make(map[string]time.Time),
		lastCleanup: time.Now(),
		expiration:  1 * time.Millisecond, // Very short for testing
	}

	// Manually add an entry that will expire soon
	keyHash := "test-hash"
	manager.keyCache[keyHash] = []byte("test-key")
	manager.cacheExpiry[keyHash] = time.Now().Add(2 * time.Millisecond) // Short expiry for test

	// Verify key is cached
	if len(manager.keyCache) != 1 {
		t.Errorf("Expected 1 cached key, got %d", len(manager.keyCache))
	}

	// Wait for expiry and force cleanup
	time.Sleep(5 * time.Millisecond)
	manager.lastCleanup = time.Time{} // Force cleanup on next call
	manager.cleanupExpiredKeys()

	// Verify expired entries are cleaned up
	if len(manager.keyCache) != 0 {
		t.Errorf("Expected 0 cached keys after cleanup, got %d", len(manager.keyCache))
	}
}

func TestKMSJWTManager_CacheEviction(t *testing.T) {
	manager := &KMSJWTManager{
		keyCache:       make(map[string][]byte),
		cacheExpiry:    make(map[string]time.Time),
		lastCleanup:    time.Now(),
		maxCacheSize:   3, // Small cache for testing
		evictBatchSize: 2, // Evict 2 at a time
	}

	now := time.Now()

	// Add 3 keys with different expiry times (oldest first)
	manager.keyCache["key1"] = []byte("value1")
	manager.cacheExpiry["key1"] = now.Add(1 * time.Hour) // Expires first

	manager.keyCache["key2"] = []byte("value2")
	manager.cacheExpiry["key2"] = now.Add(2 * time.Hour) // Expires second

	manager.keyCache["key3"] = []byte("value3")
	manager.cacheExpiry["key3"] = now.Add(3 * time.Hour) // Expires last

	// Verify cache is at capacity
	if len(manager.keyCache) != 3 {
		t.Errorf("Expected 3 cached keys, got %d", len(manager.keyCache))
	}

	// Add 4th key - should trigger eviction of 2 oldest
	manager.setCachedKey("key4", []byte("value4"))

	// Should have 2 keys remaining (key3 + key4)
	if len(manager.keyCache) != 2 {
		t.Errorf("Expected 2 cached keys after eviction, got %d", len(manager.keyCache))
	}

	// Verify oldest keys (key1, key2) were evicted, newest (key3, key4) remain
	if _, exists := manager.keyCache["key1"]; exists {
		t.Error("Expected key1 to be evicted")
	}
	if _, exists := manager.keyCache["key2"]; exists {
		t.Error("Expected key2 to be evicted")
	}
	if _, exists := manager.keyCache["key3"]; !exists {
		t.Error("Expected key3 to remain")
	}
	if _, exists := manager.keyCache["key4"]; !exists {
		t.Error("Expected key4 to remain")
	}
}

func TestKMSJWTManager_ConfigDefaults(t *testing.T) {
	config := KMSJWTConfig{
		KMSClient:  &KMSClient{},
		KeyId:      "test-key",
		Issuer:     "test-issuer",
		Audience:   "test-audience",
		Expiration: time.Hour,
		// MaxCacheSize and EvictBatchSize left as 0 to test defaults
	}

	manager := NewKMSJWTManager(config)

	if manager.maxCacheSize != defaultMaxCacheSize {
		t.Errorf("Expected maxCacheSize to be %d, got %d", defaultMaxCacheSize, manager.maxCacheSize)
	}

	if manager.evictBatchSize != defaultEvictBatchSize {
		t.Errorf("Expected evictBatchSize to be %d, got %d", defaultEvictBatchSize, manager.evictBatchSize)
	}
}

func TestKMSJWTManager_CacheHit(t *testing.T) {
	mockKMS := &MockKMSClient{
		dataKey:      []byte("plaintext-key"),
		encryptedKey: []byte("encrypted-key"),
	}

	kmsClient := &KMSClient{
		client: mockKMS,
		region: "us-east-1",
	}

	manager := NewKMSJWTManager(KMSJWTConfig{
		KMSClient:  kmsClient,
		KeyId:      "test-key",
		Issuer:     "test-issuer",
		Audience:   "test-audience",
		Expiration: time.Hour,
	})

	// Generate token (should call KMS once)
	token, err := manager.GenerateToken("user", []string{"group"}, "uid", nil, "/path", "domain", "bearer")
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	// Reset decrypt call count
	mockKMS.decryptCallCount = 0

	// Validate same token (should hit cache, no KMS decrypt call)
	_, err = manager.ValidateToken(token)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	// Verify KMS decrypt was not called (cache hit)
	if mockKMS.decryptCallCount != 0 {
		t.Errorf("Expected 0 KMS decrypt calls (cache hit), got %d", mockKMS.decryptCallCount)
	}
}

func TestKMSJWTManager_NoEvictionWhenUnderLimit(t *testing.T) {
	manager := &KMSJWTManager{
		keyCache:       make(map[string][]byte),
		cacheExpiry:    make(map[string]time.Time),
		lastCleanup:    time.Now(),
		maxCacheSize:   5, // Larger than what we'll add
		evictBatchSize: 2,
	}

	// Add 3 keys (under the limit of 5)
	manager.setCachedKey("key1", []byte("value1"))
	manager.setCachedKey("key2", []byte("value2"))
	manager.setCachedKey("key3", []byte("value3"))

	// Verify all keys remain
	if len(manager.keyCache) != 3 {
		t.Errorf("Expected 3 cached keys, got %d", len(manager.keyCache))
	}

	// Verify specific keys exist
	for _, key := range []string{"key1", "key2", "key3"} {
		if _, exists := manager.keyCache[key]; !exists {
			t.Errorf("Expected key %s to exist", key)
		}
	}
}

func TestKMSJWTManager_CleanupTiming(t *testing.T) {
	manager := &KMSJWTManager{
		keyCache:    make(map[string][]byte),
		cacheExpiry: make(map[string]time.Time),
		lastCleanup: time.Now(),
		expiration:  30 * time.Minute,
	}

	// Add some expired entries
	manager.keyCache["key1"] = []byte("value1")
	manager.cacheExpiry["key1"] = time.Now().Add(-1 * time.Hour) // Expired

	// Cleanup should not run (too recent)
	manager.cleanupExpiredKeys()
	if len(manager.keyCache) != 1 {
		t.Error("Cleanup should not have run yet")
	}

	// Force cleanup by setting old lastCleanup
	manager.lastCleanup = time.Now().Add(-20 * time.Minute)
	manager.cleanupExpiredKeys()

	if len(manager.keyCache) != 0 {
		t.Error("Cleanup should have removed expired entries")
	}
}

func TestKMSJWTManager_RejectsWrongSigningMethod(t *testing.T) {
	mockKMS := &MockKMSClient{
		dataKey:      []byte("test-data-key-32-bytes-long-key"),
		encryptedKey: []byte("encrypted-data-key-blob"),
	}

	manager := &KMSJWTManager{
		kmsClient:   NewKMSWrapper(mockKMS, "us-east-1"),
		keyId:       "test-key-id",
		issuer:      "test-issuer",
		audience:    "test-audience",
		expiration:  30 * time.Minute,
		keyCache:    make(map[string][]byte),
		cacheExpiry: make(map[string]time.Time),
		lastCleanup: time.Now(),
	}

	// Create a token with HS256 (wrong algorithm)
	claims := &jwt.Claims{
		RegisteredClaims: jwt5.RegisteredClaims{
			Subject:  "test-user",
			Issuer:   "test-issuer",
			Audience: []string{"test-audience"},
		},
	}

	token := jwt5.NewWithClaims(jwt5.SigningMethodHS256, claims)
	token.Header["edk"] = base64.URLEncoding.EncodeToString(mockKMS.encryptedKey)

	maliciousToken, err := token.SignedString(mockKMS.dataKey)
	if err != nil {
		t.Fatalf("Failed to create malicious token: %v", err)
	}

	// Should reject token with wrong signing method
	_, err = manager.ValidateToken(maliciousToken)
	if err == nil {
		t.Fatal("Expected validation to fail for wrong signing method")
	}

	expectedError := "token is unverifiable: error while executing keyfunc: unexpected signing method: HS256, expected HS384"
	if err.Error() != expectedError {
		t.Errorf("Expected error %q, got %q", expectedError, err.Error())
	}
}
