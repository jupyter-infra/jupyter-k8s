package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const (
	testInstanceID = "mi-1234567890abcdef0"
	testSessionID  = "sess-1234567890abcdef0"
	testPodUID     = "test-pod-uid-123"
)

// MockSSMClient implements SSMClientInterface for testing
type MockSSMClient struct {
	mock.Mock
}

func (m *MockSSMClient) CreateActivation(ctx context.Context, params *ssm.CreateActivationInput, optFns ...func(*ssm.Options)) (*ssm.CreateActivationOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ssm.CreateActivationOutput), args.Error(1)
}

func (m *MockSSMClient) DescribeActivations(ctx context.Context, params *ssm.DescribeActivationsInput, optFns ...func(*ssm.Options)) (*ssm.DescribeActivationsOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ssm.DescribeActivationsOutput), args.Error(1)
}

func (m *MockSSMClient) DeleteActivation(ctx context.Context, params *ssm.DeleteActivationInput, optFns ...func(*ssm.Options)) (*ssm.DeleteActivationOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ssm.DeleteActivationOutput), args.Error(1)
}

func (m *MockSSMClient) DescribeInstanceInformation(ctx context.Context, params *ssm.DescribeInstanceInformationInput, optFns ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ssm.DescribeInstanceInformationOutput), args.Error(1)
}

func (m *MockSSMClient) DeregisterManagedInstance(ctx context.Context, params *ssm.DeregisterManagedInstanceInput, optFns ...func(*ssm.Options)) (*ssm.DeregisterManagedInstanceOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ssm.DeregisterManagedInstanceOutput), args.Error(1)
}

func (m *MockSSMClient) StartSession(ctx context.Context, params *ssm.StartSessionInput, optFns ...func(*ssm.Options)) (*ssm.StartSessionOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ssm.StartSessionOutput), args.Error(1)
}

func (m *MockSSMClient) CreateDocument(ctx context.Context, params *ssm.CreateDocumentInput, optFns ...func(*ssm.Options)) (*ssm.CreateDocumentOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ssm.CreateDocumentOutput), args.Error(1)
}

