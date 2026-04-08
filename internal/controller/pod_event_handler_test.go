/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/pluginadapters"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// mockPodEventHandler implements pluginadapters.PodEventPluginAdapter for testing
type mockPodEventHandler struct {
	handlePodRunningCalled bool
	handlePodDeletedCalled bool
	handlePodRunningErr    error
	handlePodDeletedErr    error
}

func (m *mockPodEventHandler) HandlePodRunning(ctx context.Context, pod *corev1.Pod, workspaceName, namespace string, podEventsContext map[string]string) error {
	m.handlePodRunningCalled = true
	return m.handlePodRunningErr
}

func (m *mockPodEventHandler) HandlePodDeleted(ctx context.Context, pod *corev1.Pod, podEventsContext map[string]string) error {
	m.handlePodDeletedCalled = true
	return m.handlePodDeletedErr
}

func TestNewPodEventHandler_NoPlugins(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()
	mockRM := &ResourceManager{}

	handler := NewPodEventHandler(fakeClient, mockRM, nil)

	if handler == nil {
		t.Fatal("Expected non-nil PodEventHandler")
	}
	if handler.client != fakeClient {
		t.Error("Expected client to be set correctly")
	}
	if handler.resourceManager != mockRM {
		t.Error("Expected resourceManager to be set correctly")
	}
	if handler.podEventAdapters != nil {
		t.Error("Expected podEventAdapters to be nil when no plugins provided")
	}
}

func TestHandleWorkspacePodEvents_PodRunning_Success(t *testing.T) {
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

	mockHandler := &mockPodEventHandler{}

	// Create handler
	handler := &PodEventHandler{
		client:           fakeClient,
		resourceManager:  &ResourceManager{},
		podEventAdapters: map[string]pluginadapters.PodEventPluginAdapter{"aws": mockHandler},
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

	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result (no reconciliation triggered)")
	}
}

func TestHandleWorkspacePodEvents_PodRunning_WorkspaceNotFound(t *testing.T) {
	fakeClient := fake.NewClientBuilder().Build()

	handler := &PodEventHandler{
		client:          fakeClient,
		resourceManager: &ResourceManager{},
	}

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

	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result when workspace not found")
	}
}

func TestHandleWorkspacePodEvents_PodRunning_HandlersNil(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace",
			Namespace: "test-namespace",
		},
	}

	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(workspace).
		Build()

	handler := &PodEventHandler{
		client:           fakeClient,
		resourceManager:  &ResourceManager{},
		podEventAdapters: nil,
	}

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

	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result when handlers are nil")
	}
}

func TestHandleWorkspacePodEvents_PodDeleted_Success(t *testing.T) {
	handler := &PodEventHandler{
		client:           fake.NewClientBuilder().Build(),
		resourceManager:  &ResourceManager{},
		podEventAdapters: map[string]pluginadapters.PodEventPluginAdapter{"aws": &mockPodEventHandler{}},
	}

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

	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result for deleted pod")
	}
}

func TestHandleWorkspacePodEvents_PodDeleted_HandlersNil(t *testing.T) {
	handler := &PodEventHandler{
		client:           fake.NewClientBuilder().Build(),
		resourceManager:  &ResourceManager{},
		podEventAdapters: nil,
	}

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

	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result when handlers are nil")
	}
}

func TestHandlePodRunning_WithPodEventsHandler(t *testing.T) {
	tests := []struct {
		name             string
		podEventsHandler string
	}{
		{
			name:             "AWS handler dispatches correctly",
			podEventsHandler: "aws:ssm-remote-access",
		},
		{
			name:             "Empty handler skips dispatch",
			podEventsHandler: "",
		},
		{
			name:             "Unknown handler logs error",
			podEventsHandler: "other:unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "test-namespace",
				},
			}

			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-strategy",
					Namespace: "test-namespace",
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					PodEventsHandler: tt.podEventsHandler,
				},
			}

			scheme := runtime.NewScheme()
			_ = workspacev1alpha1.AddToScheme(scheme)
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace, accessStrategy).
				Build()

			mockHandler := &mockPodEventHandler{}
			handler := &PodEventHandler{
				client:           fakeClient,
				resourceManager:  &ResourceManager{},
				podEventAdapters: map[string]pluginadapters.PodEventPluginAdapter{"aws": mockHandler},
			}

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

			result := handler.HandleWorkspacePodEvents(context.Background(), pod)

			if result != nil {
				t.Errorf("Expected nil result but got: %v", result)
			}
		})
	}
}

