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
	"encoding/json"
	"fmt"
	"time"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/jwt"
)

// Connection context keys for KMS JWT operations
const (
	KMSKeyIDKey          = "kmsKeyId"
	EncryptionContextKey = "encryptionContext"
)

// AWSSignerFactory creates AWS KMS-based signers
//
//revive:disable:exported
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
