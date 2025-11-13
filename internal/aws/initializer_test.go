package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestResourceInitializer_ValidKMSKey(t *testing.T) {
	ctx := context.Background()
	t.Setenv(EKSClusterARNEnv, "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

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
