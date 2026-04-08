/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package awsadapter

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

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
)

const (
	bashCommand = "bash"

	// Test values for context keys
	testSidecarContainer         = "ssm-agent-sidecar"
	testWorkspaceContainer       = "workspace"
	testRegistrationStateFile    = "/opt/amazon/sagemaker/workspace/.ssm-registration-state.json"
	testRegistrationMarkerFile   = "/tmp/ssm-registered"
	testRegistrationScript       = "/usr/local/bin/register-ssm.sh"
	testRemoteAccessServerScript = "/opt/amazon/sagemaker/workspace/remote-access/start-remote-access-server.sh"
	testRemoteAccessServerPort   = "2222"
)

func testPodEventsContext() map[string]string {
	return map[string]string{
		ContextKeyPodUID.Key:                   "test-pod-uid-123",
		ContextKeySsmManagedNodeRole.Key:       "arn:aws:iam::123456789012:role/SSMManagedInstanceCore",
		ContextKeySidecarContainerName.Key:     testSidecarContainer,
		ContextKeyWorkspaceContainerName.Key:   testWorkspaceContainer,
		ContextKeyRegistrationStateFile.Key:    testRegistrationStateFile,
		ContextKeyRegistrationMarkerFile.Key:   testRegistrationMarkerFile,
		ContextKeyRegistrationScript.Key:       testRegistrationScript,
		ContextKeyRemoteAccessServerScript.Key: testRemoteAccessServerScript,
		ContextKeyRemoteAccessServerPort.Key:   testRemoteAccessServerPort,
		ContextKeyRegion.Key:                   "us-west-2",
	}
}

// Test helper functions for common mocking patterns

// mockReadStateFile mocks reading the state file from the sidecar container
func mockReadStateFile(mockPodExec *MockPodExecUtil, state *RegistrationState) {
	if state == nil {
		// File doesn't exist
		mockPodExec.On("ExecInPod",
			mock.Anything,
			mock.Anything,
			testSidecarContainer,
			[]string{"cat", testRegistrationStateFile},
			"",
		).Return("", errors.New("file not found")).Once()
	} else {
		// File exists with state
		stateJSON, _ := json.Marshal(state)
		mockPodExec.On("ExecInPod",
			mock.Anything,
			mock.Anything,
			testSidecarContainer,
			[]string{"cat", testRegistrationStateFile},
			"",
		).Return(string(stateJSON), nil).Once()
	}
}

