package aws

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

const (
	bashCommand = "bash"
)

// Test helper functions for common mocking patterns

// mockReadStateFile mocks reading the state file from the sidecar container
func mockReadStateFile(mockPodExec *MockPodExecUtil, state *RegistrationState) {
	if state == nil {
		// File doesn't exist
		mockPodExec.On("ExecInPod",
			mock.Anything,
			mock.Anything,
			SSMAgentSidecarContainerName,
			[]string{"cat", SSMRegistrationStateFile},
			"",
		).Return("", errors.New("file not found")).Once()
	} else {
		// File exists with state
		stateJSON, _ := json.Marshal(state)
		mockPodExec.On("ExecInPod",
			mock.Anything,
			mock.Anything,
			SSMAgentSidecarContainerName,
			[]string{"cat", SSMRegistrationStateFile},
			"",
		).Return(string(stateJSON), nil).Once()
	}
}

// mockWriteStateFile mocks writing the state file to the sidecar container
func mockWriteStateFile(mockPodExec *MockPodExecUtil, expectedState *RegistrationState) {
	mockPodExec.On("ExecInPod",
		mock.Anything,
		mock.Anything,
		SSMAgentSidecarContainerName,
		mock.MatchedBy(func(cmd []string) bool {
			if len(cmd) != 3 || cmd[0] != bashCommand || cmd[1] != "-c" {
				return false
			}
			if !strings.Contains(cmd[2], "echo") || !strings.Contains(cmd[2], SSMRegistrationStateFile) {
				return false
			}
			// Extract and verify JSON
			if strings.Contains(cmd[2], "echo '") {
				start := strings.Index(cmd[2], "echo '") + 6
				end := strings.Index(cmd[2][start:], "'")
				if end > 0 {
					jsonStr := cmd[2][start : start+end]
					var state RegistrationState
					if err := json.Unmarshal([]byte(jsonStr), &state); err != nil {
						return false
					}
					return state.SidecarRestartCount == expectedState.SidecarRestartCount &&
						state.WorkspaceRestartCount == expectedState.WorkspaceRestartCount &&
						state.SetupInProgress == expectedState.SetupInProgress
				}
			}
			return false
		}),
		"",
	).Return("", nil).Once()
}

// mockCleanup mocks SSM cleanup operations
func mockCleanup(mockSSMClient *MockSSMRemoteAccessClient, podUID string) {
	mockSSMClient.On("CleanupManagedInstancesByPodUID", mock.Anything, podUID).Return(nil).Once()
	mockSSMClient.On("CleanupActivationsByPodUID", mock.Anything, podUID).Return(nil).Once()
}

// mockSSMRegistration mocks SSM agent registration in sidecar
func mockSSMRegistration(mockPodExec *MockPodExecUtil) {
	mockPodExec.On("ExecInPod",
		mock.Anything,
		mock.Anything,
		SSMAgentSidecarContainerName,
		mock.MatchedBy(func(cmd []string) bool {
			return len(cmd) == 3 && cmd[0] == "bash" && cmd[1] == "-c" &&
				strings.Contains(cmd[2], "register-ssm.sh")
		}),
		mock.MatchedBy(func(stdin string) bool {
			return strings.Contains(stdin, "test-activation-id")
		}),
	).Return("", nil).Once()
}

// mockMarkerFile mocks marker file creation for backward compatibility
func mockMarkerFile(mockPodExec *MockPodExecUtil) {
	mockPodExec.On("ExecInPod",
		mock.Anything,
		mock.Anything,
		SSMAgentSidecarContainerName,
		[]string{"touch", SSMRegistrationMarkerFile},
		"",
	).Return("", nil).Once()
}

// mockWorkspaceSetup mocks starting the remote access server in workspace container
func mockWorkspaceSetup(mockPodExec *MockPodExecUtil) {
	mockPodExec.On("ExecInPod",
		mock.Anything,
		mock.Anything,
		WorkspaceContainerName,
		[]string{RemoteAccessServerScriptPath, "--port", RemoteAccessServerPort},
		"",
	).Return("", nil).Once()
}

// mockSSMActivation mocks SSM activation creation
func mockSSMActivation(mockSSMClient *MockSSMRemoteAccessClient) {
	mockSSMClient.On("GetRegion").Return("us-west-2")
	mockSSMClient.On("CreateActivation",
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
	).Return(&SSMActivation{
		ActivationId:   "test-activation-id",
		ActivationCode: "test-activation-code",
	}, nil).Once()
}

// Mock implementations for testing

// MockSSMRemoteAccessClient implements SSMRemoteAccessClientInterface for testing
type MockSSMRemoteAccessClient struct {
	mock.Mock
}

