// Package aws provides AWS client functionality for workspace access
package aws

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// SSMAPI interface for SSM operations
type SSMAPI interface {
	CreateDocument(ctx context.Context, params *ssm.CreateDocumentInput, optFns ...func(*ssm.Options)) (*ssm.CreateDocumentOutput, error)
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

// CreateDocument creates a session document in SSM
func (c *SSMClient) CreateDocument(ctx context.Context, ssmDocConfig SSMDocConfig) error {
	input := &ssm.CreateDocumentInput{
		Name:         aws.String(ssmDocConfig.Name),
		DocumentType: types.DocumentTypeSession,
		Content:      aws.String(ssmDocConfig.Content),
	}

	_, err := c.client.CreateDocument(ctx, input)
	if err != nil {
		// Check if document already exists
		var docAlreadyExists *types.DocumentAlreadyExists
		if errors.As(err, &docAlreadyExists) {
			return nil // Document already exists, that's fine
		}
		return fmt.Errorf("failed to create document %s: %w", ssmDocConfig.Name, err)
	}

	return nil
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
