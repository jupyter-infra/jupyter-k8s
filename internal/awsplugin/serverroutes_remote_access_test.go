/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package awsplugin

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
)

func newTestSSMClient(mockSSM *MockSSMClient) *SSMClient {
	return NewSSMClientWithMock(mockSSM, "us-west-2")
}

func TestAWSRemoteAccessRoutes_RegisterNodeAgent_Success(t *testing.T) {
	mockSSM := new(MockSSMClient)
	mockSSM.On("CreateActivation", mock.Anything, mock.Anything).Return(
		&ssm.CreateActivationOutput{
			ActivationId:   aws.String("act-123"),
			ActivationCode: aws.String("code-456"),
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	resp, err := handler.RegisterNodeAgent(context.Background(), &pluginapi.RegisterNodeAgentRequest{
		PodUID:        "pod-uid-123",
		WorkspaceName: "my-workspace",
		Namespace:     "default",
		PodEventsContext: map[string]string{
			"ssmManagedNodeRole": "arn:aws:iam::123456789012:role/SSMRole",
		},
	})

	assert.NoError(t, err)
	assert.Equal(t, "act-123", resp.ActivationID)
	assert.Equal(t, "code-456", resp.ActivationCode)
	mockSSM.AssertExpectations(t)
}

func TestAWSRemoteAccessRoutes_RegisterNodeAgent_MissingRole(t *testing.T) {
	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(new(MockSSMClient)))

	_, err := handler.RegisterNodeAgent(context.Background(), &pluginapi.RegisterNodeAgentRequest{
		PodUID:           "pod-uid-123",
		WorkspaceName:    "my-workspace",
		Namespace:        "default",
		PodEventsContext: map[string]string{},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ssmManagedNodeRole")
}

func TestAWSRemoteAccessRoutes_DeregisterNodeAgent_Success(t *testing.T) {
	mockSSM := new(MockSSMClient)

	// Mock CleanupManagedInstancesByPodUID flow
	mockSSM.On("DescribeInstanceInformation", mock.Anything, mock.Anything).Return(
		&ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []types.InstanceInformation{},
		}, nil,
	)

	// Mock CleanupActivationsByPodUID flow
	mockSSM.On("DescribeActivations", mock.Anything, mock.Anything).Return(
		&ssm.DescribeActivationsOutput{
			ActivationList: []types.Activation{},
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	resp, err := handler.DeregisterNodeAgent(context.Background(), &pluginapi.DeregisterNodeAgentRequest{
		PodUID: "pod-uid-123",
	})

	assert.NoError(t, err)
	assert.NotNil(t, resp)
	mockSSM.AssertExpectations(t)
}

func TestAWSRemoteAccessRoutes_CreateSession_Success(t *testing.T) {
	mockSSM := new(MockSSMClient)

	// Mock FindInstanceByPodUID
	mockSSM.On("DescribeInstanceInformation", mock.Anything, mock.Anything).Return(
		&ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []types.InstanceInformation{
				{InstanceId: aws.String("mi-test-instance")},
			},
		}, nil,
	)

	// Mock checkNumActiveSessions
	mockSSM.On("DescribeSessions", mock.Anything, mock.Anything).Return(
		&ssm.DescribeSessionsOutput{Sessions: []types.Session{}}, nil,
	)

	// Mock StartSession
	mockSSM.On("StartSession", mock.Anything, mock.Anything).Return(
		&ssm.StartSessionOutput{
			SessionId:  aws.String("sess-abc"),
			StreamUrl:  aws.String("wss://stream.example.com"),
			TokenValue: aws.String("token-xyz"),
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	resp, err := handler.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:        "pod-uid-123",
		WorkspaceName: "my-workspace",
		Namespace:     "default",
		ConnectionContext: map[string]string{
			"ssmDocumentName": "my-ssm-document",
		},
	})

	assert.NoError(t, err)
	assert.Contains(t, resp.ConnectionURL, "sess-abc")
	assert.Contains(t, resp.ConnectionURL, VSCodeScheme.Default)
	assert.Contains(t, resp.ConnectionURL, "streamUrl=wss://stream.example.com")
	assert.Contains(t, resp.ConnectionURL, "sessionToken=token-xyz")
	assert.Contains(t, resp.ConnectionURL, "workspaceName=my-workspace")
	assert.Contains(t, resp.ConnectionURL, "namespace=default")
	mockSSM.AssertExpectations(t)
}

func TestAWSRemoteAccessRoutes_CreateSession_MissingDocumentName(t *testing.T) {
	mockSSM := new(MockSSMClient)

	// Mock FindInstanceByPodUID
	mockSSM.On("DescribeInstanceInformation", mock.Anything, mock.Anything).Return(
		&ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []types.InstanceInformation{
				{InstanceId: aws.String("mi-test-instance")},
			},
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	_, err := handler.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:            "pod-uid-123",
		WorkspaceName:     "my-workspace",
		Namespace:         "default",
		ConnectionContext: map[string]string{},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ssmDocumentName")
}

func TestAWSRemoteAccessRoutes_CreateSession_InstanceNotFound(t *testing.T) {
	mockSSM := new(MockSSMClient)

	mockSSM.On("DescribeInstanceInformation", mock.Anything, mock.Anything).Return(
		&ssm.DescribeInstanceInformationOutput{
			InstanceInformationList: []types.InstanceInformation{},
		}, nil,
	)

	handler := NewAWSRemoteAccessRoutes(newTestSSMClient(mockSSM))

	_, err := handler.CreateSession(context.Background(), &pluginapi.CreateSessionRequest{
		PodUID:        "pod-uid-123",
		WorkspaceName: "my-workspace",
		Namespace:     "default",
		ConnectionContext: map[string]string{
			"ssmDocumentName": "my-doc",
		},
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no managed instance found")
}
