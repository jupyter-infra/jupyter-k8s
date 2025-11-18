package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/aws/aws-sdk-go-v2/service/kms/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// KMSClientInterface defines the interface for KMS operations we need
type KMSClientInterface interface {
	GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error)
	Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error)
	DescribeKey(ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options)) (*kms.DescribeKeyOutput, error)
}

// KMSClient handles AWS Key Management Service operations
type KMSClient struct {
	client  KMSClientInterface
	region  string
	keySpec types.DataKeySpec
}

// NewKMSClient creates a new KMS client with default AWS config
func NewKMSClient(ctx context.Context) (*KMSClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &KMSClient{
		client:  kms.NewFromConfig(cfg),
		region:  cfg.Region,
		keySpec: types.DataKeySpecAes256,
	}, nil
}

// NewKMSWrapper creates a KMSClient wrapping another KMS client
func NewKMSWrapper(client KMSClientInterface, region string) *KMSClient {
	return &KMSClient{
		client:  client,
		region:  region,
		keySpec: types.DataKeySpecAes256,
	}
}

// GetRegion returns the AWS region for this KMS client
func (k *KMSClient) GetRegion() string {
	return k.region
}

// GenerateDataKey generates a new data key using the configured key spec
func (k *KMSClient) GenerateDataKey(ctx context.Context, keyId string, encryptionContext map[string]string) ([]byte, []byte, error) {
	logger := log.FromContext(ctx).WithName("kms-client")
	logger.Info("Generating data key", "keyId", keyId, "keySpec", k.keySpec, "region", k.region, "encryptionContext", encryptionContext)

	input := &kms.GenerateDataKeyInput{
		KeyId:             &keyId,
		KeySpec:           k.keySpec,
		EncryptionContext: encryptionContext,
	}

	result, err := k.client.GenerateDataKey(ctx, input)
	if err != nil {
		logger.Error(err, "Failed to generate data key", "keyId", keyId, "region", k.region)
		return nil, nil, fmt.Errorf("failed to generate data key: %w", err)
	}

	logger.Info("Successfully generated data key",
		"keyId", keyId,
		"plaintextLength", len(result.Plaintext),
		"ciphertextLength", len(result.CiphertextBlob),
		"region", k.region)

	return result.Plaintext, result.CiphertextBlob, nil
}

// Decrypt decrypts the given ciphertext using KMS
func (k *KMSClient) Decrypt(ctx context.Context, ciphertextBlob []byte, encryptionContext map[string]string) ([]byte, error) {
	logger := log.FromContext(ctx).WithName("kms-client")
	logger.Info("Decrypting data", "ciphertextLength", len(ciphertextBlob), "region", k.region, "encryptionContext", encryptionContext)

	input := &kms.DecryptInput{
		CiphertextBlob:    ciphertextBlob,
		EncryptionContext: encryptionContext,
	}

	result, err := k.client.Decrypt(ctx, input)
	if err != nil {
		logger.Error(err, "Failed to decrypt data", "region", k.region)
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	logger.Info("Successfully decrypted data",
		"plaintextLength", len(result.Plaintext),
		"region", k.region)

	return result.Plaintext, nil
}
