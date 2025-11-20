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
	"testing"

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
	plaintext, encrypted, err := client.GenerateDataKey(context.Background(), "test-key-id", nil)
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
	plaintext, err := client.Decrypt(context.Background(), []byte("some-encrypted-blob"), nil)
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

// MockKMSClientWithError simulates CreateAlias returning a generic error
type MockKMSClientWithError struct{}

func (m *MockKMSClientWithError) GenerateDataKey(ctx context.Context, params *kms.GenerateDataKeyInput, optFns ...func(*kms.Options)) (*kms.GenerateDataKeyOutput, error) {
	return &kms.GenerateDataKeyOutput{}, nil
}

func (m *MockKMSClientWithError) Decrypt(ctx context.Context, params *kms.DecryptInput, optFns ...func(*kms.Options)) (*kms.DecryptOutput, error) {
	return &kms.DecryptOutput{}, nil
}

func (m *MockKMSClientWithError) DescribeKey(ctx context.Context, params *kms.DescribeKeyInput, optFns ...func(*kms.Options)) (*kms.DescribeKeyOutput, error) {
	return &kms.DescribeKeyOutput{}, nil
}