// mockWriteStateFile mocks writing the state file to the sidecar container
func mockWriteStateFile(mockPodExec *MockPodExecUtil, expectedState *RegistrationState) {
	mockPodExec.On("ExecInPod",
		mock.Anything,
		mock.Anything,
		testSidecarContainer,
		mock.MatchedBy(func(cmd []string) bool {
			if len(cmd) != 3 || cmd[0] != bashCommand || cmd[1] != "-c" {
				return false
			}
			if !strings.Contains(cmd[2], "echo") || !strings.Contains(cmd[2], testRegistrationStateFile) {
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

// mockCleanup mocks plugin deregister-node-agent
func mockCleanup(mockPluginClient *MockPluginRemoteAccessClient) {
	mockPluginClient.On("DeregisterNodeAgent", mock.Anything, mock.Anything).Return(&pluginapi.DeregisterNodeAgentResponse{}, nil).Once()
}

// mockSSMRegistration mocks SSM agent registration in sidecar
func mockSSMRegistration(mockPodExec *MockPodExecUtil) {
	mockPodExec.On("ExecInPod",
		mock.Anything,
		mock.Anything,
		testSidecarContainer,
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
		testSidecarContainer,
		[]string{"touch", testRegistrationMarkerFile},
		"",
	).Return("", nil).Once()
}

// mockWorkspaceSetup mocks starting the remote access server in workspace container
func mockWorkspaceSetup(mockPodExec *MockPodExecUtil) {
	mockPodExec.On("ExecInPod",
		mock.Anything,
		mock.Anything,
		testWorkspaceContainer,
		[]string{testRemoteAccessServerScript, "--port", testRemoteAccessServerPort},
		"",
	).Return("", nil).Once()
}

// mockRegisterNodeAgent mocks plugin register-node-agent
func mockRegisterNodeAgent(mockPluginClient *MockPluginRemoteAccessClient) {
	mockPluginClient.On("RegisterNodeAgent",
		mock.Anything, mock.Anything,
	).Return(&pluginapi.RegisterNodeAgentResponse{
		ActivationID:   "test-activation-id",
		ActivationCode: "test-activation-code",
	}, nil).Once()
}

// Mock implementations for testing

// MockPluginRemoteAccessClient implements plugin.RemoteAccessPluginApis for testing
type MockPluginRemoteAccessClient struct {
	mock.Mock
}

func (m *MockPluginRemoteAccessClient) Initialize(ctx context.Context, req *pluginapi.InitializeRequest) (*pluginapi.InitializeResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pluginapi.InitializeResponse), args.Error(1)
}

func (m *MockPluginRemoteAccessClient) RegisterNodeAgent(ctx context.Context, req *pluginapi.RegisterNodeAgentRequest) (*pluginapi.RegisterNodeAgentResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pluginapi.RegisterNodeAgentResponse), args.Error(1)
}

func (m *MockPluginRemoteAccessClient) DeregisterNodeAgent(ctx context.Context, req *pluginapi.DeregisterNodeAgentRequest) (*pluginapi.DeregisterNodeAgentResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pluginapi.DeregisterNodeAgentResponse), args.Error(1)
}

func (m *MockPluginRemoteAccessClient) CreateSession(ctx context.Context, req *pluginapi.CreateSessionRequest) (*pluginapi.CreateSessionResponse, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*pluginapi.CreateSessionResponse), args.Error(1)
}

// MockPodExecUtil implements PodExecInterface for testing
type MockPodExecUtil struct {
	mock.Mock
}

func (m *MockPodExecUtil) ExecInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (string, error) {
	args := m.Called(ctx, pod, containerName, cmd, stdin)
	return args.String(0), args.Error(1)
}

func TestNewAwsSsmPodEventAdapter(t *testing.T) {
	mockPluginClient := &MockPluginRemoteAccessClient{}
	mockPodExecUtil := &MockPodExecUtil{}

	handler, err := NewAwsSsmPodEventAdapter(mockPluginClient, mockPodExecUtil)

	assert.NoError(t, err)
	assert.NotNil(t, handler)
	assert.Equal(t, mockPluginClient, handler.pluginClient)
	assert.Equal(t, mockPodExecUtil, handler.podExecUtil)
}

func TestNewAwsSsmPodEventAdapter_NilPodExec(t *testing.T) {
	mockPluginClient := &MockPluginRemoteAccessClient{}

	_, err := NewAwsSsmPodEventAdapter(mockPluginClient, nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "podExecUtil is required")
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

// HandlePodRunning Tests

func TestHandlePodRunning_FirstTimeSetup_NoStateFile(t *testing.T) {
	mockPodExecUtil := &MockPodExecUtil{}
	mockPluginClient := &MockPluginRemoteAccessClient{}

	// Mock 1: Read state file - doesn't exist
	mockReadStateFile(mockPodExecUtil, nil)

	// Mock 2: Write state - mark in progress
	inProgressState := &RegistrationState{
		SidecarRestartCount:   0,
		WorkspaceRestartCount: 0,
		SetupInProgress:       true,
	}
	mockWriteStateFile(mockPodExecUtil, inProgressState)

	// Mock 3: Plugin register-node-agent and registration
	mockRegisterNodeAgent(mockPluginClient)
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

	handler, err := NewAwsSsmPodEventAdapter(mockPluginClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create pod with running containers (restart count = 0)
	containerStatuses := []corev1.ContainerStatus{
		{
			Name:         testSidecarContainer,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0,
		},
		{
			Name:         testWorkspaceContainer,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0,
		},
	}
	pod := createTestPod(containerStatuses)
	err = handler.HandlePodRunning(context.Background(), pod, "test-workspace", "test-namespace", testPodEventsContext())

	assert.NoError(t, err)
	mockPodExecUtil.AssertExpectations(t)
	mockPluginClient.AssertExpectations(t)
}

func TestHandlePodRunning_SidecarContainerRestart(t *testing.T) {
	mockPodExecUtil := &MockPodExecUtil{}
	mockPluginClient := &MockPluginRemoteAccessClient{}

	// Mock 1: Read existing state (sidecar restart count = 1)
	existingState := &RegistrationState{
		SidecarRestartCount:   1,
		WorkspaceRestartCount: 0,
		SetupInProgress:       false,
	}
	mockReadStateFile(mockPodExecUtil, existingState)

	// Mock 2: Cleanup (because sidecar restarted)
	mockCleanup(mockPluginClient)

	// Mock 3: Write state - mark in progress
	inProgressState := &RegistrationState{
		SidecarRestartCount:   2,
		WorkspaceRestartCount: 0,
		SetupInProgress:       true,
	}
	mockWriteStateFile(mockPodExecUtil, inProgressState)

	// Mock 4: Setup sidecar only (no workspace setup)
	mockRegisterNodeAgent(mockPluginClient)
	mockSSMRegistration(mockPodExecUtil)
	mockMarkerFile(mockPodExecUtil)

	// Mock 5: Write state - mark complete
	completeState := &RegistrationState{
		SidecarRestartCount:   2,
		WorkspaceRestartCount: 0,
		SetupInProgress:       false,
	}
	mockWriteStateFile(mockPodExecUtil, completeState)

	handler, err := NewAwsSsmPodEventAdapter(mockPluginClient, mockPodExecUtil)
	assert.NoError(t, err)

	// Create pod with sidecar restarted (count = 2)
	containerStatuses := []corev1.ContainerStatus{
		{
			Name:         testSidecarContainer,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 2, // Sidecar restarted
		},
		{
			Name:         testWorkspaceContainer,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0, // Workspace not restarted
		},
	}
	pod := createTestPod(containerStatuses)
	err = handler.HandlePodRunning(context.Background(), pod, "test-workspace", "test-namespace", testPodEventsContext())

	assert.NoError(t, err)
	mockPodExecUtil.AssertExpectations(t)
	mockPluginClient.AssertExpectations(t)
}

func TestHandlePodRunning_NoRestartDetected_AlreadySetup(t *testing.T) {
	mockPodExecUtil := &MockPodExecUtil{}
	mockPluginClient := &MockPluginRemoteAccessClient{}

	// Mock 1: Read existing state - restart counts match current pod status
	existingState := &RegistrationState{
		SidecarRestartCount:   0,
		WorkspaceRestartCount: 0,
		SetupInProgress:       false,
	}
	mockReadStateFile(mockPodExecUtil, existingState)

	handler, err := NewAwsSsmPodEventAdapter(mockPluginClient, mockPodExecUtil)
	assert.NoError(t, err)

	containerStatuses := []corev1.ContainerStatus{
		{
			Name:         testSidecarContainer,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0,
		},
		{
			Name:         testWorkspaceContainer,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0,
		},
	}
	pod := createTestPod(containerStatuses)
	err = handler.HandlePodRunning(context.Background(), pod, "test-workspace", "test-namespace", testPodEventsContext())

	assert.NoError(t, err)
	mockPodExecUtil.AssertExpectations(t)
	mockPluginClient.AssertExpectations(t)
}

func TestHandlePodRunning_SidecarNotRunning(t *testing.T) {
	mockPodExecUtil := &MockPodExecUtil{}
	mockPluginClient := &MockPluginRemoteAccessClient{}

	handler, err := NewAwsSsmPodEventAdapter(mockPluginClient, mockPodExecUtil)
	assert.NoError(t, err)

	containerStatuses := []corev1.ContainerStatus{
		{
			Name: testSidecarContainer,
			State: corev1.ContainerState{
				Running: nil,
			},
			Ready:        false,
			RestartCount: 0,
		},
		{
			Name:         testWorkspaceContainer,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0,
		},
	}
	pod := createTestPod(containerStatuses)
	err = handler.HandlePodRunning(context.Background(), pod, "test-workspace", "test-namespace", testPodEventsContext())

	assert.NoError(t, err)
	mockPodExecUtil.AssertExpectations(t)
	mockPluginClient.AssertExpectations(t)
}

func TestHandlePodRunning_SetupInProgress(t *testing.T) {
	mockPodExecUtil := &MockPodExecUtil{}
	mockPluginClient := &MockPluginRemoteAccessClient{}

	existingState := &RegistrationState{
		SidecarRestartCount:   0,
		WorkspaceRestartCount: 0,
		SetupInProgress:       true,
	}
	mockReadStateFile(mockPodExecUtil, existingState)

	handler, err := NewAwsSsmPodEventAdapter(mockPluginClient, mockPodExecUtil)
	assert.NoError(t, err)

	containerStatuses := []corev1.ContainerStatus{
		{
			Name:         testSidecarContainer,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0,
		},
		{
			Name:         testWorkspaceContainer,
			State:        corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
			Ready:        true,
			RestartCount: 0,
		},
	}
	pod := createTestPod(containerStatuses)
	err = handler.HandlePodRunning(context.Background(), pod, "test-workspace", "test-namespace", testPodEventsContext())

	assert.NoError(t, err)
	mockPodExecUtil.AssertExpectations(t)
	mockPluginClient.AssertExpectations(t)
}

// HandlePodDeleted Tests

func TestHandlePodDeleted_PluginClientNotAvailable(t *testing.T) {
	mockPodExecUtil := &MockPodExecUtil{}
	handler := &AwsSsmPodEventAdapter{
		pluginClient: nil,
		podExecUtil:  mockPodExecUtil,
	}

	pod := createTestPod([]corev1.ContainerStatus{})

	err := handler.HandlePodDeleted(context.Background(), pod, testPodEventsContext())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "plugin client not available")
}

func TestHandlePodDeleted_Success(t *testing.T) {
	mockPluginClient := &MockPluginRemoteAccessClient{}
	mockPodExecUtil := &MockPodExecUtil{}

	mockPluginClient.On("DeregisterNodeAgent", mock.Anything, mock.Anything).Return(&pluginapi.DeregisterNodeAgentResponse{}, nil)

	handler, err := NewAwsSsmPodEventAdapter(mockPluginClient, mockPodExecUtil)
	assert.NoError(t, err)

	pod := createTestPod([]corev1.ContainerStatus{})

	err = handler.HandlePodDeleted(context.Background(), pod, testPodEventsContext())

	assert.NoError(t, err)
	mockPluginClient.AssertExpectations(t)
}

func TestHandlePodDeleted_CleanupFailure(t *testing.T) {
	mockPluginClient := &MockPluginRemoteAccessClient{}
	mockPodExecUtil := &MockPodExecUtil{}

	expectedError := errors.New("plugin: deregister failed")
	mockPluginClient.On("DeregisterNodeAgent", mock.Anything, mock.Anything).Return(nil, expectedError)

	handler, err := NewAwsSsmPodEventAdapter(mockPluginClient, mockPodExecUtil)
	assert.NoError(t, err)

	pod := createTestPod([]corev1.ContainerStatus{})

	err = handler.HandlePodDeleted(context.Background(), pod, testPodEventsContext())

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "deregister failed")
	mockPluginClient.AssertExpectations(t)
}
