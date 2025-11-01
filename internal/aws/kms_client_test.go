package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/kms/types"
)

func TestNewKMSWrapper(t *testing.T) {
	mockKMS := &MockKMSClient{}
	client := NewKMSWrapper(mockKMS, "us-west-2")

	if client.region != "us-west-2" {
		t.Errorf("Expected region %q, got %q", "us-west-2", client.region)
	}

	if client.keySpec != types.DataKeySpecAes256 {
		t.Errorf("Expected keySpec %v, got %v", types.DataKeySpecAes256, client.keySpec)
	}
}

func TestKMSClient_GenerateDataKey(t *testing.T) {
	// Create mock KMS client
	mockKMS := &MockKMSClient{
		dataKey:      []byte("test-plaintext-key-32-bytes-long"),
		encryptedKey: []byte("encrypted-data-key-blob"),
	}

	// Create KMS client with mock
	client := NewKMSWrapper(mockKMS, "us-east-1")

	// Test GenerateDataKey
	plaintext, encrypted, err := client.GenerateDataKey(context.Background(), testKeyID)
	if err != nil {
		t.Fatalf("GenerateDataKey failed: %v", err)
	}

	if string(plaintext) != "test-plaintext-key-32-bytes-long" {
		t.Errorf("Expected plaintext key %q, got %q", "test-plaintext-key-32-bytes-long", string(plaintext))
	}

	if string(encrypted) != "encrypted-data-key-blob" {
		t.Errorf("Expected encrypted key %q, got %q", "encrypted-data-key-blob", string(encrypted))
	}
}

func TestKMSClient_Decrypt(t *testing.T) {
	// Create mock KMS client
	mockKMS := &MockKMSClient{
		dataKey: []byte("decrypted-plaintext-key"),
	}

	// Create KMS client with mock
	client := NewKMSWrapper(mockKMS, "us-east-1")

	// Test Decrypt
	plaintext, err := client.Decrypt(context.Background(), []byte("some-encrypted-blob"))
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if string(plaintext) != "decrypted-plaintext-key" {
		t.Errorf("Expected decrypted key %q, got %q", "decrypted-plaintext-key", string(plaintext))
	}
}

func TestKMSClient_GetRegion(t *testing.T) {
	// Create KMS client with mock
	client := NewKMSWrapper(&MockKMSClient{}, "us-west-2")

	region := client.GetRegion()
	if region != "us-west-2" {
		t.Errorf("Expected region %q, got %q", "us-west-2", region)
	}
}

func TestKMSClient_CreateJWTKMSKey_Success(t *testing.T) {
	mockKMS := &MockKMSClient{}
	client := NewKMSWrapper(mockKMS, "us-west-2")

	t.Setenv(EKSClusterARNEnv, "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	keyID, err := client.CreateJWTKMSKey(context.Background())

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if keyID != KMSJWTKeyAlias {
		t.Errorf("Expected keyID %q, got %q", KMSJWTKeyAlias, keyID)
	}
}

func TestKMSClient_CreateJWTKMSKey_KeyExists(t *testing.T) {
	mockKMS := &MockKMSClient{}
	client := NewKMSWrapper(mockKMS, "us-west-2")

	t.Setenv(EKSClusterARNEnv, "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	keyID, err := client.CreateJWTKMSKey(context.Background())

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if keyID != KMSJWTKeyAlias {
		t.Errorf("Expected keyID %q, got %q", KMSJWTKeyAlias, keyID)
	}
}
