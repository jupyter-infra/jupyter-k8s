package controller

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// Test constants
const (
	testWorkspaceName = "test-workspace"
	testPodName       = "test-pod"
)

// Helper functions
func createTestPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testPodName,
			Namespace: "default",
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}
}

func createTestIdleConfig(timeoutMinutes int) *workspacev1alpha1.IdleShutdownSpec {
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

// Tests for CreateIdleDetector
func TestCreateIdleDetector_HTTPGet_Success(t *testing.T) {
	detection := &workspacev1alpha1.IdleDetectionSpec{
		HTTPGet: &corev1.HTTPGetAction{
			Path: "/api/idle",
			Port: intstr.FromInt(8888),
		},
	}

	detector, err := CreateIdleDetector(detection)

	assert.NoError(t, err)
	assert.NotNil(t, detector)
	assert.IsType(t, &HTTPGetDetector{}, detector)
}

func TestCreateIdleDetector_NoDetectionMethod_Error(t *testing.T) {
	detection := &workspacev1alpha1.IdleDetectionSpec{}

	detector, err := CreateIdleDetector(detection)

	assert.Error(t, err)
	assert.Nil(t, detector)
	assert.Contains(t, err.Error(), "no detection method configured")
}

// Tests for NewHTTPGetDetector
func TestNewHTTPGetDetectorWithExec_Success(t *testing.T) {
	mockExecUtil := &MockPodExecUtil{}
	detector := NewHTTPGetDetectorWithExec(mockExecUtil)

	assert.NotNil(t, detector)
	assert.NotNil(t, detector.execUtil)
	assert.Equal(t, mockExecUtil, detector.execUtil)
}

// Mock for testing HTTPGetDetector.CheckIdle
type MockPodExecUtil struct {
	mock.Mock
}

func (m *MockPodExecUtil) ExecInPod(ctx context.Context, pod *corev1.Pod, containerName string, cmd []string, stdin string) (string, error) {
	args := m.Called(ctx, pod, containerName, cmd, stdin)
	return args.String(0), args.Error(1)
}

// Helper to create HTTPGetDetector with mock
func createDetectorWithMock(mockExecUtil *MockPodExecUtil) *HTTPGetDetector {
	return NewHTTPGetDetectorWithExec(mockExecUtil)
}

// Test HTTPGetDetector.checkIdleTimeout (private method but critical logic)
func TestHTTPGetDetector_checkIdleTimeout_NotIdle(t *testing.T) {
	detector := NewHTTPGetDetector()
	ctx := context.Background()

	// Create response with recent activity (5 minutes ago)
	recentTime := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
	idleResp := &EndpointIdleResponse{
		LastActivity: recentTime,
	}
	idleConfig := createTestIdleConfig(30) // 30 minute timeout

	// Execute
	isIdle := detector.checkIdleTimeout(ctx, testWorkspaceName, idleResp, idleConfig)

	// Assert
	assert.False(t, isIdle) // Should not be idle (recent activity)
}

func TestHTTPGetDetector_checkIdleTimeout_IsIdle(t *testing.T) {
	detector := NewHTTPGetDetector()
	ctx := context.Background()

	// Create response with old activity (45 minutes ago)
	oldTime := time.Now().Add(-45 * time.Minute).Format(time.RFC3339)
	idleResp := &EndpointIdleResponse{
		LastActivity: oldTime,
	}
	idleConfig := createTestIdleConfig(30) // 30 minute timeout

	// Execute
	isIdle := detector.checkIdleTimeout(ctx, testWorkspaceName, idleResp, idleConfig)

	// Assert
	assert.True(t, isIdle) // Should be idle (old activity)
}

func TestHTTPGetDetector_checkIdleTimeout_LowercaseZ_Normalization(t *testing.T) {
	detector := NewHTTPGetDetector()
	ctx := context.Background()

	// Create response with lowercase 'z' in timestamp (UTC format)
	baseTime := time.Now().Add(-5 * time.Minute).UTC()
	timeWithLowercaseZ := baseTime.Format("2006-01-02T15:04:05") + "z" // Use lowercase z for UTC
	idleResp := &EndpointIdleResponse{
		LastActivity: timeWithLowercaseZ,
	}
	idleConfig := createTestIdleConfig(45) // Different timeout for variety

	// Execute
	isIdle := detector.checkIdleTimeout(ctx, testWorkspaceName, idleResp, idleConfig)

	// Assert - should handle lowercase 'z' correctly
	assert.False(t, isIdle) // Recent activity, not idle
}

func TestHTTPGetDetector_checkIdleTimeout_InvalidTimestamp(t *testing.T) {
	detector := NewHTTPGetDetector()
	ctx := context.Background()

	// Create response with invalid timestamp
	idleResp := &EndpointIdleResponse{
		LastActivity: "invalid-timestamp",
	}
	idleConfig := createTestIdleConfig(30)

	// Execute
	isIdle := detector.checkIdleTimeout(ctx, testWorkspaceName, idleResp, idleConfig)

	// Assert - should return false on parse error
	assert.False(t, isIdle)
}

// Test HTTPGetDetector.CheckIdle method
func TestHTTPGetDetector_CheckIdle_Success_NotIdle(t *testing.T) {
	// Create mock
	mockExecUtil := &MockPodExecUtil{}
	detector := createDetectorWithMock(mockExecUtil)

	// Setup test data
	ctx := context.Background()
	pod := createTestPod()
	idleConfig := createTestIdleConfig(30) // 30 minute timeout

	// Mock the response (recent activity = not idle)
	recentTime := time.Now().Add(-5 * time.Minute).Format(time.RFC3339)
	curlOutput := fmt.Sprintf(`{"lastActiveTimestamp": "%s"}
HTTP Status: 200`, recentTime)

	mockExecUtil.On("ExecInPod",
		ctx, pod, "",
		mock.AnythingOfType("[]string"), // Don't match exact command, just verify it's called
		"").Return(curlOutput, nil)

	// Execute
	result, err := detector.CheckIdle(ctx, testWorkspaceName, pod, idleConfig)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.False(t, result.IsIdle)     // Not idle (recent activity)
	assert.True(t, result.ShouldRetry) // Continue checking
	mockExecUtil.AssertExpectations(t)
}

func TestHTTPGetDetector_CheckIdle_Success_IsIdle(t *testing.T) {
	// Create mock
	mockExecUtil := &MockPodExecUtil{}
	detector := createDetectorWithMock(mockExecUtil)

	// Setup test data
	ctx := context.Background()
	pod := createTestPod()
	idleConfig := createTestIdleConfig(30) // 30 minute timeout

	// Mock the response (old activity = is idle)
	oldTime := time.Now().Add(-45 * time.Minute).Format(time.RFC3339)
	curlOutput := fmt.Sprintf(`{"lastActiveTimestamp": "%s"}
HTTP Status: 200`, oldTime)

	mockExecUtil.On("ExecInPod",
		ctx, pod, "",
		mock.AnythingOfType("[]string"),
		"").Return(curlOutput, nil)

	// Execute
	result, err := detector.CheckIdle(ctx, testWorkspaceName, pod, idleConfig)

	// Assert
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.True(t, result.IsIdle)      // Is idle (old activity)
	assert.True(t, result.ShouldRetry) // Continue checking
	mockExecUtil.AssertExpectations(t)
}

func TestHTTPGetDetector_CheckIdle_HTTP404_PermanentFailure(t *testing.T) {
	// Create mock
	mockExecUtil := &MockPodExecUtil{}
	detector := createDetectorWithMock(mockExecUtil)

	// Setup test data
	ctx := context.Background()
	pod := createTestPod()
	idleConfig := createTestIdleConfig(30)

	// Mock 404 response
	curlOutput := `HTTP Status: 404`

	mockExecUtil.On("ExecInPod",
		ctx, pod, "",
		mock.AnythingOfType("[]string"),
		"").Return(curlOutput, nil)

	// Execute
	result, err := detector.CheckIdle(ctx, testWorkspaceName, pod, idleConfig)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint not found")
	assert.NotNil(t, result)
	assert.False(t, result.IsIdle)
	assert.False(t, result.ShouldRetry) // Should NOT retry on 404 (permanent failure)
	mockExecUtil.AssertExpectations(t)
}

func TestHTTPGetDetector_CheckIdle_ConnectionRefused_TemporaryFailure(t *testing.T) {
	// Create mock
	mockExecUtil := &MockPodExecUtil{}
	detector := createDetectorWithMock(mockExecUtil)

	// Setup test data
	ctx := context.Background()
	pod := createTestPod()
	idleConfig := createTestIdleConfig(30)

	// Mock connection refused error (curl exit code 7)
	mockExecUtil.On("ExecInPod",
		ctx, pod, "",
		mock.AnythingOfType("[]string"),
		"").Return("", errors.New("command terminated with exit code 7"))

	// Execute
	result, err := detector.CheckIdle(ctx, testWorkspaceName, pod, idleConfig)

	// Assert
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
	assert.NotNil(t, result)
	assert.False(t, result.IsIdle)
	assert.True(t, result.ShouldRetry) // Should retry on connection refused (temporary failure)
	mockExecUtil.AssertExpectations(t)
}

func TestHTTPGetDetector_CheckIdle_HTTP500_TemporaryFailure(t *testing.T) {
	// Create mock
	mockExecUtil := &MockPodExecUtil{}
	detector := createDetectorWithMock(mockExecUtil)

	// Setup test data
	ctx := context.Background()
	pod := createTestPod()
	idleConfig := createTestIdleConfig(30)

	// Test different HTTP 5xx status codes that should be treated as temporary failures
	testCases := []struct {
		statusCode string
		name       string
	}{
		{"500", "Internal Server Error"},
		{"503", "Service Unavailable"},
		{"502", "Bad Gateway"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Reset mock for each test case
			mockExecUtil.ExpectedCalls = nil

			curlOutput := fmt.Sprintf(`HTTP Status: %s`, tc.statusCode)

			mockExecUtil.On("ExecInPod",
				ctx, pod, "",
				mock.AnythingOfType("[]string"),
				"").Return(curlOutput, nil)

			// Execute
			result, err := detector.CheckIdle(ctx, testWorkspaceName, pod, idleConfig)

			// Assert
			assert.Error(t, err)
			assert.Contains(t, err.Error(), fmt.Sprintf("unexpected HTTP status: %s", tc.statusCode))
			assert.NotNil(t, result)
			assert.False(t, result.IsIdle)
			assert.True(t, result.ShouldRetry) // Should retry on HTTP 5xx errors (temporary failure)
			mockExecUtil.AssertExpectations(t)
		})
	}
}
