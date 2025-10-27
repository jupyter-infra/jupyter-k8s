package aws

import (
	"context"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// Variables for dependency injection in tests
var (
	newSSMClient = NewSSMClient
)

// SSMRemoteAccessClientInterface defines the interface for SSM operations needed by the remote access strategy.
type SSMRemoteAccessClientInterface interface {
	CreateActivation(ctx context.Context, description string, instanceName string, iamRole string, tags map[string]string) (*SSMActivation, error)
	GetRegion() string
	CleanupManagedInstancesByPodUID(ctx context.Context, podUID string) error
	CleanupActivationsByPodUID(ctx context.Context, podUID string) error
	FindInstanceByPodUID(ctx context.Context, podUID string) (string, error)
	StartSession(ctx context.Context, instanceID, documentName string) (*SessionInfo, error)
}

// PodExecInterface defines the interface for executing commands in pods.
type PodExecInterface interface {
	ExecInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (string, error)
}

// Constants for SSM remote access strategy
const (
	// Container names
	SSMAgentSidecarContainerName = "ssm-agent-sidecar"
	WorkspaceContainerName       = "workspace"

	// Tag keys
	TagManagedBy       = "managed-by"
	TagWorkspaceName   = "workspace-name"
	TagNamespace       = "namespace"
	TagWorkspacePodUID = "workspace-pod-uid"

	// File paths
	SSMRegistrationMarkerFile = "/tmp/ssm-registered"
	SSMRegistrationScript     = "/usr/local/bin/register-ssm.sh"
	RemoteAccessServerPath    = "/ssm-remote-access/remote-access-server"
)

// SSMRemoteAccessStrategy handles SSM remote access strategy operations
type SSMRemoteAccessStrategy struct {
	ssmClient   SSMRemoteAccessClientInterface
	podExecUtil PodExecInterface
}

// NewSSMRemoteAccessStrategy creates a new SSMRemoteAccessStrategy with optional dependency injection
// If ssmClient or podExecUtil are nil, default implementations will be created
func NewSSMRemoteAccessStrategy(ssmClient SSMRemoteAccessClientInterface, podExecUtil PodExecInterface) (*SSMRemoteAccessStrategy, error) {
	// If SSM client not provided, create default
	if ssmClient == nil {
		realSSMClient, err := newSSMClient(context.Background())
		if err != nil {
			// Log error but don't fail - SSM features will be disabled to avoid blocking pod operations
			logf.Log.Error(err, "Failed to initialize SSM client")
		}
		ssmClient = realSSMClient
	}

	// PodExecUtil must be provided by the caller since it's in the controller package
	if podExecUtil == nil {
		return nil, fmt.Errorf("podExecUtil is required")
	}

	return &SSMRemoteAccessStrategy{
		ssmClient:   ssmClient,
		podExecUtil: podExecUtil,
	}, nil
}

// InitSSMAgent initializes SSM agent for aws-ssm-remote-access strategy
func (s *SSMRemoteAccessStrategy) InitSSMAgent(ctx context.Context, pod *corev1.Pod, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) error {
	if s.ssmClient == nil {
		return fmt.Errorf("SSM client not available")
	}

	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "workspace", workspace.Name)

	// Find the SSM sidecar container
	var ssmSidecarStatus *corev1.ContainerStatus
	for i, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.Name == SSMAgentSidecarContainerName {
			ssmSidecarStatus = &pod.Status.ContainerStatuses[i]
			break
		}
	}

	if ssmSidecarStatus == nil {
		logger.Info("SSM sidecar container not found in pod")
		return nil
	}

	// Check if sidecar is running
	if ssmSidecarStatus.State.Running == nil {
		logger.Info("SSM sidecar container is not running yet", "ready", ssmSidecarStatus.Ready)
		return nil
	}

	// Check if SSM registration already completed
	if s.isSSMRegistrationCompleted(ctx, pod) {
		logger.Info("SSM registration already completed for this pod")
		return nil
	}

	// Perform SSM registration
	if err := s.performSSMRegistration(ctx, pod, workspace, accessStrategy); err != nil {
		logger.Error(err, "Failed to perform SSM registration")
		return err
	}

	logger.Info("SSM registration completed successfully")
	return nil
}

