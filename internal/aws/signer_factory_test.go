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
	"testing"
	"time"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
)

func TestNewAWSSignerFactory(t *testing.T) {
	tests := []struct {
		name       string
		expiration time.Duration
	}{
		{
			name:       "5 minute expiration",
			expiration: time.Minute * 5,
		},
		{
			name:       "1 hour expiration",
			expiration: time.Hour,
		},
		{
			name:       "24 hour expiration",
			expiration: time.Hour * 24,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKMS := &MockKMSClient{}
			factory := NewAWSSignerFactory(NewKMSWrapper(mockKMS, "us-east-1"), "test-key", tt.expiration)

			assert.NotNil(t, factory)
			assert.Equal(t, tt.expiration, factory.expiration)
			assert.Equal(t, "test-key", factory.defaultKeyId)
		})
	}
}

func TestAWSSignerFactory_CreateSigner_WithAccessStrategy(t *testing.T) {
	mockKMS := &MockKMSClient{}
	factory := NewAWSSignerFactory(NewKMSWrapper(mockKMS, "us-east-1"), "default-key", time.Minute*5)

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "aws",
			CreateConnectionContext: map[string]string{
				"kmsKeyId": "test-key-id",
			},
		},
	}

	signer, err := factory.CreateSigner(accessStrategy)

	assert.NoError(t, err)
	assert.NotNil(t, signer)
}

func TestAWSSignerFactory_CreateSigner_EmptyAccessStrategy(t *testing.T) {
	mockKMS := &MockKMSClient{}
	factory := NewAWSSignerFactory(NewKMSWrapper(mockKMS, "us-east-1"), "default-key", time.Minute*5)

	signer, err := factory.CreateSigner(nil)

	assert.NoError(t, err)
	assert.NotNil(t, signer)
}

func TestAWSSignerFactory_CreateSigner_InvalidHandler(t *testing.T) {
	mockKMS := &MockKMSClient{}
	factory := NewAWSSignerFactory(NewKMSWrapper(mockKMS, "us-east-1"), "default-key", time.Minute*5)

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "invalid",
		},
	}

	signer, err := factory.CreateSigner(accessStrategy)

	assert.Error(t, err)
	assert.Nil(t, signer)
	assert.Contains(t, err.Error(), "unsupported connection handler")
}

func TestAWSSignerFactory_CreateSigner_MissingKMSKey(t *testing.T) {
	mockKMS := &MockKMSClient{}
	factory := NewAWSSignerFactory(NewKMSWrapper(mockKMS, "us-east-1"), "default-key", time.Minute*5)

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "aws",
			CreateConnectionContext: map[string]string{
				// Missing kmsKeyId
			},
		},
	}

	signer, err := factory.CreateSigner(accessStrategy)

	assert.Error(t, err)
	assert.Nil(t, signer)
	assert.Contains(t, err.Error(), "kmsKeyId is required")
}

func TestAWSSignerFactory_CreateSigner_InvalidEncryptionContext(t *testing.T) {
	mockKMS := &MockKMSClient{}
	factory := NewAWSSignerFactory(NewKMSWrapper(mockKMS, "us-east-1"), "default-key", time.Minute*5)

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionHandler: "aws",
			CreateConnectionContext: map[string]string{
				"kmsKeyId":          "test-key-id",
				"encryptionContext": "invalid-json",
			},
		},
	}

	signer, err := factory.CreateSigner(accessStrategy)

	assert.Error(t, err)
	assert.Nil(t, signer)
	assert.Contains(t, err.Error(), "failed to parse encryptionContext")
}

func TestAWSSignerFactory_CreateDefaultSigner(t *testing.T) {
	mockKMS := &MockKMSClient{}
	factory := NewAWSSignerFactory(NewKMSWrapper(mockKMS, "us-east-1"), "default-key", time.Minute*5)

	signer := factory.createDefaultSigner()

	assert.NotNil(t, signer)
}