func (m *MockSSMRemoteAccessClient) CreateActivation(ctx context.Context, description string, instanceName string, iamRole string, tags map[string]string) (*SSMActivation, error) {
	args := m.Called(ctx, description, instanceName, iamRole, tags)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SSMActivation), args.Error(1)
}

func (m *MockSSMRemoteAccessClient) GetRegion() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockSSMRemoteAccessClient) CleanupManagedInstancesByPodUID(ctx context.Context, podUID string) error {
	args := m.Called(ctx, podUID)
	return args.Error(0)
}

func (m *MockSSMRemoteAccessClient) CleanupActivationsByPodUID(ctx context.Context, podUID string) error {
	args := m.Called(ctx, podUID)
	return args.Error(0)
}

func (m *MockSSMRemoteAccessClient) FindInstanceByPodUID(ctx context.Context, podUID string) (string, error) {
	args := m.Called(ctx, podUID)
	return args.String(0), args.Error(1)
}

func (m *MockSSMRemoteAccessClient) StartSession(ctx context.Context, instanceID, documentName, port string) (*SessionInfo, error) {
	args := m.Called(ctx, instanceID, documentName, port)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*SessionInfo), args.Error(1)
}

// MockPodExecUtil implements PodExecInterface for testing
type MockPodExecUtil struct {
	mock.Mock
}

func (m *MockPodExecUtil) ExecInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (string, error) {
	args := m.Called(ctx, pod, containerName, cmd, stdin)
	return args.String(0), args.Error(1)
}

func TestNewSSMRemoteAccessStrategy_WithDependencyInjection(t *testing.T) {
	// Create mock dependencies
	mockSSMClient := &MockSSMRemoteAccessClient{}
	mockPodExecUtil := &MockPodExecUtil{}

	// Test creation with dependency injection
	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)

	assert.NoError(t, err)
	assert.NotNil(t, strategy)
	assert.Equal(t, mockSSMClient, strategy.ssmClient)
	assert.Equal(t, mockPodExecUtil, strategy.podExecUtil)
}

func TestNewSSMRemoteAccessStrategy_WithDefaults(t *testing.T) {
	// Create mock PodExecUtil
	mockPodExecUtil := &MockPodExecUtil{}

	// Test creation with nil SSM client (should create default)
	strategy, err := NewSSMRemoteAccessStrategy(nil, mockPodExecUtil)

	assert.NoError(t, err)
	assert.NotNil(t, strategy)
	assert.NotNil(t, strategy.ssmClient) // Should have created default SSM client
	assert.Equal(t, mockPodExecUtil, strategy.podExecUtil)
}

func TestNewSSMRemoteAccessStrategy_SSMClientFailure(t *testing.T) {
	// Save original function
	originalNewSSMClient := newSSMClient
	defer func() {
		newSSMClient = originalNewSSMClient
	}()

	// Mock SSM client creation failure
	expectedSSMError := errors.New("failed to initialize AWS config")
	newSSMClient = func(ctx context.Context) (*SSMClient, error) {
		return nil, expectedSSMError
	}

	// Create mock PodExecUtil
	mockPodExecUtil := &MockPodExecUtil{}

	// Test creation with SSM client failure (should still succeed with nil SSM client)
	strategy, err := NewSSMRemoteAccessStrategy(nil, mockPodExecUtil)

	assert.NoError(t, err)
	assert.NotNil(t, strategy)
	assert.Nil(t, strategy.ssmClient) // SSM client should be nil due to failure
	assert.Equal(t, mockPodExecUtil, strategy.podExecUtil)
}

// Test helper functions for creating test objects

func createTestPod(containerStatuses []corev1.ContainerStatus) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "test-namespace",
			UID:       "test-pod-uid-123",
		},
		Status: corev1.PodStatus{
			ContainerStatuses: containerStatuses,
		},
	}
}

func createTestWorkspace() *workspacev1alpha1.Workspace {
	return &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace",
			Namespace: "test-namespace",
		},
	}
}

func createTestAccessStrategy(connectionContext map[string]string) *workspacev1alpha1.WorkspaceAccessStrategy {
	return &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "aws-ssm-remote-access",
		},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			CreateConnectionContext: connectionContext,
		},
	}
}

// SetupContainers Tests

