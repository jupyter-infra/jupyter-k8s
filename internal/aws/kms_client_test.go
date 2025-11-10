package aws

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
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
	plaintext, encrypted, err := client.GenerateDataKey(context.Background(), "test-key-id")
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
	expectedKeyAlias := "alias/sagemaker-devspace-key-arn:aws:eks:us-west-2:123456789012:cluster/test-cluster"
	if keyID != expectedKeyAlias {
		t.Errorf("Expected keyID %q, got %q", expectedKeyAlias, keyID)
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
	expectedKeyAlias := "alias/sagemaker-devspace-key-arn:aws:eks:us-west-2:123456789012:cluster/test-cluster"
	if keyID != expectedKeyAlias {
		t.Errorf("Expected keyID %q, got %q", expectedKeyAlias, keyID)
	}
}

func TestKMSClient_createAliasWithConflictResolution_Success(t *testing.T) {
	mockKMS := &MockKMSClient{}
	client := NewKMSWrapper(mockKMS, "us-west-2")

	keyID, err := client.createAliasWithConflictResolution(context.Background(), "alias/test-alias", "new-key-id")

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if keyID != "new-key-id" {
		t.Errorf("Expected keyID %q, got %q", "new-key-id", keyID)
	}
}

func TestKMSClient_createAliasWithConflictResolution_AlreadyExists(t *testing.T) {
	// Mock that returns AlreadyExistsException on CreateAlias
	mockKMS := &MockKMSClientWithAliasConflict{
		existingKeyID: "existing-key-id",
	}
	client := NewKMSWrapper(mockKMS, "us-west-2")

	keyID, err := client.createAliasWithConflictResolution(context.Background(), "alias/test-alias", "new-key-id")

	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if keyID != "existing-key-id" {
		t.Errorf("Expected existing keyID %q, got %q", "existing-key-id", keyID)
	}
	// Verify ScheduleKeyDeletion was called
	if !mockKMS.scheduleKeyDeletionCalled {
		t.Error("Expected ScheduleKeyDeletion to be called")
	}
	if mockKMS.scheduledKeyID != "new-key-id" {
		t.Errorf("Expected scheduled key ID %q, got %q", "new-key-id", mockKMS.scheduledKeyID)
	}
}

func TestKMSClient_createAliasWithConflictResolution_CreateAliasError(t *testing.T) {
	// Mock that returns a generic error on CreateAlias
	mockKMS := &MockKMSClientWithError{
		createAliasError: aws.String("AccessDenied: User is not authorized"),
	}
	client := NewKMSWrapper(mockKMS, "us-west-2")

	keyID, err := client.createAliasWithConflictResolution(context.Background(), "alias/test-alias", "new-key-id")

	if err == nil {
		t.Error("Expected error, got nil")
	}
	if keyID != "" {
		t.Errorf("Expected empty keyID, got %q", keyID)
	}
	if err.Error() != "failed to create KMS key alias: AccessDenied: User is not authorized" {
		t.Errorf("Expected specific error message, got %q", err.Error())
	}
}

// MockKMSClientWithError simulates CreateAlias returning a generic error
type MockKMSClientWithError struct {
	createAliasError *string
}

func (m *MockKMSClientWithError) GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return &kms.GenerateDataKeyOutput{}, nil
}

func (m *MockKMSClientWithError) Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	return &kms.DecryptOutput{}, nil
}

func (m *MockKMSClientWithError) CreateKey(ctx context.Context, params *kms.CreateKeyInput, optFns ...func(*kms.Options)) (*kms.CreateKeyOutput, error) {
	return &kms.CreateKeyOutput{}, nil
}

func (m *MockKMSClientWithError) CreateAlias(ctx context.Context, params *kms.CreateAliasInput, optFns ...func(*kms.Options)) (*kms.CreateAliasOutput, error) {
	if m.createAliasError != nil {
		return nil, errors.New(*m.createAliasError)
	}
	return &kms.CreateAliasOutput{}, nil
}

func (m *MockKMSClientWithError) DescribeKey(ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options)) (*kms.DescribeKeyOutput, error) {
	return &kms.DescribeKeyOutput{}, nil
}

func (m *MockKMSClientWithError) ScheduleKeyDeletion(ctx context.Context, params *kms.ScheduleKeyDeletionInput, optFns ...func(*kms.Options)) (*kms.ScheduleKeyDeletionOutput, error) {
	return &kms.ScheduleKeyDeletionOutput{}, nil
}

// MockKMSClientWithAliasConflict simulates alias already exists scenario
type MockKMSClientWithAliasConflict struct {
	existingKeyID             string
	scheduleKeyDeletionCalled bool
	scheduledKeyID            string
}

func (m *MockKMSClientWithAliasConflict) GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return &kms.GenerateDataKeyOutput{}, nil
}

func (m *MockKMSClientWithAliasConflict) Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	return &kms.DecryptOutput{}, nil
}

func (m *MockKMSClientWithAliasConflict) CreateKey(ctx context.Context, params *kms.CreateKeyInput, optFns ...func(*kms.Options)) (*kms.CreateKeyOutput, error) {
	return &kms.CreateKeyOutput{}, nil
}

func (m *MockKMSClientWithAliasConflict) CreateAlias(ctx context.Context, params *kms.CreateAliasInput, optFns ...func(*kms.Options)) (*kms.CreateAliasOutput, error) {
	return nil, &types.AlreadyExistsException{
		Message: aws.String("An alias with the name already exists"),
	}
}

func (m *MockKMSClientWithAliasConflict) DescribeKey(ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options)) (*kms.DescribeKeyOutput, error) {
	return &kms.DescribeKeyOutput{
		KeyMetadata: &types.KeyMetadata{
			KeyId: aws.String(m.existingKeyID),
		},
	}, nil
}

func (m *MockKMSClientWithAliasConflict) ScheduleKeyDeletion(ctx context.Context, params *kms.ScheduleKeyDeletionInput, optFns ...func(*kms.Options)) (*kms.ScheduleKeyDeletionOutput, error) {
	m.scheduleKeyDeletionCalled = true
	m.scheduledKeyID = *params.KeyId
	return &kms.ScheduleKeyDeletionOutput{}, nil
}