// CleanupSSMManagedNodes finds and deregisters SSM managed nodes for a pod
func (s *SSMRemoteAccessStrategy) CleanupSSMManagedNodes(ctx context.Context, pod *corev1.Pod) error {
	if s.ssmClient == nil {
		return fmt.Errorf("SSM client not available")
	}

	logger := logf.FromContext(ctx).WithValues("pod", pod.Name)

	// Cleanup managed instances
	if err := s.ssmClient.CleanupManagedInstancesByPodUID(ctx, string(pod.UID)); err != nil {
		logger.Error(err, "Failed to cleanup managed instances")
		// Don't return early - try to cleanup activations too
	}

	// Cleanup hybrid activations
	if err := s.ssmClient.CleanupActivationsByPodUID(ctx, string(pod.UID)); err != nil {
		logger.Error(err, "Failed to cleanup activations")
		return fmt.Errorf("failed to cleanup activations for pod %s: %w", pod.UID, err)
	}

	return nil
}

// isSSMRegistrationCompleted checks if SSM registration is already done for this pod
func (s *SSMRemoteAccessStrategy) isSSMRegistrationCompleted(ctx context.Context, pod *corev1.Pod) bool {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name)
	noStdin := "" // For commands that don't need stdin input

	// TODO: improve race condition handling for rapid pod events

	// Check for completion marker file in sidecar
	cmd := []string{"test", "-f", SSMRegistrationMarkerFile}
	_, err := s.podExecUtil.ExecInPod(ctx, pod, SSMAgentSidecarContainerName, cmd, noStdin)

	completed := err == nil
	logger.V(2).Info("SSM registration completion check", "completed", completed)
	return completed
}

// performSSMRegistration handles the SSM activation and registration process
func (s *SSMRemoteAccessStrategy) performSSMRegistration(ctx context.Context, pod *corev1.Pod, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) error {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "workspace", workspace.Name)
	noStdin := "" // For commands that don't need stdin input

	if s.ssmClient == nil {
		return fmt.Errorf("SSM client not available")
	}

	// Step 1: Create SSM activation
	logger.Info("Creating SSM activation")
	activationCode, activationId, err := s.createSSMActivation(ctx, pod, workspace, accessStrategy)
	if err != nil {
		return fmt.Errorf("failed to create SSM activation: %w", err)
	}

	// Step 2: Run register-ssm.sh with sensitive values passed via stdin
	logger.Info("Running SSM registration script in sidecar")
	region := s.ssmClient.GetRegion()

	// Use stdin to pass only sensitive values securely
	cmd := []string{"bash", "-c", fmt.Sprintf("read ACTIVATION_ID && read ACTIVATION_CODE && env ACTIVATION_ID=\"$ACTIVATION_ID\" ACTIVATION_CODE=\"$ACTIVATION_CODE\" REGION=%s %s", region, SSMRegistrationScript)}
	stdinData := fmt.Sprintf("%s\n%s\n", activationId, activationCode)

	if _, err := s.podExecUtil.ExecInPod(ctx, pod, SSMAgentSidecarContainerName, cmd, stdinData); err != nil {
		return fmt.Errorf("failed to execute SSM registration script: %w", err)
	}

	// Step 3: Start remote access server in main container
	logger.Info("Starting remote access server in main container")
	serverCmd := []string{"bash", "-c", fmt.Sprintf("sudo %s > /dev/null 2>&1 &", RemoteAccessServerPath)}
	if _, err := s.podExecUtil.ExecInPod(ctx, pod, WorkspaceContainerName, serverCmd, noStdin); err != nil {
		return fmt.Errorf("failed to start remote access server: %w", err)
	}

	// Step 4: Create completion marker file
	logger.Info("Creating SSM registration completion marker")
	markerCmd := []string{"touch", SSMRegistrationMarkerFile}
	if _, err := s.podExecUtil.ExecInPod(ctx, pod, SSMAgentSidecarContainerName, markerCmd, noStdin); err != nil {
		return fmt.Errorf("failed to create completion marker: %w", err)
	}

	return nil
}

