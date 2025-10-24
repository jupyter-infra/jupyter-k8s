package aws

import (
	"context"
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

// MockPodExecUtil implements PodExecInterface for testing
type MockPodExecUtil struct {
	mock.Mock
}

func (m *MockPodExecUtil) ExecInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string) (string, error) {
	args := m.Called(ctx, pod, containerName, cmd)
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

func createTestAccessStrategy(controllerConfig map[string]string) *workspacev1alpha1.WorkspaceAccessStrategy {
	return &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name: "aws-ssm-remote-access",
		},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			ControllerConfig: controllerConfig,
		},
	}
}

// InitSSMAgent Tests

func TestInitSSMAgent_SSMClientNotAvailable(t *testing.T) {
	// Create strategy directly with nil SSM client (bypass constructor)
	mockPodExecUtil := &MockPodExecUtil{}
	strategy := &SSMRemoteAccessStrategy{
		ssmClient:   nil, // Explicitly nil
		podExecUtil: mockPodExecUtil,
	}

	// Create test objects (container status doesn't matter since we return early)
	pod := createTestPod([]corev1.ContainerStatus{})
	workspace := createTestWorkspace()
	accessStrategy := createTestAccessStrategy(nil)

	// Test InitSSMAgent with nil SSM client
	err := strategy.InitSSMAgent(context.Background(), pod, workspace, accessStrategy)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "SSM client not available")
}

func TestInitSSMAgent_SidecarNotFound(t *testing.T) {
	// Create mocks (we won't reach them due to early return)
	mockSSMClient := &MockSSMRemoteAccessClient{}
	mockPodExecUtil := &MockPodExecUtil{}

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create pod with no SSM sidecar container
	containerStatuses := []corev1.ContainerStatus{
		{
			Name: "main-container",
			State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{},
			},
		},
		{
			Name: "other-sidecar",
			State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{},
			},
		},
	}
	pod := createTestPod(containerStatuses)
	workspace := createTestWorkspace()
	accessStrategy := createTestAccessStrategy(nil)

	// Test InitSSMAgent with no SSM sidecar
	err = strategy.InitSSMAgent(context.Background(), pod, workspace, accessStrategy)

	assert.NoError(t, err) // Should return nil (graceful exit)
}

func TestInitSSMAgent_SidecarNotRunning(t *testing.T) {
	// Create mocks (we won't reach them due to early return)
	mockSSMClient := &MockSSMRemoteAccessClient{}
	mockPodExecUtil := &MockPodExecUtil{}

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create pod with SSM sidecar that is not running
	containerStatuses := []corev1.ContainerStatus{
		{
			Name: SSMAgentSidecarContainerName,
			State: corev1.ContainerState{
				Running: nil, // Not running
			},
			Ready: false,
		},
	}
	pod := createTestPod(containerStatuses)
	workspace := createTestWorkspace()
	accessStrategy := createTestAccessStrategy(nil)

	// Test InitSSMAgent with non-running sidecar
	err = strategy.InitSSMAgent(context.Background(), pod, workspace, accessStrategy)

	assert.NoError(t, err) // Should return nil (graceful exit)
}

func TestInitSSMAgent_AlreadyCompleted(t *testing.T) {
	// Create mock PodExecUtil that simulates completed registration
	mockPodExecUtil := &MockPodExecUtil{}
	mockPodExecUtil.On("ExecInPod", mock.Anything, mock.Anything, SSMAgentSidecarContainerName,
		[]string{"test", "-f", SSMRegistrationMarkerFile}).Return("", nil) // File exists

	mockSSMClient := &MockSSMRemoteAccessClient{}
	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create pod with running SSM sidecar
	containerStatuses := []corev1.ContainerStatus{
		{
			Name: SSMAgentSidecarContainerName,
			State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{},
			},
			Ready: true,
		},
	}
	pod := createTestPod(containerStatuses)
	workspace := createTestWorkspace()
	accessStrategy := createTestAccessStrategy(nil)

	// Test InitSSMAgent with already completed registration
	err = strategy.InitSSMAgent(context.Background(), pod, workspace, accessStrategy)

	assert.NoError(t, err) // Should return nil (graceful exit)
	mockPodExecUtil.AssertExpectations(t)
}

