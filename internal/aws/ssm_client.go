// Package aws provides AWS client functionality for workspace access
package aws

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// GetSSMDocumentName returns the SSM document name from environment
func GetSSMDocumentName() (string, error) {
	name := os.Getenv("SSM_DOCUMENT_NAME")
	if name == "" {
		return "", fmt.Errorf("SSM_DOCUMENT_NAME environment variable is required")
	}
	return name, nil
}

// SSMAPI interface for SSM operations
type SSMAPI interface {
	DescribeInstanceInformation(ctx context.Context, params *ssm.DescribeInstanceInformationInput, optFns ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error)
	StartSession(ctx context.Context, params *ssm.StartSessionInput, optFns ...func(*ssm.Options)) (*ssm.StartSessionOutput, error)
}

// SSMClient wraps AWS SSM client functionality
type SSMClient struct {
	client SSMAPI
	region string
}

// SessionInfo contains SSM session connection details
type SessionInfo struct {
	SessionID    string `json:"sessionId"`
	StreamURL    string `json:"streamUrl"`
	TokenValue   string `json:"tokenValue"`
	WebSocketURL string `json:"webSocketUrl"`
}

// SSMDocConfig contains configuration for creating SSM documents
type SSMDocConfig struct {
	Name        string
	Content     string
	Description string
}

// NewSSMClient creates a new SSM client
func NewSSMClient(ctx context.Context) (*SSMClient, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &SSMClient{
		client: ssm.NewFromConfig(cfg),
		region: cfg.Region,
	}, nil
}

// FindInstanceByPodUID finds SSM managed instance by pod UID tag
func (c *SSMClient) FindInstanceByPodUID(ctx context.Context, podUID string) (string, error) {
	input := &ssm.DescribeInstanceInformationInput{
		Filters: []types.InstanceInformationStringFilter{
			{
				Key:    aws.String("tag:workspace-pod-uid"),
				Values: []string{podUID},
			},
		},
	}

	result, err := c.client.DescribeInstanceInformation(ctx, input)
	if err != nil {
		return "", fmt.Errorf("failed to describe instances: %w", err)
	}

	if len(result.InstanceInformationList) == 0 {
		return "", fmt.Errorf("no managed instance found with workspace-pod-uid tag: %s", podUID)
	}

	if result.InstanceInformationList[0].InstanceId == nil {
		return "", fmt.Errorf("instance ID is nil for pod UID: %s", podUID)
	}

	return *result.InstanceInformationList[0].InstanceId, nil
}

// StartSession starts an SSM session for the given instance with specified document
func (c *SSMClient) StartSession(ctx context.Context, instanceID, documentName string) (*SessionInfo, error) {
	input := &ssm.StartSessionInput{
		Target:       &instanceID,
		DocumentName: aws.String(documentName),
	}

	result, err := c.client.StartSession(ctx, input)
	if err != nil {
		// Check for specific SSM errors
		var invalidDocument *types.InvalidDocument
		if errors.As(err, &invalidDocument) {
			return nil, fmt.Errorf("SSM document '%s' not found or invalid: %w", documentName, err)
		}
		return nil, fmt.Errorf("failed to start session for instance %s: %w", instanceID, err)
	}

	if result.SessionId == nil {
		return nil, fmt.Errorf("received nil SessionId from SSM service")
	}

	sessionInfo := &SessionInfo{
		SessionID: *result.SessionId,
	}

	if result.StreamUrl != nil {
		sessionInfo.StreamURL = *result.StreamUrl
	}
	if result.TokenValue != nil {
		sessionInfo.TokenValue = *result.TokenValue
	}

	sessionInfo.WebSocketURL = fmt.Sprintf("wss://ssmmessages.%s.amazonaws.com/v1/data-channel/%s", c.region, *result.SessionId)

	return sessionInfo, nil
}
