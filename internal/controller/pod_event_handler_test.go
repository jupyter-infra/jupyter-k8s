package controller

import (
	"context"
	"errors"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	awsutil "github.com/jupyter-ai-contrib/jupyter-k8s/internal/aws"
	workspaceutil "github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
)

func TestNewPodEventHandler_Success(t *testing.T) {
	// Save original
	original := newSSMRemoteAccessStrategy
	originalNewPodExecUtil := newPodExecUtil
	defer func() {
		newSSMRemoteAccessStrategy = original
		newPodExecUtil = originalNewPodExecUtil
	}()

	// Mock successful PodExecUtil creation
	mockPodExecUtil := &PodExecUtil{}
	newPodExecUtil = func() (*PodExecUtil, error) {
		return mockPodExecUtil, nil
	}

	// Mock successful SSM strategy creation
	mockStrategy := &awsutil.SSMRemoteAccessStrategy{}
	newSSMRemoteAccessStrategy = func(podExecUtil awsutil.PodExecInterface) (*awsutil.SSMRemoteAccessStrategy, error) {
		return mockStrategy, nil
	}

	// Create test dependencies
	fakeClient := fake.NewClientBuilder().Build()
	mockRM := &ResourceManager{} // Use real type with zero values for constructor test

	// Test successful creation
	handler := NewPodEventHandler(fakeClient, mockRM)

	if handler == nil {
		t.Fatal("Expected non-nil PodEventHandler")
		return
	}
	if handler.client != fakeClient {
		t.Error("Expected client to be set correctly")
	}
	if handler.resourceManager != mockRM {
		t.Error("Expected resourceManager to be set correctly")
	}
	if handler.ssmRemoteAccessStrategy != mockStrategy {
		t.Error("Expected ssmRemoteAccessStrategy to be set to mock strategy")
	}
}

func TestNewPodEventHandler_SSMStrategyFailure(t *testing.T) {
	// Save original
	original := newSSMRemoteAccessStrategy
	originalNewPodExecUtil := newPodExecUtil
	defer func() {
		newSSMRemoteAccessStrategy = original
		newPodExecUtil = originalNewPodExecUtil
	}()

	// Mock successful PodExecUtil creation
	mockPodExecUtil := &PodExecUtil{}
	newPodExecUtil = func() (*PodExecUtil, error) {
		return mockPodExecUtil, nil
	}

	// Mock SSM strategy creation failure
	expectedError := errors.New("mock SSM strategy creation error")
	newSSMRemoteAccessStrategy = func(podExecUtil awsutil.PodExecInterface) (*awsutil.SSMRemoteAccessStrategy, error) {
		return nil, expectedError
	}

	// Create test dependencies
	fakeClient := fake.NewClientBuilder().Build()
	mockRM := &ResourceManager{} // Use real type with zero values for constructor test

	// Test creation with SSM strategy failure
	handler := NewPodEventHandler(fakeClient, mockRM)

	// Verify handler is still created (main test)
	if handler == nil {
		t.Fatal("Expected non-nil PodEventHandler even when SSM strategy fails")
		return
	}
	if handler.client != fakeClient {
		t.Error("Expected client to be set correctly")
	}
	if handler.resourceManager != mockRM {
		t.Error("Expected resourceManager to be set correctly")
	}
	if handler.ssmRemoteAccessStrategy != nil {
		t.Error("Expected ssmRemoteAccessStrategy to be nil when creation fails")
	}
}

func TestHandleWorkspacePodEvents_PodRunning_SSMSuccess(t *testing.T) {
	// Create workspace object
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace",
			Namespace: "test-namespace",
		},
	}

	// Create scheme and add our types
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create fake client with workspace
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workspace).
		Build()

	// Create handler with minimal setup (we'll test the basic flow)
	handler := &PodEventHandler{
		client:                  fakeClient,
		resourceManager:         &ResourceManager{},                 // Will fail gracefully
		ssmRemoteAccessStrategy: &awsutil.SSMRemoteAccessStrategy{}, // Will fail gracefully
	}

	// Create running workspace pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workspace-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: "test-workspace",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	// Test handling running workspace pod (will exercise the flow)
	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result (no reconciliation triggered)")
	}
	// Success is indicated by no panics and proper execution flow
}