func TestInitSSMAgent_SuccessFlow(t *testing.T) {
	// Create mock PodExecUtil that simulates successful registration flow
	mockPodExecUtil := &MockPodExecUtil{}

	// First call: check if registration completed (return error = not completed)
	mockPodExecUtil.On("ExecInPod", mock.Anything, mock.Anything, SSMAgentSidecarContainerName,
		[]string{"test", "-f", SSMRegistrationMarkerFile}).Return("", errors.New("file not found"))

	// Second call: registration script execution
	mockPodExecUtil.On("ExecInPod", mock.Anything, mock.Anything, SSMAgentSidecarContainerName,
		mock.MatchedBy(func(cmd []string) bool {
			return len(cmd) == 3 && cmd[0] == bashCommand && cmd[1] == "-c" &&
				strings.Contains(cmd[2], "register-ssm.sh")
		})).Return("", nil)

	// Third call: remote access server start
	mockPodExecUtil.On("ExecInPod", mock.Anything, mock.Anything, WorkspaceContainerName,
		mock.MatchedBy(func(cmd []string) bool {
			return len(cmd) == 3 && cmd[0] == bashCommand && cmd[1] == "-c" &&
				strings.Contains(cmd[2], "remote-access-server")
		})).Return("", nil)

	// Fourth call: completion marker creation
	mockPodExecUtil.On("ExecInPod", mock.Anything, mock.Anything, SSMAgentSidecarContainerName,
		[]string{"touch", SSMRegistrationMarkerFile}).Return("", nil)

	// Create mock SSM client
	mockSSMClient := &MockSSMRemoteAccessClient{}
	mockSSMClient.On("GetRegion").Return("us-west-2")
	mockSSMClient.On("CreateActivation", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&SSMActivation{
			ActivationId:   "test-activation-id",
			ActivationCode: "test-activation-code",
		}, nil)

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create pod with running SSM sidecar
	containerStatuses := []corev1.ContainerStatus{
		{
			Name: SSMAgentSidecarContainerName,
			State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{},
			},
			Ready: true,
		},
	}
	pod := createTestPod(containerStatuses)
	workspace := createTestWorkspace()
	accessStrategy := createTestAccessStrategy(map[string]string{
		"SSM_MANAGED_NODE_ROLE": "arn:aws:iam::123456789012:role/SSMManagedInstanceCore",
	})

	// Test successful InitSSMAgent flow
	err = strategy.InitSSMAgent(context.Background(), pod, workspace, accessStrategy)

	assert.NoError(t, err)
	mockPodExecUtil.AssertExpectations(t)
	mockSSMClient.AssertExpectations(t)
}

func TestInitSSMAgent_RegistrationFailure(t *testing.T) {
	// Create mock PodExecUtil that simulates registration failure
	mockPodExecUtil := &MockPodExecUtil{}

	// First call: check if registration completed (return error = not completed)
	mockPodExecUtil.On("ExecInPod", mock.Anything, mock.Anything, SSMAgentSidecarContainerName,
		[]string{"test", "-f", SSMRegistrationMarkerFile}).Return("", errors.New("file not found"))

	// Second call: registration script execution fails
	expectedError := errors.New("registration script failed")
	mockPodExecUtil.On("ExecInPod", mock.Anything, mock.Anything, SSMAgentSidecarContainerName,
		mock.MatchedBy(func(cmd []string) bool {
			return len(cmd) == 3 && cmd[0] == bashCommand && cmd[1] == "-c" &&
				strings.Contains(cmd[2], "register-ssm.sh")
		})).Return("", expectedError)

	// Create mock SSM client
	mockSSMClient := &MockSSMRemoteAccessClient{}
	mockSSMClient.On("GetRegion").Return("us-west-2")
	mockSSMClient.On("CreateActivation", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&SSMActivation{
			ActivationId:   "test-activation-id",
			ActivationCode: "test-activation-code",
		}, nil)

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create pod with running SSM sidecar
	containerStatuses := []corev1.ContainerStatus{
		{
			Name: SSMAgentSidecarContainerName,
			State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{},
			},
			Ready: true,
		},
	}
	pod := createTestPod(containerStatuses)
	workspace := createTestWorkspace()
	accessStrategy := createTestAccessStrategy(map[string]string{
		"SSM_MANAGED_NODE_ROLE": "arn:aws:iam::123456789012:role/SSMManagedInstanceCore",
	})

	// Test InitSSMAgent with registration failure
	err = strategy.InitSSMAgent(context.Background(), pod, workspace, accessStrategy)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to execute SSM registration script")
	mockPodExecUtil.AssertExpectations(t)
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

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create test pod
	pod := createTestPod([]corev1.ContainerStatus{})

	// Test successful cleanup
	err = strategy.CleanupSSMManagedNodes(context.Background(), pod)

	assert.NoError(t, err)
	mockSSMClient.AssertExpectations(t)

	// Verify the method was called with the correct pod UID
	mockSSMClient.AssertCalled(t, "CleanupManagedInstancesByPodUID", mock.Anything, "test-pod-uid-123")
}

func TestCleanupSSMManagedNodes_CleanupFailure(t *testing.T) {
	// Create mocks
	mockSSMClient := &MockSSMRemoteAccessClient{}
	mockPodExecUtil := &MockPodExecUtil{}

	// Set up expectation for cleanup failure
	expectedError := errors.New("AWS API error: instance not found")
	mockSSMClient.On("CleanupManagedInstancesByPodUID", mock.Anything, "test-pod-uid-123").Return(expectedError)

	strategy, err := NewSSMRemoteAccessStrategy(mockSSMClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create test pod
	pod := createTestPod([]corev1.ContainerStatus{})

	// Test cleanup failure
	err = strategy.CleanupSSMManagedNodes(context.Background(), pod)

	assert.Error(t, err)
	assert.Equal(t, expectedError, err) // Should return the exact same error
	mockSSMClient.AssertExpectations(t)

	// Verify the method was called with the correct pod UID
	mockSSMClient.AssertCalled(t, "CleanupManagedInstancesByPodUID", mock.Anything, "test-pod-uid-123")
}