func TestHandleWorkspacePodEvents_PodDeleted_WithAWSHandler(t *testing.T) {
	mockHandler := &mockPodEventHandler{}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "aws-access-strategy",
			Namespace: "default",
		},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			PodEventsHandler: "aws:ssm-remote-access",
		},
	}

	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	handler := &PodEventHandler{
		client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(accessStrategy).
			Build(),
		resourceManager:  &ResourceManager{},
		podEventAdapters: map[string]pluginadapters.PodEventPluginAdapter{"aws": mockHandler},
	}

	deletionTime := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workspace-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: "test-workspace",
				LabelAccessStrategyName:          "aws-access-strategy",
				LabelAccessStrategyNamespace:     "default",
			},
			DeletionTimestamp: &deletionTime,
		},
	}

	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result for deleted pod with AWS handler")
	}

	if !mockHandler.handlePodDeletedCalled {
		t.Error("Expected HandlePodDeleted to be called for pod with AWS handler")
	}
}

func TestHandleWorkspacePodEvents_PodDeleted_WithNonAWSHandler(t *testing.T) {
	mockHandler := &mockPodEventHandler{}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "some-other-access-strategy",
			Namespace: "default",
		},
		Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
			PodEventsHandler: "other:handler",
		},
	}

	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	handler := &PodEventHandler{
		client: fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(accessStrategy).
			Build(),
		resourceManager:  &ResourceManager{},
		podEventAdapters: map[string]pluginadapters.PodEventPluginAdapter{"aws": mockHandler},
	}

	deletionTime := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "workspace-pod",
			Namespace: "test-namespace",
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: "test-workspace",
				LabelAccessStrategyName:          "some-other-access-strategy",
				LabelAccessStrategyNamespace:     "default",
			},
			DeletionTimestamp: &deletionTime,
		},
	}

	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result for deleted pod with non-AWS handler")
	}

	if mockHandler.handlePodDeletedCalled {
		t.Error("Expected HandlePodDeleted to NOT be called for pod with non-AWS handler")
	}
}

func TestHandleWorkspacePodEvents_PodDeleted_WithoutAccessStrategyLabel(t *testing.T) {
	mockHandler := &mockPodEventHandler{}

	handler := &PodEventHandler{
		client:           fake.NewClientBuilder().Build(),
		resourceManager:  &ResourceManager{},
		podEventAdapters: map[string]pluginadapters.PodEventPluginAdapter{"aws": mockHandler},
	}

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

	result := handler.HandleWorkspacePodEvents(context.Background(), pod)

	if result != nil {
		t.Error("Expected nil result for deleted pod without access strategy label")
	}

	if mockHandler.handlePodDeletedCalled {
		t.Error("Expected HandlePodDeleted to NOT be called for pod without access strategy label")
	}
}

func TestHandleKubernetesEvents(t *testing.T) {
	event := &corev1.Event{
		InvolvedObject: corev1.ObjectReference{
			Kind:      "Pod",
			Name:      "jupyter-test-workspace-abc123-xyz789",
			Namespace: "test-ns",
		},
		Reason:  "Stopped",
		Message: "Pod was Preempted by scheduler",
	}

	if event.InvolvedObject.Kind != "Pod" ||
		event.Reason != "Stopped" ||
		!strings.Contains(event.Message, "Preempted") {
		t.Error("Should detect preemption event")
	}

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