func TestHandleWorkspacePodEvents_PodRunning_WorkspaceNotFound(t *testing.T) {
	// Create fake client without workspace (simulates deleted workspace)
	fakeClient := fake.NewClientBuilder().Build()

	// Create handler
	handler := &PodEventHandler{
		client:          fakeClient,
		resourceManager: &ResourceManager{},
	}

	// Create running workspace pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workspace-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: "missing-workspace",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	// Test handling running pod when workspace is not found
	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result when workspace not found")
	}
	// Success is indicated by graceful handling (no panic)
}

func TestHandleWorkspacePodEvents_PodRunning_SSMStrategyNil(t *testing.T) {
	// Create workspace object
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace",
			Namespace: "test-namespace",
		},
	}

	// Create scheme and add our types
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create fake client with workspace
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workspace).
		Build()

	// Create handler with nil SSM strategy
	handler := &PodEventHandler{
		client:                  fakeClient,
		resourceManager:         &ResourceManager{}, // Will fail gracefully
		ssmRemoteAccessStrategy: nil,                // SSM strategy is nil
	}

	// Create running workspace pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workspace-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: "test-workspace",
			},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
		},
	}

	// Test handling running pod when SSM strategy is nil
	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result when SSM strategy is nil")
	}
	// Success is indicated by graceful handling (logs error but doesn't crash)
}

func TestHandleWorkspacePodEvents_PodDeleted_Success(t *testing.T) {
	// Create handler
	handler := &PodEventHandler{
		client:                  fake.NewClientBuilder().Build(),
		resourceManager:         &ResourceManager{},
		ssmRemoteAccessStrategy: &awsutil.SSMRemoteAccessStrategy{}, // Will fail gracefully
	}

	// Create deleted workspace pod
	deletionTime := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workspace-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: "test-workspace",
			},
			DeletionTimestamp: &deletionTime,
		},
	}

	// Test handling deleted workspace pod
	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result for deleted pod")
	}
	// Success is indicated by no panics and proper execution flow
}

func TestHandleWorkspacePodEvents_PodDeleted_SSMStrategyNil(t *testing.T) {
	// Create handler with nil SSM strategy
	handler := &PodEventHandler{
		client:                  fake.NewClientBuilder().Build(),
		resourceManager:         &ResourceManager{},
		ssmRemoteAccessStrategy: nil, // SSM strategy is nil
	}

	// Create deleted workspace pod
	deletionTime := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workspace-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: "test-workspace",
			},
			DeletionTimestamp: &deletionTime,
		},
	}

	// Test handling deleted pod when SSM strategy is nil
	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result when SSM strategy is nil")
	}
	// Success is indicated by graceful handling (logs error but doesn't crash)
}

func TestHandleWorkspacePodEvents_PodDeleted_SSMCleanupFailure(t *testing.T) {
	// Create handler
	handler := &PodEventHandler{
		client:                  fake.NewClientBuilder().Build(),
		resourceManager:         &ResourceManager{},
		ssmRemoteAccessStrategy: &awsutil.SSMRemoteAccessStrategy{}, // Will fail gracefully
	}

	// Create deleted workspace pod
	deletionTime := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workspace-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: "test-workspace",
			},
			DeletionTimestamp: &deletionTime,
		},
	}

	// Test handling deleted pod when SSM cleanup fails
	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result even when SSM cleanup fails")
	}
	// Success is indicated by graceful handling (logs error but doesn't crash)
}

