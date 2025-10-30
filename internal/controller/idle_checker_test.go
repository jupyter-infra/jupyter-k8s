package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// MockIdleDetector mocks the IdleDetector for testing
type MockIdleDetector struct {
	mock.Mock
}

func (m *MockIdleDetector) CheckIdle(ctx context.Context, workspaceName string, pod *corev1.Pod, idleConfig *workspacev1alpha1.IdleShutdownSpec) (*IdleCheckResult, error) {
	args := m.Called(ctx, workspaceName, pod, idleConfig)
	return args.Get(0).(*IdleCheckResult), args.Error(1)
}

// Test constants
const (
	testWorkspaceCheckerName = "test-workspace"
	testNamespace            = "default"
	testCheckerPodName       = "test-workspace-pod"
	testTimeoutMinutes       = 30
)

// Helper functions
func createTestWorkspace() *workspacev1alpha1.Workspace {
	return &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testWorkspaceCheckerName,
			Namespace: testNamespace,
		},
	}
}

func createTestIdleConfigChecker(timeoutMinutes int) *workspacev1alpha1.IdleShutdownSpec {
	return &workspacev1alpha1.IdleShutdownSpec{
		TimeoutMinutes: timeoutMinutes,
		Detection: workspacev1alpha1.IdleDetectionSpec{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/api/idle",
				Port:   intstr.FromInt(8888),
				Scheme: corev1.URISchemeHTTP,
			},
		},
	}
}

// Test setup structure
type testSetup struct {
	checker      *WorkspaceIdleChecker
	workspace    *workspacev1alpha1.Workspace
	idleConfig   *workspacev1alpha1.IdleShutdownSpec
	mockDetector *MockIdleDetector
	cleanup      func()
}

// setupWorkspaceIdleCheckerTest creates common test setup
func setupWorkspaceIdleCheckerTest(_ *testing.T) *testSetup {
	// Setup scheme
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create test workspace
	workspace := createTestWorkspace()

	// Create a running pod with the expected labels
	runningPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testCheckerPodName,
			Namespace: workspace.Namespace,
			Labels:    GenerateLabels(workspace.Name),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	// Create fake client with pre-populated pod
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(runningPod).
		Build()

	checker := NewWorkspaceIdleChecker(fakeClient)
	idleConfig := createTestIdleConfigChecker(testTimeoutMinutes)

	// Setup mock detector
	mockDetector := &MockIdleDetector{}

	// Override factory function
	originalCreateIdleDetector := CreateIdleDetector
	CreateIdleDetector = func(detection *workspacev1alpha1.IdleDetectionSpec) (IdleDetector, error) {
		return mockDetector, nil
	}

	cleanup := func() {
		CreateIdleDetector = originalCreateIdleDetector
	}

	return &testSetup{
		checker:      checker,
		workspace:    workspace,
		idleConfig:   idleConfig,
		mockDetector: mockDetector,
		cleanup:      cleanup,
	}
}

// Tests for NewWorkspaceIdleChecker
func TestNewWorkspaceIdleChecker_Success(t *testing.T) {
	// Create fake client
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	checker := NewWorkspaceIdleChecker(fakeClient)

	assert.NotNil(t, checker)
	assert.NotNil(t, checker.client)
}

// Test CheckWorkspaceIdle - Success Cases with mocked detector
func TestWorkspaceIdleChecker_CheckWorkspaceIdle_Success_NotIdle(t *testing.T) {
	setup := setupWorkspaceIdleCheckerTest(t)
	defer setup.cleanup()

	// Mock detector to return "not idle"
	setup.mockDetector.On("CheckIdle", mock.Anything, setup.workspace.Name, mock.Anything, setup.idleConfig).
		Return(&IdleCheckResult{IsIdle: false, ShouldRetry: true}, nil)

	// Execute
	result, err := setup.checker.CheckWorkspaceIdle(context.Background(), setup.workspace, setup.idleConfig)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsIdle)     // Not idle (controlled by mock)
	assert.True(t, result.ShouldRetry) // Continue checking (controlled by mock)
	setup.mockDetector.AssertExpectations(t)
}