// createSSMActivation creates an SSM activation and returns activation code and ID
func (s *SSMRemoteAccessStrategy) createSSMActivation(ctx context.Context, pod *corev1.Pod, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (string, string, error) {
	logger := logf.FromContext(ctx).WithValues("workspace", workspace.Name)

	if s.ssmClient == nil {
		return "", "", fmt.Errorf("SSM client not available")
	}

	// Get IAM role from access strategy controller configuration
	var iamRole string
	if accessStrategy.Spec.ControllerConfig != nil {
		iamRole = accessStrategy.Spec.ControllerConfig["SSM_MANAGED_NODE_ROLE"]
	}

	if iamRole == "" {
		return "", "", fmt.Errorf("SSM_MANAGED_NODE_ROLE not found in access strategy")
	}

	// Get EKS cluster ARN from environment variable
	eksClusterARN := os.Getenv(EKSClusterARNEnv)
	if eksClusterARN == "" {
		return "", "", fmt.Errorf("%s environment variable is required", EKSClusterARNEnv)
	}

	// Prepare tags - include SageMaker required tags for policy compliance
	tags := map[string]string{
		SageMakerManagedByTagKey:  SageMakerManagedByTagValue,
		SageMakerEKSClusterTagKey: eksClusterARN,
		TagWorkspaceName:          workspace.Name,
		TagNamespace:              workspace.Namespace,
		TagWorkspacePodUID:        string(pod.UID),
	}

	// Create description and instance name with fixed prefix
	description := fmt.Sprintf("Activation for %s/%s (pod: %s)", workspace.Namespace, workspace.Name, string(pod.UID))
	instanceName := fmt.Sprintf("%s-%s", SSMInstanceNamePrefix, string(pod.UID))

	// Pass the IAM role directly to the SSM client
	activation, err := s.ssmClient.CreateActivation(ctx, description, instanceName, iamRole, tags)
	if err != nil {
		logger.Error(err, "Failed to create SSM activation")
		return "", "", fmt.Errorf("failed to create SSM activation: %w", err)
	}

	logger.Info("SSM activation created successfully",
		"activationId", activation.ActivationId,
		"iamRole", iamRole)

	return activation.ActivationCode, activation.ActivationId, nil
}

// GenerateVSCodeConnectionURL generates a VSCode connection URL using SSM session
func (s *SSMRemoteAccessStrategy) GenerateVSCodeConnectionURL(ctx context.Context, workspaceName, namespace, podUID, eksClusterARN string) (string, error) {
	logger := logf.FromContext(ctx).WithName("ssm-vscode-connection")

	// Find managed instance by pod UID
	instanceID, err := s.ssmClient.FindInstanceByPodUID(ctx, podUID)
	if err != nil {
		return "", fmt.Errorf("failed to find instance by pod UID: %w", err)
	}

	logger.Info("Found managed instance for pod", "podUID", podUID, "instanceID", instanceID)

	// Get SSM document name
	documentName, err := GetSSMDocumentName()
	if err != nil {
		return "", fmt.Errorf("failed to get SSM document name: %w", err)
	}

	// Start SSM session
	sessionInfo, err := s.ssmClient.StartSession(ctx, instanceID, documentName)
	if err != nil {
		return "", fmt.Errorf("failed to start SSM session: %w", err)
	}

	logger.Info("SSM session started successfully", "instanceID", instanceID, "sessionID", sessionInfo.SessionID)

	// Generate VSCode URL
	url := fmt.Sprintf("%s?sessionId=%s&sessionToken=%s&streamUrl=%s&workspaceName=%s&namespace=%s&eksClusterArn=%s",
		VSCodeScheme,
		sessionInfo.SessionID,
		sessionInfo.TokenValue,
		sessionInfo.StreamURL,
		workspaceName,
		namespace,
		eksClusterARN)

	return url, nil
}