func TestHandlePodRunning_WithPodEventsHandler(t *testing.T) {
	tests := []struct {
		name             string
		podEventsHandler string
		expectError      bool
	}{
		{
			name:             "AWS handler processes normally",
			podEventsHandler: "aws",
			expectError:      false,
		},
		{
			name:             "Empty handler processes normally",
			podEventsHandler: "",
			expectError:      false,
		},
		{
			name:             "Non-AWS handler processes normally",
			podEventsHandler: "other",
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create workspace
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "test-namespace",
				},
			}

			// Create access strategy with specified handler
			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-strategy",
					Namespace: "test-namespace",
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					PodEventsHandler: tt.podEventsHandler,
				},
			}

			// Create fake client with workspace and access strategy
			scheme := runtime.NewScheme()
			_ = workspacev1alpha1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace, accessStrategy).
				Build()

			handler := &PodEventHandler{
				client:                  fakeClient,
				resourceManager:         &ResourceManager{},
				ssmRemoteAccessStrategy: &awsutil.SSMRemoteAccessStrategy{}, // Will fail gracefully for non-AWS handlers
			}

			// Create pod
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-pod",
					Namespace: "test-namespace",
					Labels: map[string]string{
						workspaceutil.LabelWorkspaceName: "test-workspace",
					},
				},
				Status: corev1.PodStatus{
					Phase: corev1.PodRunning,
				},
			}

			// Test handling the pod event (this will exercise our podEventsHandler logic)
			result := handler.HandleWorkspacePodEvents(context.Background(), pod)

			if tt.expectError {
				if result == nil {
					t.Error("Expected error result but got nil")
				}
			} else {
				if result != nil {
					t.Errorf("Expected nil result but got: %v", result)
				}
			}
		})
	}
}

func TestHandleKubernetesEvents(t *testing.T) {
	// Test preemption event
	event := &corev1.Event{
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "jupyter-test-workspace-abc123-xyz789",
			Namespace: "test-ns",
		},
		Reason:  "Stopped",
		Message: "Pod was Preempted by scheduler",
	}

	// Test preemption event detection
	if event.InvolvedObject.Kind != "Pod" ||
		event.Reason != "Stopped" ||
		!strings.Contains(event.Message, "Preempted") {
		t.Error("Should detect preemption event")
	}

	// Test workspace name extraction
	podName := event.InvolvedObject.Name
	if strings.HasPrefix(podName, "jupyter-") {
		parts := strings.Split(podName, "-")
		if len(parts) >= 4 {
			workspaceName := strings.Join(parts[1:len(parts)-2], "-")
			if workspaceName != "test-workspace" {
				t.Errorf("Expected 'test-workspace', got '%s'", workspaceName)
			}
		}
	}
}

// TestWorkspaceNameExtraction tests workspace name extraction edge cases
func TestWorkspaceNameExtraction(t *testing.T) {
	tests := []struct {
		name         string
		podName      string
		expectedName string
		shouldMatch  bool
	}{
		{
			name:         "Standard workspace with hyphens",
			podName:      "jupyter-my-long-workspace-name-7d4b8c9f6d-x8k2m",
			expectedName: "my-long-workspace-name",
			shouldMatch:  true,
		},
		{
			name:        "Edge case: Too few parts (truncated)",
			podName:     "jupyter-workspace-x8k2m",
			shouldMatch: false,
		},
		{
			name:         "Edge case: Very long name near 63 char limit",
			podName:      "jupyter-very-long-workspace-name-that-might-be-truncated-7d4b8c-x8k2m",
			expectedName: "very-long-workspace-name-that-might-be-truncated",
			shouldMatch:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !strings.HasPrefix(tt.podName, "jupyter-") {
				if tt.shouldMatch {
					t.Errorf("Expected to match but pod name doesn't have jupyter- prefix")
				}
				return
			}

			parts := strings.Split(tt.podName, "-")
			if len(parts) < 4 {
				if tt.shouldMatch {
					t.Errorf("Expected to match but pod name has too few parts: %d", len(parts))
				}
				return
			}

			workspaceName := strings.Join(parts[1:len(parts)-2], "-")
			if tt.shouldMatch && workspaceName != tt.expectedName {
				t.Errorf("Expected workspace name '%s', got '%s'", tt.expectedName, workspaceName)
			}
		})
	}
}