func TestSetupContainers_FirstTimeSetup_NoStateFile(t *testing.T) {
	t.Setenv(EKSClusterARNEnv, "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	mockPodExecUtil := &MockPodExecUtil{}
	mockSSMClient := &MockSSMRemoteAccessClient{}

	// Mock 1: Read state file - doesn't exist
	mockReadStateFile(mockPodExecUtil, nil)

	// Mock 2: Write state - mark in progress
	inProgressState := &RegistrationState{
		SidecarRestartCount:   0,
		WorkspaceRestartCount: 0,
		SetupInProgress:       true,
	}
	mockWriteStateFile(mockPodExecUtil, inProgressState)

	// Mock 3: SSM activation and registration
	mockSSMActivation(mockSSMClient)
	mockSSMRegistration(mockPodExecUtil)
	mockMarkerFile(mockPodExecUtil)

	// Mock 4: Workspace setup
	mockWorkspaceSetup(mockPodExecUtil)

	// Mock 5: Write state - mark complete
	completeState := &RegistrationState{
		SidecarRestartCount:   0,
		WorkspaceRestartCount: 0,
		SetupInProgress:       false,
	}
	mockWriteStateFile(mockPodExecUtil, completeState)

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create pod with running containers (restart count = 0)
	containerStatuses := []corev1.ContainerStatus{
		{
			Name:         SSMAgentSidecarContainerName,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0,
		},
		{
			Name:         WorkspaceContainerName,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0,
		},
	}
	pod := createTestPod(containerStatuses)
	workspace := createTestWorkspace()
	accessStrategy := createTestAccessStrategy(map[string]string{
		"SSM_MANAGED_NODE_ROLE": "arn:aws:iam::123456789012:role/SSMManagedInstanceCore",
	})

	// Execute
	err = strategy.SetupContainers(context.Background(), pod, workspace, accessStrategy)

	// Verify
	assert.NoError(t, err)
	mockPodExecUtil.AssertExpectations(t)
	mockSSMClient.AssertExpectations(t)
}

func TestSetupContainers_SidecarContainerRestart(t *testing.T) {
	t.Setenv(EKSClusterARNEnv, "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	mockPodExecUtil := &MockPodExecUtil{}
	mockSSMClient := &MockSSMRemoteAccessClient{}

	// Mock 1: Read existing state (sidecar restart count = 1)
	existingState := &RegistrationState{
		SidecarRestartCount:   1,
		WorkspaceRestartCount: 0,
		SetupInProgress:       false,
	}
	mockReadStateFile(mockPodExecUtil, existingState)

	// Mock 2: Cleanup (because sidecar restarted)
	mockCleanup(mockSSMClient, "test-pod-uid-123")

	// Mock 3: Write state - mark in progress
	inProgressState := &RegistrationState{
		SidecarRestartCount:   2,
		WorkspaceRestartCount: 0,
		SetupInProgress:       true,
	}
	mockWriteStateFile(mockPodExecUtil, inProgressState)

	// Mock 4: Setup sidecar only (no workspace setup)
	mockSSMActivation(mockSSMClient)
	mockSSMRegistration(mockPodExecUtil)
	mockMarkerFile(mockPodExecUtil)

	// Mock 5: Write state - mark complete
	completeState := &RegistrationState{
		SidecarRestartCount:   2,
		WorkspaceRestartCount: 0,
		SetupInProgress:       false,
	}
	mockWriteStateFile(mockPodExecUtil, completeState)

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create pod with sidecar restarted (count = 2)
	containerStatuses := []corev1.ContainerStatus{
		{
			Name:         SSMAgentSidecarContainerName,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 2, // Sidecar restarted
		},
		{
			Name:         WorkspaceContainerName,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0, // Workspace not restarted
		},
	}
	pod := createTestPod(containerStatuses)
	workspace := createTestWorkspace()
	accessStrategy := createTestAccessStrategy(map[string]string{
		"SSM_MANAGED_NODE_ROLE": "arn:aws:iam::123456789012:role/SSMManagedInstanceCore",
	})

	// Execute
	err = strategy.SetupContainers(context.Background(), pod, workspace, accessStrategy)

	// Verify
	assert.NoError(t, err)
	mockPodExecUtil.AssertExpectations(t)
	mockSSMClient.AssertExpectations(t)
}

func TestSetupContainers_NoRestartDetected_AlreadySetup(t *testing.T) {
	t.Setenv(EKSClusterARNEnv, "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	mockPodExecUtil := &MockPodExecUtil{}
	mockSSMClient := &MockSSMRemoteAccessClient{}

	// Mock 1: Read existing state - restart counts match current pod status
	existingState := &RegistrationState{
		SidecarRestartCount:   0,
		WorkspaceRestartCount: 0,
		SetupInProgress:       false,
	}
	mockReadStateFile(mockPodExecUtil, existingState)

	// NO OTHER MOCKS - should exit early after reading state

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create pod with matching restart counts (both = 0)
	containerStatuses := []corev1.ContainerStatus{
		{
			Name:         SSMAgentSidecarContainerName,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0, // Matches state file
		},
		{
			Name:         WorkspaceContainerName,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0, // Matches state file
		},
	}
	pod := createTestPod(containerStatuses)
	workspace := createTestWorkspace()
	accessStrategy := createTestAccessStrategy(map[string]string{
		"SSM_MANAGED_NODE_ROLE": "arn:aws:iam::123456789012:role/SSMManagedInstanceCore",
	})

	// Execute
	err = strategy.SetupContainers(context.Background(), pod, workspace, accessStrategy)

	// Verify - should exit early with no error
	assert.NoError(t, err)
	// Only 1 call should have been made (reading state file)
	mockPodExecUtil.AssertExpectations(t)
	// No SSM client calls should have been made
	mockSSMClient.AssertExpectations(t)
}

func TestSetupContainers_SidecarNotRunning(t *testing.T) {
	t.Setenv(EKSClusterARNEnv, "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	mockPodExecUtil := &MockPodExecUtil{}
	mockSSMClient := &MockSSMRemoteAccessClient{}

	// NO MOCKS - should exit early before any operations

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create pod with sidecar NOT running (nil Running state)
	containerStatuses := []corev1.ContainerStatus{
		{
			Name: SSMAgentSidecarContainerName,
			State: corev1.ContainerState{
				Running: nil, // Not running
			},
			Ready:        false,
			RestartCount: 0,
		},
		{
			Name:         WorkspaceContainerName,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0,
		},
	}
	pod := createTestPod(containerStatuses)
	workspace := createTestWorkspace()
	accessStrategy := createTestAccessStrategy(map[string]string{
		"ssmManagedNodeRole": "arn:aws:iam::123456789012:role/SSMManagedInstanceCore",
	})

	// Execute
	err = strategy.SetupContainers(context.Background(), pod, workspace, accessStrategy)

	// Verify - should exit early with no error, no operations
	assert.NoError(t, err)
	mockPodExecUtil.AssertExpectations(t) // No calls
	mockSSMClient.AssertExpectations(t)   // No calls
}

func TestSetupContainers_SetupInProgress(t *testing.T) {
	t.Setenv(EKSClusterARNEnv, "arn:aws:eks:us-west-2:123456789012:cluster/test-cluster")

	mockPodExecUtil := &MockPodExecUtil{}
	mockSSMClient := &MockSSMRemoteAccessClient{}

	// Mock 1: Read state file - setup is already in progress
	existingState := &RegistrationState{
		SidecarRestartCount:   0,
		WorkspaceRestartCount: 0,
		SetupInProgress:       true, // Another event is already setting up
	}
	mockReadStateFile(mockPodExecUtil, existingState)

	// NO OTHER MOCKS - should exit early after detecting setup in progress

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create pod with running containers
	containerStatuses := []corev1.ContainerStatus{
		{
			Name:         SSMAgentSidecarContainerName,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0,
		},
		{
			Name:         WorkspaceContainerName,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0,
		},
	}
	pod := createTestPod(containerStatuses)
	workspace := createTestWorkspace()
	accessStrategy := createTestAccessStrategy(map[string]string{
		"ssmManagedNodeRole": "arn:aws:iam::123456789012:role/SSMManagedInstanceCore",
	})

	// Execute
	err = strategy.SetupContainers(context.Background(), pod, workspace, accessStrategy)

	// Verify - should exit early with no error
	assert.NoError(t, err)
	// Only 1 call should have been made (reading state file)
	mockPodExecUtil.AssertExpectations(t)
	// No SSM client calls should have been made
	mockSSMClient.AssertExpectations(t)
}

// CleanupSSMManagedNodes Tests

func TestCleanupSSMManagedNodes_SSMClientNotAvailable(t *testing.T) {
	// Create strategy directly with nil SSM client (bypass constructor)
	mockPodExecUtil := &MockPodExecUtil{}
	strategy := &SSMRemoteAccessStrategy{
		ssmClient:   nil, // Explicitly nil
		podExecUtil: mockPodExecUtil,
	}

	// Create test pod (container status doesn't matter for CleanupSSMManagedNodes)
	pod := createTestPod([]corev1.ContainerStatus{})

	// Test CleanupSSMManagedNodes with nil SSM client
	err := strategy.CleanupSSMManagedNodes(context.Background(), pod)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SSM client not available")
}

func TestCleanupSSMManagedNodes_Success(t *testing.T) {
	// Create mocks
	mockSSMClient := &MockSSMRemoteAccessClient{}
	mockPodExecUtil := &MockPodExecUtil{}

	// Set up expectation for successful cleanup
	mockSSMClient.On("CleanupManagedInstancesByPodUID", mock.Anything, "test-pod-uid-123").Return(nil)
	mockSSMClient.On("CleanupActivationsByPodUID", mock.Anything, "test-pod-uid-123").Return(nil)

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create test pod
	pod := createTestPod([]corev1.ContainerStatus{})

	// Test successful cleanup
	err = strategy.CleanupSSMManagedNodes(context.Background(), pod)

	assert.NoError(t, err)
	mockSSMClient.AssertExpectations(t)

	// Verify the methods were called with the correct pod UID
	mockSSMClient.AssertCalled(t, "CleanupManagedInstancesByPodUID", mock.Anything, "test-pod-uid-123")
	mockSSMClient.AssertCalled(t, "CleanupActivationsByPodUID", mock.Anything, "test-pod-uid-123")
}

func TestCleanupSSMManagedNodes_CleanupFailure(t *testing.T) {
	// Create mocks
	mockSSMClient := &MockSSMRemoteAccessClient{}
	mockPodExecUtil := &MockPodExecUtil{}

	// Set up expectation for cleanup failure
	expectedError := errors.New("AWS API error: activation not found")
	mockSSMClient.On("CleanupManagedInstancesByPodUID", mock.Anything, "test-pod-uid-123").Return(nil)
	mockSSMClient.On("CleanupActivationsByPodUID", mock.Anything, "test-pod-uid-123").Return(expectedError)

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create test pod
	pod := createTestPod([]corev1.ContainerStatus{})

	// Test cleanup failure
	err = strategy.CleanupSSMManagedNodes(context.Background(), pod)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to cleanup activations for pod test-pod-uid-123")
	mockSSMClient.AssertExpectations(t)

	// Verify both methods were called with the correct pod UID
	mockSSMClient.AssertCalled(t, "CleanupManagedInstancesByPodUID", mock.Anything, "test-pod-uid-123")
	mockSSMClient.AssertCalled(t, "CleanupActivationsByPodUID", mock.Anything, "test-pod-uid-123")
}

func TestGenerateVSCodeConnectionURL_Success(t *testing.T) {
	mockSSMClient := &MockSSMRemoteAccessClient{}
	mockPodExecUtil := &MockPodExecUtil{}

	// Mock FindInstanceByPodUID
	mockSSMClient.On("FindInstanceByPodUID", mock.Anything, "test-pod-uid").Return("i-1234567890abcdef0", nil)

	// Mock StartSession
	mockSSMClient.On("StartSession", mock.Anything, "i-1234567890abcdef0", "test-document", RemoteAccessServerPort).Return(
		&SessionInfo{
			SessionID:  "sess-123",
			TokenValue: "token-456",
			StreamURL:  "wss://stream-url",
		}, nil)

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create access strategy with SSM document name
	accessStrategy := createTestAccessStrategy(map[string]string{
		"ssmDocumentName": "test-document",
	})

	url, err := strategy.GenerateVSCodeConnectionURL(context.Background(), "test-workspace", "default", "test-pod-uid", "arn:aws:eks:us-east-1:123456789012:cluster/test", accessStrategy)

	assert.NoError(t, err)
	assert.Contains(t, url, "vscode://amazonwebservices.aws-toolkit-vscode/connect/workspace")
	assert.Contains(t, url, "sessionId=sess-123")
	assert.Contains(t, url, "sessionToken=token-456")
	assert.Contains(t, url, "streamUrl=wss://stream-url")
	mockSSMClient.AssertExpectations(t)
}

func TestGenerateVSCodeConnectionURL_StartSessionError(t *testing.T) {
	mockSSMClient := &MockSSMRemoteAccessClient{}
	mockPodExecUtil := &MockPodExecUtil{}

	// Mock successful FindInstanceByPodUID
	mockSSMClient.On("FindInstanceByPodUID", mock.Anything, "test-pod-uid").Return("i-1234567890abcdef0", nil)

	// Mock StartSession failure
	mockSSMClient.On("StartSession", mock.Anything, "i-1234567890abcdef0", "test-document", RemoteAccessServerPort).Return(nil, errors.New("session start failed"))

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create access strategy with SSM document name
	accessStrategy := createTestAccessStrategy(map[string]string{
		"ssmDocumentName": "test-document",
	})

	url, err := strategy.GenerateVSCodeConnectionURL(context.Background(), "test-workspace", "default", "test-pod-uid", "arn:aws:eks:us-east-1:123456789012:cluster/test", accessStrategy)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to start SSM session")
	assert.Empty(t, url)
}
