package aws

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// GetSSMDocumentName returns the SSM document name from environment
func GetSSMDocumentName() (string, error) {
	name := os.Getenv(AWSSSMDocumentNameEnv)
	if name == "" {
		return "", fmt.Errorf("%s environment variable is required", AWSSSMDocumentNameEnv)
	}
	return name, nil
}

// SSMClientInterface defines the interface for SSM operations we need
type SSMClientInterface interface {
	CreateActivation(ctx context.Context, params *ssm.CreateActivationInput, optFns ...func(*ssm.Options)) (*ssm.CreateActivationOutput, error)
	DescribeInstanceInformation(ctx context.Context, params *ssm.DescribeInstanceInformationInput, optFns ...func(*ssm.Options)) (*ssm.DescribeInstanceInformationOutput, error)
	DeregisterManagedInstance(ctx context.Context, params *ssm.DeregisterManagedInstanceInput, optFns ...func(*ssm.Options)) (*ssm.DeregisterManagedInstanceOutput, error)
	StartSession(ctx context.Context, params *ssm.StartSessionInput, optFns ...func(*ssm.Options)) (*ssm.StartSessionOutput, error)
}

// SSMClient handles AWS Systems Manager operations
type SSMClient struct {
	client SSMClientInterface
	region string
}

// SSMActivation represents the result of CreateActivation API call
type SSMActivation struct {
	ActivationId   string
	ActivationCode string
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

// NewSSMClientWithMock creates an SSMClient with a mock client for testing
func NewSSMClientWithMock(mockClient SSMClientInterface, region string) *SSMClient {
	return &SSMClient{
		client: mockClient,
		region: region,
	}
}

// GetRegion returns the AWS region for this SSM client
func (s *SSMClient) GetRegion() string {
	return s.region
}

// FindInstanceByPodUID finds SSM managed instance by pod UID tag
func (c *SSMClient) FindInstanceByPodUID(ctx context.Context, podUID string) (string, error) {
	filters := []types.InstanceInformationStringFilter{
		{
			Key:    aws.String(WorkspacePodUIDTagKey),
			Values: []string{podUID},
		},
	}

	instances, err := c.describeInstanceInformation(ctx, filters)
	if err != nil {
		return "", fmt.Errorf("failed to describe instances: %w", err)
	}

	if len(instances) == 0 {
		return "", fmt.Errorf("no managed instance found with workspace-pod-uid tag: %s", podUID)
	}

	if instances[0].InstanceId == nil {
		return "", fmt.Errorf("instance ID is nil for pod UID: %s", podUID)
	}

	return *instances[0].InstanceId, nil
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

// CreateActivation creates an SSM activation for managed instance registration
func (s *SSMClient) CreateActivation(ctx context.Context, description string, instanceName string, iamRole string, tags map[string]string) (*SSMActivation, error) {
	logger := log.FromContext(ctx).WithName("ssm-client")
	logger.Info("Creating SSM activation",
		"description", description,
		"instanceName", instanceName,
		"region", s.region,
		"iamRole", iamRole,
		"tags", tags,
	)

	// Validate required parameters
	if iamRole == "" {
		logger.Error(nil, "IAM role is required for SSM activation")
		return nil, fmt.Errorf("IAM role is required for SSM activation")
	}

	// Prepare tags
	awsTags := make([]types.Tag, 0, len(tags))
	for key, value := range tags {
		awsTags = append(awsTags, types.Tag{
			Key:   aws.String(key),
			Value: aws.String(value),
		})
	}

	// Set expiration to 24 hours from now
	expirationTime := time.Now().Add(24 * time.Hour)

	// Create activation input
	input := &ssm.CreateActivationInput{
		Description:         aws.String(description),
		IamRole:             aws.String(iamRole),
		RegistrationLimit:   aws.Int32(1),    // Only one instance can use this activation
		ExpirationDate:      &expirationTime, // Expires in 24 hours
		DefaultInstanceName: aws.String(instanceName),
		Tags:                awsTags,
	}

	logger.Info("Calling AWS SSM CreateActivation API",
		"iamRole", iamRole,
		"registrationLimit", *input.RegistrationLimit,
		"defaultInstanceName", instanceName,
	)

	result, err := s.client.CreateActivation(ctx, input)
	if err != nil {
		logger.Error(err, "AWS SSM CreateActivation API call failed",
			"description", description,
			"region", s.region,
		)
		return nil, fmt.Errorf("failed to create SSM activation: %w", err)
	}

	activation := &SSMActivation{
		ActivationId:   *result.ActivationId,
		ActivationCode: *result.ActivationCode,
	}

	logger.Info("Successfully created SSM activation",
		"activationId", activation.ActivationId,
		"instanceName", instanceName,
		"region", s.region,
		"description", description,
	)

	return activation, nil
}

// describeInstanceInformation retrieves information about SSM managed instances
func (s *SSMClient) describeInstanceInformation(ctx context.Context, filters []types.InstanceInformationStringFilter) ([]types.InstanceInformation, error) {
	logger := log.FromContext(ctx).WithName("ssm-client")
	logger.Info("Describing SSM managed instances", "filterCount", len(filters), "region", s.region)

	input := &ssm.DescribeInstanceInformationInput{
		Filters: filters,
	}

	result, err := s.client.DescribeInstanceInformation(ctx, input)
	if err != nil {
		logger.Error(err, "Failed to describe SSM managed instances", "region", s.region)
		return nil, fmt.Errorf("failed to describe SSM managed instances: %w", err)
	}

	logger.Info("Successfully described SSM managed instances",
		"instanceCount", len(result.InstanceInformationList),
		"region", s.region,
	)

	return result.InstanceInformationList, nil
}

// deregisterManagedInstance deregisters an SSM managed instance
func (s *SSMClient) deregisterManagedInstance(ctx context.Context, instanceId string) error {
	logger := log.FromContext(ctx).WithName("ssm-client")
	logger.Info("Deregistering SSM managed instance", "instanceId", instanceId, "region", s.region)

	input := &ssm.DeregisterManagedInstanceInput{
		InstanceId: aws.String(instanceId),
	}

	_, err := s.client.DeregisterManagedInstance(ctx, input)
	if err != nil {
		logger.Error(err, "Failed to deregister SSM managed instance",
			"instanceId", instanceId,
			"region", s.region,
		)
		return fmt.Errorf("failed to deregister SSM managed instance %s: %w", instanceId, err)
	}

	logger.Info("Successfully deregistered SSM managed instance",
		"instanceId", instanceId,
		"region", s.region,
	)

	return nil
}

// CleanupManagedInstancesByPodUID finds and deregisters SSM managed instances tagged with a specific pod UID
func (s *SSMClient) CleanupManagedInstancesByPodUID(ctx context.Context, podUID string) error {
	logger := log.FromContext(ctx).WithName("ssm-client")
	logger.Info("Cleaning up SSM managed instances for pod", "podUID", podUID, "region", s.region)

	// Create filter for pod UID tag - use constant
	filters := []types.InstanceInformationStringFilter{
		{
			Key:    aws.String(WorkspacePodUIDTagKey),
			Values: []string{podUID},
		},
	}

	// Find instances with the pod UID tag
	instances, err := s.describeInstanceInformation(ctx, filters)
	if err != nil {
		return fmt.Errorf("failed to find managed instances for pod %s: %w", podUID, err)
	}

	if len(instances) == 0 {
		logger.Info("No SSM managed instances found for pod", "podUID", podUID)
		return nil
	}

	// Warn if multiple instances found
	if len(instances) > 1 {
		logger.Error(nil, "Multiple SSM managed instances found for pod - this is unexpected and may indicate a cleanup issue",
			"podUID", podUID,
			"instanceCount", len(instances),
			"region", s.region)
	}

	// Deregister each found instance
	var errs []error
	for _, instance := range instances {
		instanceId := aws.ToString(instance.InstanceId)
		if err := s.deregisterManagedInstance(ctx, instanceId); err != nil {
			logger.Error(err, "Failed to deregister managed instance",
				"instanceId", instanceId,
				"podUID", podUID,
			)
			errs = append(errs, err)
			continue
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to deregister %d out of %d managed instances for pod %s",
			len(errs), len(instances), podUID)
	}

	logger.Info("Successfully cleaned up all SSM managed instances for pod",
		"podUID", podUID,
		"instanceCount", len(instances),
		"region", s.region,
	)

	return nil
}
