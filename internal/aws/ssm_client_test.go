package aws

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSSMAPI is a mock implementation of the SSM API
type MockSSMAPI struct {
	mock.Mock
}

func (m *MockSSMAPI) CreateDocument(ctx context.Context, params *ssm.CreateDocumentInput, optFns ...func(*ssm.Options)) (*ssm.CreateDocumentOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ssm.CreateDocumentOutput), args.Error(1)
}

func (m *MockSSMAPI) DescribeInstanceInformation(ctx context.Context, params *ssm.DescribeInstanceInformationInput, optFns ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ssm.DescribeInstanceInformationOutput), args.Error(1)
}

func (m *MockSSMAPI) StartSession(ctx context.Context, params *ssm.StartSessionInput, optFns ...func(*ssm.Options)) (*ssm.StartSessionOutput, error) {
	args := m.Called(ctx, params)
	return args.Get(0).(*ssm.StartSessionOutput), args.Error(1)
}

func TestSSMClient_CreateDocument(t *testing.T) {
	tests := []struct {
		name      string
		docConfig SSMDocConfig
		mockSetup func(*MockSSMAPI)
		wantErr   bool
	}{
		{
			name: "successful document creation",
			docConfig: SSMDocConfig{
				Name:        "test-document",
				Content:     `{"test": "content"}`,
				Description: "Test document",
			},
			mockSetup: func(m *MockSSMAPI) {
				m.On("CreateDocument", mock.Anything, mock.MatchedBy(func(input *ssm.CreateDocumentInput) bool {
					return *input.Name == "test-document"
				})).Return(&ssm.CreateDocumentOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name: "document already exists",
			docConfig: SSMDocConfig{
				Name:        "existing-document",
				Content:     `{"test": "content"}`,
				Description: "Existing document",
			},
			mockSetup: func(m *MockSSMAPI) {
				m.On("CreateDocument", mock.Anything, mock.MatchedBy(func(input *ssm.CreateDocumentInput) bool {
					return *input.Name == "existing-document"
				})).Return(
					&ssm.CreateDocumentOutput{},
					&types.DocumentAlreadyExists{Message: aws.String("Document already exists")},
				)
			},
			wantErr: false, // Should not error when document already exists
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &MockSSMAPI{}
			tt.mockSetup(mockAPI)

			client := &SSMClient{
				client: mockAPI,
				region: "us-east-1",
			}

			err := client.CreateDocument(context.Background(), tt.docConfig)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockAPI.AssertExpectations(t)
		})
	}
}

func TestSSMClient_FindInstanceByPodUID(t *testing.T) {
	tests := []struct {
		name      string
		podUID    string
		mockSetup func(*MockSSMAPI)
		want      string
		wantErr   bool
	}{
		{
			name:   "successful instance lookup",
			podUID: "test-pod-uid",
			mockSetup: func(m *MockSSMAPI) {
				instanceID := "mi-1234567890abcdef0"
				m.On("DescribeInstanceInformation", mock.Anything, mock.MatchedBy(func(input *ssm.DescribeInstanceInformationInput) bool {
					return len(input.Filters) > 0 && *input.Filters[0].Key == "tag:workspace-pod-uid"
				})).Return(
					&ssm.DescribeInstanceInformationOutput{
						InstanceInformationList: []types.InstanceInformation{
							{
								InstanceId: &instanceID,
							},
						},
					}, nil)
			},
			want:    "mi-1234567890abcdef0",
			wantErr: false,
		},
		{
			name:   "no instances found",
			podUID: "nonexistent-pod-uid",
			mockSetup: func(m *MockSSMAPI) {
				m.On("DescribeInstanceInformation", mock.Anything, mock.MatchedBy(func(input *ssm.DescribeInstanceInformationInput) bool {
					return len(input.Filters) > 0 && *input.Filters[0].Key == "tag:workspace-pod-uid"
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
			mockAPI := &MockSSMAPI{}
			tt.mockSetup(mockAPI)

			client := &SSMClient{
				client: mockAPI,
				region: "us-east-1",
			}

			got, err := client.FindInstanceByPodUID(context.Background(), tt.podUID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			mockAPI.AssertExpectations(t)
		})
	}
}

func TestSSMClient_StartSession(t *testing.T) {
	tests := []struct {
		name         string
		instanceID   string
		documentName string
		mockSetup    func(*MockSSMAPI)
		want         *SessionInfo
		wantErr      bool
	}{
		{
			name:         "successful session start",
			instanceID:   "mi-1234567890abcdef0",
			documentName: "test-document",
			mockSetup: func(m *MockSSMAPI) {
				sessionID := "sess-1234567890abcdef0"
				tokenValue := "test-token"
				streamURL := "wss://test-stream-url"
				m.On("StartSession", mock.Anything, mock.MatchedBy(func(input *ssm.StartSessionInput) bool {
					return *input.Target == "mi-1234567890abcdef0" && *input.DocumentName == "test-document"
				})).Return(
					&ssm.StartSessionOutput{
						SessionId:  &sessionID,
						TokenValue: &tokenValue,
						StreamUrl:  &streamURL,
					}, nil)
			},
			want: &SessionInfo{
				SessionID:    "sess-1234567890abcdef0",
				TokenValue:   "test-token",
				StreamURL:    "wss://test-stream-url",
				WebSocketURL: "wss://ssmmessages.us-east-1.amazonaws.com/v1/data-channel/sess-1234567890abcdef0",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &MockSSMAPI{}
			tt.mockSetup(mockAPI)

			client := &SSMClient{
				client: mockAPI,
				region: "us-east-1",
			}

			got, err := client.StartSession(context.Background(), tt.instanceID, tt.documentName)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			mockAPI.AssertExpectations(t)
		})
	}
}

func TestGetSSHDocumentConfig(t *testing.T) {
	config := GetSSHDocumentConfig()
	
	assert.Equal(t, CustomSSHDocumentName, config.Name)
	assert.Equal(t, SSHDocumentContent, config.Content)
	assert.NotEmpty(t, config.Description)
}
