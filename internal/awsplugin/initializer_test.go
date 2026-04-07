/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package awsplugin

import (
	"context"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
)

func TestResourceInitializer_EnsureResources(t *testing.T) {
	ctx := context.Background()
	t.Setenv("CLUSTER_ID", "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	// Create mock clients
	mockSSM := &MockSSMClient{}

	// Mock SSM CreateDocument success
	mockSSM.On("CreateDocument", mock.Anything, mock.AnythingOfType("*ssm.CreateDocumentInput")).Return(&ssm.CreateDocumentOutput{}, nil)

	// Create initializer with mocks
	initializer := &ResourceInitializer{
		ssmClient: NewSSMClientWithMock(mockSSM, "us-west-2"),
	}

	err := initializer.EnsureResourcesInitialized(ctx)

	assert.NoError(t, err)
	mockSSM.AssertExpectations(t)
}

func TestResourceInitializer_EnsureResources_OnlyCalledOnce(t *testing.T) {
	ctx := context.Background()
	t.Setenv("CLUSTER_ID", "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	mockSSM := &MockSSMClient{}
	mockSSM.On("CreateDocument", mock.Anything, mock.AnythingOfType("*ssm.CreateDocumentInput")).Return(&ssm.CreateDocumentOutput{}, nil).Once()

	initializer := &ResourceInitializer{
		ssmClient: NewSSMClientWithMock(mockSSM, "us-west-2"),
	}

	// First call initializes
	err := initializer.EnsureResourcesInitialized(ctx)
	assert.NoError(t, err)

	// Second call is a no-op (CreateDocument not called again)
	err = initializer.EnsureResourcesInitialized(ctx)
	assert.NoError(t, err)

	mockSSM.AssertExpectations(t)
}

func TestResourceInitializer_EnsureResources_Error(t *testing.T) {
	ctx := context.Background()
	t.Setenv("CLUSTER_ID", "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	mockSSM := &MockSSMClient{}
	mockSSM.On("CreateDocument", mock.Anything, mock.AnythingOfType("*ssm.CreateDocumentInput")).Return(
		(*ssm.CreateDocumentOutput)(nil), fmt.Errorf("access denied"),
	).Once()

	initializer := &ResourceInitializer{
		ssmClient: NewSSMClientWithMock(mockSSM, "us-west-2"),
	}

	err := initializer.EnsureResourcesInitialized(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SSH document")
	mockSSM.AssertExpectations(t)
}

func TestAWSRemoteAccessRoutes_Initialize_Success(t *testing.T) {
	t.Setenv("CLUSTER_ID", "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	mockSSM := &MockSSMClient{}
	mockSSM.On("CreateDocument", mock.Anything, mock.AnythingOfType("*ssm.CreateDocumentInput")).Return(&ssm.CreateDocumentOutput{}, nil)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	resp, err := handler.Initialize(context.Background(), &pluginapi.InitializeRequest{})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	mockSSM.AssertExpectations(t)
}

func TestAWSRemoteAccessRoutes_Initialize_Error(t *testing.T) {
	t.Setenv("CLUSTER_ID", "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	mockSSM := &MockSSMClient{}
	mockSSM.On("CreateDocument", mock.Anything, mock.AnythingOfType("*ssm.CreateDocumentInput")).Return(
		(*ssm.CreateDocumentOutput)(nil), fmt.Errorf("access denied"),
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	_, err := handler.Initialize(context.Background(), &pluginapi.InitializeRequest{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize resources")
	mockSSM.AssertExpectations(t)
}