func TestWorkspaceIdleChecker_CheckWorkspaceIdle_Success_IsIdle(t *testing.T) {
	setup := setupWorkspaceIdleCheckerTest(t)
	defer setup.cleanup()

	// Mock detector to return "is idle"
	setup.mockDetector.On("CheckIdle", mock.Anything, setup.workspace.Name, mock.Anything, setup.idleConfig).
		Return(&IdleCheckResult{IsIdle: true, ShouldRetry: true}, nil)

	// Execute
	result, err := setup.checker.CheckWorkspaceIdle(context.Background(), setup.workspace, setup.idleConfig)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsIdle)      // Is idle (controlled by mock)
	assert.True(t, result.ShouldRetry) // Continue checking (controlled by mock)
	setup.mockDetector.AssertExpectations(t)
}

// Test CheckWorkspaceIdle - Error Cases
func TestWorkspaceIdleChecker_CheckWorkspaceIdle_NoPodFound(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create fake client with NO pods
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	checker := NewWorkspaceIdleChecker(fakeClient)
	workspace := createTestWorkspace()
	idleConfig := createTestIdleConfigChecker(15) // Different timeout for variety

	// Execute
	result, err := checker.CheckWorkspaceIdle(context.Background(), workspace, idleConfig)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find workspace pod")
	assert.Contains(t, err.Error(), "no running pod found for workspace")
	assert.NotNil(t, result)
	assert.False(t, result.IsIdle)
	assert.True(t, result.ShouldRetry) // Should retry on pod not found (temporary failure)
}

func TestWorkspaceIdleChecker_CheckWorkspaceIdle_PodNotRunning(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = workspacev1alpha1.AddToScheme(scheme)

	workspace := createTestWorkspace()

	// Create a pod that is NOT running (Pending state)
	pendingPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testCheckerPodName,
			Namespace: workspace.Namespace,
			Labels:    GenerateLabels(workspace.Name),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodPending, // Not running
		},
	}

	// Create fake client with pending pod
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pendingPod).
		Build()

	checker := NewWorkspaceIdleChecker(fakeClient)
	idleConfig := createTestIdleConfigChecker(testTimeoutMinutes)

	// Execute
	result, err := checker.CheckWorkspaceIdle(context.Background(), workspace, idleConfig)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to find workspace pod")
	assert.Contains(t, err.Error(), "no running pod found for workspace")
	assert.NotNil(t, result)
	assert.False(t, result.IsIdle)
	assert.True(t, result.ShouldRetry) // Should retry when pod not running (temporary failure)
}

func TestWorkspaceIdleChecker_CheckWorkspaceIdle_DetectorCreationFailure(t *testing.T) {
	// Setup scheme
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = workspacev1alpha1.AddToScheme(scheme)

	workspace := createTestWorkspace()

	// Create a running pod
	runningPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testCheckerPodName,
			Namespace: workspace.Namespace,
			Labels:    GenerateLabels(workspace.Name),
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(runningPod).
		Build()

	checker := NewWorkspaceIdleChecker(fakeClient)

	// Override factory function to return error
	originalCreateIdleDetector := CreateIdleDetector
	CreateIdleDetector = func(detection *workspacev1alpha1.IdleDetectionSpec) (IdleDetector, error) {
		return nil, fmt.Errorf("failed to create detector: unsupported detection method")
	}
	defer func() {
		CreateIdleDetector = originalCreateIdleDetector
	}()

	idleConfig := createTestIdleConfigChecker(testTimeoutMinutes)

	// Execute
	result, err := checker.CheckWorkspaceIdle(context.Background(), workspace, idleConfig)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create idle detector")
	assert.Contains(t, err.Error(), "unsupported detection method")
	assert.NotNil(t, result)
	assert.False(t, result.IsIdle)
	assert.False(t, result.ShouldRetry) // Should NOT retry on detector creation failure (permanent failure)
}

func TestWorkspaceIdleChecker_CheckWorkspaceIdle_DetectorExecutionError(t *testing.T) {
	setup := setupWorkspaceIdleCheckerTest(t)
	defer setup.cleanup()

	// Mock detector to return an error
	setup.mockDetector.On("CheckIdle", mock.Anything, setup.workspace.Name, mock.Anything, setup.idleConfig).
		Return(&IdleCheckResult{IsIdle: false, ShouldRetry: true}, fmt.Errorf("detector execution failed: connection timeout"))

	// Execute
	result, err := setup.checker.CheckWorkspaceIdle(context.Background(), setup.workspace, setup.idleConfig)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "detector execution failed: connection timeout")
	assert.NotNil(t, result)
	assert.False(t, result.IsIdle)     // Not idle when error occurs
	assert.True(t, result.ShouldRetry) // Should retry based on detector response
	setup.mockDetector.AssertExpectations(t)
}
