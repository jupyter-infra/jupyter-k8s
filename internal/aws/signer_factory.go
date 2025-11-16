package aws

import (
	"encoding/json"
	"fmt"
	"time"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/jwt"
)

// Connection context keys for KMS JWT operations
const (
	KMSKeyIDKey          = "kmsKeyId"
	EncryptionContextKey = "encryptionContext"
)

// AWSSignerFactory creates AWS KMS-based signers
type AWSSignerFactory struct {
	kmsClient    *KMSClient
	defaultKeyId string
	expiration   time.Duration
}

// NewAWSSignerFactory creates a new AWS signer factory
func NewAWSSignerFactory(kmsClient *KMSClient, defaultKeyId string, expiration time.Duration) *AWSSignerFactory {
	return &AWSSignerFactory{
		kmsClient:    kmsClient,
		defaultKeyId: defaultKeyId,
		expiration:   expiration,
	}
}

// CreateSigner creates a JWT signer based on access strategy configuration
func (f *AWSSignerFactory) CreateSigner(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (jwt.Signer, error) {
	// Handle missing access strategy
	if accessStrategy == nil {
		return f.createDefaultSigner(), nil
	}

	// Handle access strategies without createConnectionHandler (use default)
	if accessStrategy.Spec.CreateConnectionHandler == "" {
		return f.createDefaultSigner(), nil
	}

	// Only support "aws" handler for now
	if accessStrategy.Spec.CreateConnectionHandler != "aws" {
		return nil, fmt.Errorf("unsupported connection handler: %s", accessStrategy.Spec.CreateConnectionHandler)
	}

	// Parse configuration from createConnectionContext
	context := accessStrategy.Spec.CreateConnectionContext
	keyId := context[KMSKeyIDKey]
	if keyId == "" {
		return nil, fmt.Errorf("%s is required in createConnectionContext", KMSKeyIDKey)
	}

	// Parse encryption context from JSON
	var encryptionContext map[string]string
	if encCtxStr := context[EncryptionContextKey]; encCtxStr != "" {
		if err := json.Unmarshal([]byte(encCtxStr), &encryptionContext); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", EncryptionContextKey, err)
		}
	}

	return NewKMSJWTManager(KMSJWTConfig{
		KMSClient:         f.kmsClient,
		KeyId:             keyId,
		Issuer:            "jupyter-k8s",
		Audience:          "workspace-ui",
		Expiration:        f.expiration,
		EncryptionContext: encryptionContext,
	}), nil
}

// createDefaultSigner creates a signer with default configuration
func (f *AWSSignerFactory) createDefaultSigner() jwt.Signer {
	return NewKMSJWTManager(KMSJWTConfig{
		KMSClient:         f.kmsClient,
		KeyId:             f.defaultKeyId,
		Issuer:            "jupyter-k8s",
		Audience:          "workspace-ui",
		Expiration:        f.expiration,
		EncryptionContext: nil,
	})
}