func TestSSMClient_FindInstanceByPodUID(t *testing.T) {
	tests := []struct {
		name      string
		podUID    string
		mockSetup func(*MockSSMClient)
		want      string
		wantErr   bool
	}{
		{
			name:   "successful instance lookup",
			podUID: "test-pod-uid",
			mockSetup: func(m *MockSSMClient) {
				instanceID := testInstanceID
				m.On("DescribeInstanceInformation", mock.Anything, mock.MatchedBy(func(input *ssm.DescribeInstanceInformationInput) bool {
					return len(input.Filters) > 0 && *input.Filters[0].Key == WorkspacePodUIDTagKey
				})).Return(
					&ssm.DescribeInstanceInformationOutput{
						InstanceInformationList: []types.InstanceInformation{
							{
								InstanceId: &instanceID,
							},
						},
					}, nil)
			},
			want:    testInstanceID,
			wantErr: false,
		},
		{
			name:   "no instances found",
			podUID: "nonexistent-pod-uid",
			mockSetup: func(m *MockSSMClient) {
				m.On("DescribeInstanceInformation", mock.Anything, mock.MatchedBy(func(input *ssm.DescribeInstanceInformationInput) bool {
					return len(input.Filters) > 0 && *input.Filters[0].Key == WorkspacePodUIDTagKey
				})).Return(
					&ssm.DescribeInstanceInformationOutput{
						InstanceInformationList: []types.InstanceInformation{},
					}, nil)
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockSSMClient{}
			tt.mockSetup(mockClient)

			client := NewSSMClientWithMock(mockClient, "us-east-1")

			got, err := client.FindInstanceByPodUID(context.Background(), tt.podUID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestSSMClient_StartSession(t *testing.T) {
	tests := []struct {
		name         string
		instanceID   string
		documentName string
		mockSetup    func(*MockSSMClient)
		want         *SessionInfo
		wantErr      bool
	}{
		{
			name:         "successful session start",
			instanceID:   testInstanceID,
			documentName: "test-document",
			mockSetup: func(m *MockSSMClient) {
				sessionID := testSessionID
				tokenValue := "test-token"
				streamURL := "wss://test-stream-url"
				m.On("StartSession", mock.Anything, mock.MatchedBy(func(input *ssm.StartSessionInput) bool {
					return *input.Target == testInstanceID &&
						*input.DocumentName == "test-document" &&
						input.Parameters != nil &&
						len(input.Parameters["portNumber"]) == 1 &&
						input.Parameters["portNumber"][0] == "2222"
				})).Return(
					&ssm.StartSessionOutput{
						SessionId:  &sessionID,
						TokenValue: &tokenValue,
						StreamUrl:  &streamURL,
					}, nil)
			},
			want: &SessionInfo{
				SessionID:    testSessionID,
				TokenValue:   "test-token",
				StreamURL:    "wss://test-stream-url",
				WebSocketURL: "wss://ssmmessages.us-east-1.amazonaws.com/v1/data-channel/" + testSessionID,
			},
			wantErr: false,
		},
		{
			name:         "session start error",
			instanceID:   testInstanceID,
			documentName: "invalid-document",
			mockSetup: func(m *MockSSMClient) {
				m.On("StartSession", mock.Anything, mock.MatchedBy(func(input *ssm.StartSessionInput) bool {
					return *input.Target == testInstanceID &&
						*input.DocumentName == "invalid-document" &&
						input.Parameters != nil &&
						len(input.Parameters["portNumber"]) == 1 &&
						input.Parameters["portNumber"][0] == "2222"
				})).Return(
					(*ssm.StartSessionOutput)(nil),
					&types.InvalidDocument{Message: aws.String("Document not found")})
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := &MockSSMClient{}
			tt.mockSetup(mockClient)

			client := NewSSMClientWithMock(mockClient, "us-east-1")

			got, err := client.StartSession(context.Background(), tt.instanceID, tt.documentName, "2222")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestCreateActivation_Success(t *testing.T) {
	// Setup
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")
	ctx := context.Background()

	// Mock AWS response
	expectedOutput := &ssm.CreateActivationOutput{
		ActivationId:   aws.String("activation-123"),
		ActivationCode: aws.String("code-456"),
	}
	mockClient.On("CreateActivation", ctx, mock.AnythingOfType("*ssm.CreateActivationInput")).Return(expectedOutput, nil)

	// Test data
	description := "Test activation"
	instanceName := "test-instance"
	iamRole := "arn:aws:iam::123456789012:role/SSMManagedInstanceCore"
	tags := map[string]string{
		"managed-by":        "jupyter-k8s-operator",
		"workspace-name":    "test-workspace",
		"workspace-pod-uid": "pod-123",
	}

	// Execute
	result, err := client.CreateActivation(ctx, description, instanceName, iamRole, tags)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "activation-123", result.ActivationId)
	assert.Equal(t, "code-456", result.ActivationCode)

	// Verify mock was called with correct parameters
	mockClient.AssertCalled(t, "CreateActivation", ctx, mock.MatchedBy(func(input *ssm.CreateActivationInput) bool {
		return *input.Description == description &&
			*input.DefaultInstanceName == instanceName &&
			*input.RegistrationLimit == 1 &&
			len(input.Tags) == 3 // Should have 3 tags
	}))
}

func TestCreateActivation_MissingIAMRole(t *testing.T) {
	// Setup
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")
	ctx := context.Background()

	// Execute with empty IAM role
	result, err := client.CreateActivation(ctx, "test", "instance", "", map[string]string{})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "IAM role is required for SSM activation")
	assert.Nil(t, result)

	// Verify no AWS call was made
	mockClient.AssertNotCalled(t, "CreateActivation")
}

func TestCreateActivation_AWSAPIError(t *testing.T) {
	// Setup
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")
	ctx := context.Background()

	// Mock AWS error
	awsError := errors.New("AccessDenied: User is not authorized")
	mockClient.On("CreateActivation", ctx, mock.AnythingOfType("*ssm.CreateActivationInput")).Return(nil, awsError)

	// Execute
	iamRole := "arn:aws:iam::123456789012:role/SSMManagedInstanceCore"
	result, err := client.CreateActivation(ctx, "test", "instance", iamRole, map[string]string{})

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create SSM activation")
	assert.Contains(t, err.Error(), "AccessDenied")
	assert.Nil(t, result)
}

func TestCleanupByPodUID_Success(t *testing.T) {
	// Setup
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")
	ctx := context.Background()
	podUID := "pod-123"

	// Mock DescribeInstanceInformation response - single instance
	describeOutput := &ssm.DescribeInstanceInformationOutput{
		InstanceInformationList: []types.InstanceInformation{
			{
				InstanceId: aws.String("i-1234567890abcdef0"),
			},
		},
	}
	mockClient.On("DescribeInstanceInformation", ctx, mock.MatchedBy(func(input *ssm.DescribeInstanceInformationInput) bool {
		// Verify correct filter format
		return len(input.Filters) == 1 &&
			*input.Filters[0].Key == WorkspacePodUIDTagKey &&
			len(input.Filters[0].Values) == 1 &&
			input.Filters[0].Values[0] == podUID
	})).Return(describeOutput, nil)

	// Mock DeregisterManagedInstance response
	deregisterOutput := &ssm.DeregisterManagedInstanceOutput{}
	mockClient.On("DeregisterManagedInstance", ctx, mock.MatchedBy(func(input *ssm.DeregisterManagedInstanceInput) bool {
		return *input.InstanceId == "i-1234567890abcdef0"
	})).Return(deregisterOutput, nil)

	// Execute
	err := client.CleanupManagedInstancesByPodUID(ctx, podUID)

	// Assert
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestCleanupByPodUID_NoInstances(t *testing.T) {
	// Setup
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")
	ctx := context.Background()
	podUID := "pod-123"

	// Mock DescribeInstanceInformation response - no instances
	describeOutput := &ssm.DescribeInstanceInformationOutput{
		InstanceInformationList: []types.InstanceInformation{},
	}
	mockClient.On("DescribeInstanceInformation", ctx, mock.AnythingOfType("*ssm.DescribeInstanceInformationInput")).Return(describeOutput, nil)

	// Execute
	err := client.CleanupManagedInstancesByPodUID(ctx, podUID)

	// Assert
	assert.NoError(t, err)

	// Verify no deregister calls were made
	mockClient.AssertNotCalled(t, "DeregisterManagedInstance")
}

func TestCleanupActivationsByPodUID_Success(t *testing.T) {
	// Setup
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")
	ctx := context.Background()
	podUID := testPodUID

	// Mock DescribeActivations response - single activation
	describeOutput := &ssm.DescribeActivationsOutput{
		ActivationList: []types.Activation{
			{
				ActivationId: aws.String("activation-123"),
			},
		},
	}
	mockClient.On("DescribeActivations", ctx, mock.MatchedBy(func(input *ssm.DescribeActivationsInput) bool {
		return len(input.Filters) == 1 &&
			string(input.Filters[0].FilterKey) == "DefaultInstanceName" &&
			len(input.Filters[0].FilterValues) == 1 &&
			input.Filters[0].FilterValues[0] == fmt.Sprintf("%s-%s", SSMInstanceNamePrefix, podUID)
	})).Return(describeOutput, nil)

	// Mock DeleteActivation response
	deleteOutput := &ssm.DeleteActivationOutput{}
	mockClient.On("DeleteActivation", ctx, mock.MatchedBy(func(input *ssm.DeleteActivationInput) bool {
		return *input.ActivationId == "activation-123"
	})).Return(deleteOutput, nil)

	// Execute
	err := client.CleanupActivationsByPodUID(ctx, podUID)

	// Assert
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestCleanupActivationsByPodUID_NoActivations(t *testing.T) {
	// Setup
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")
	ctx := context.Background()
	podUID := testPodUID

	// Mock DescribeActivations response - empty list
	describeOutput := &ssm.DescribeActivationsOutput{
		ActivationList: []types.Activation{},
	}
	mockClient.On("DescribeActivations", ctx, mock.AnythingOfType("*ssm.DescribeActivationsInput")).Return(describeOutput, nil)

	// Execute
	err := client.CleanupActivationsByPodUID(ctx, podUID)

	// Assert
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
	// Verify no delete calls were made
	mockClient.AssertNotCalled(t, "DeleteActivation")
}

func TestCleanupActivationsByPodUID_DescribeError(t *testing.T) {
	// Setup
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")
	ctx := context.Background()
	podUID := testPodUID

	// Mock DescribeActivations error
	expectedError := errors.New("AWS API error: access denied")
	mockClient.On("DescribeActivations", ctx, mock.AnythingOfType("*ssm.DescribeActivationsInput")).Return(nil, expectedError)

	// Execute
	err := client.CleanupActivationsByPodUID(ctx, podUID)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to describe SSM activations for pod")
	mockClient.AssertExpectations(t)
}

func TestCleanupActivationsByPodUID_DeleteError(t *testing.T) {
	// Setup
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")
	ctx := context.Background()
	podUID := testPodUID

	// Mock DescribeActivations response - single activation
	describeOutput := &ssm.DescribeActivationsOutput{
		ActivationList: []types.Activation{
			{
				ActivationId: aws.String("activation-123"),
			},
		},
	}
	mockClient.On("DescribeActivations", ctx, mock.AnythingOfType("*ssm.DescribeActivationsInput")).Return(describeOutput, nil)

	// Mock DeleteActivation error
	expectedError := errors.New("AWS API error: activation not found")
	mockClient.On("DeleteActivation", ctx, mock.AnythingOfType("*ssm.DeleteActivationInput")).Return(nil, expectedError)

	// Execute
	err := client.CleanupActivationsByPodUID(ctx, podUID)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to delete 1 out of 1 activations for pod")
	mockClient.AssertExpectations(t)
}

func TestSSMClient_createSageMakerSpaceSSMDocument_Success(t *testing.T) {
	// Setup
	ctx := context.Background()
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")

	t.Setenv(EKSClusterARNEnv, "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	expectedOutput := &ssm.CreateDocumentOutput{}
	mockClient.On("CreateDocument", ctx, mock.MatchedBy(func(input *ssm.CreateDocumentInput) bool {
		return *input.Name == CustomSSHDocumentName &&
			input.DocumentType == types.DocumentTypeSession &&
			*input.Content == SageMakerSpaceSSHSessionDocumentContent &&
			len(input.Tags) == 2
	})).Return(expectedOutput, nil)

	// Execute
	err := client.createSageMakerSpaceSSMDocument(ctx)

	// Assert
	assert.NoError(t, err)
	mockClient.AssertExpectations(t)
}

func TestSageMakerSpaceSSHSessionDocumentContent_EmbedWorking(t *testing.T) {
	// Test that the embedded SSH document content is properly loaded
	assert.NotEmpty(t, SageMakerSpaceSSHSessionDocumentContent, "SSH document content should not be empty")
	assert.Contains(t, SageMakerSpaceSSHSessionDocumentContent, "schemaVersion", "SSH document should contain schemaVersion")
	assert.Contains(t, SageMakerSpaceSSHSessionDocumentContent, "sessionType", "SSH document should contain sessionType")
	assert.Contains(t, SageMakerSpaceSSHSessionDocumentContent, "Port", "SSH document should contain Port sessionType")
	assert.Contains(t, SageMakerSpaceSSHSessionDocumentContent, "portNumber", "SSH document should contain portNumber parameter")
	assert.Contains(t, SageMakerSpaceSSHSessionDocumentContent, "idleSessionTimeout", "SSH document should contain idleSessionTimeout")
	assert.Contains(t, SageMakerSpaceSSHSessionDocumentContent, "maxSessionDuration", "SSH document should contain maxSessionDuration")

	// Verify it's valid JSON by unmarshaling
	var doc map[string]interface{}
	err := json.Unmarshal([]byte(SageMakerSpaceSSHSessionDocumentContent), &doc)
	assert.NoError(t, err, "SSH document content should be valid JSON")

	// Verify specific structure
	assert.Equal(t, "1.0", doc["schemaVersion"])
	assert.Equal(t, "Port", doc["sessionType"])
}

func TestSSMClient_createSageMakerSpaceSSMDocument_DocumentAlreadyExists(t *testing.T) {
	// Setup
	ctx := context.Background()
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")

	t.Setenv(EKSClusterARNEnv, "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	docExistsError := &types.DocumentAlreadyExists{}
	mockClient.On("CreateDocument", ctx, mock.AnythingOfType("*ssm.CreateDocumentInput")).Return(nil, docExistsError)

	// Execute
	err := client.createSageMakerSpaceSSMDocument(ctx)

	// Assert
	assert.NoError(t, err) // Should not return error when document already exists
	mockClient.AssertExpectations(t)
}

func TestSSMClient_createSageMakerSpaceSSMDocument_MissingClusterARN(t *testing.T) {
	// Setup
	ctx := context.Background()
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")

	err := client.createSageMakerSpaceSSMDocument(ctx)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "environment variable is required")
}

func TestSSMClient_createSageMakerSpaceSSMDocument_CreateError(t *testing.T) {
	// Setup
	ctx := context.Background()
	mockClient := &MockSSMClient{}
	client := NewSSMClientWithMock(mockClient, "us-west-2")

	t.Setenv(EKSClusterARNEnv, "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	expectedError := errors.New("create document failed")
	mockClient.On("CreateDocument", ctx, mock.AnythingOfType("*ssm.CreateDocumentInput")).Return(nil, expectedError)

	// Execute
	err := client.createSageMakerSpaceSSMDocument(ctx)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create SSH document")
	mockClient.AssertExpectations(t)
}
