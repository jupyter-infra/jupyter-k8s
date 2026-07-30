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
	"sigs.k8s.io/controller-runtime/pkg/client"
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
			Name:      testWorkspaceName,
			Namespace: testNamespaceName,
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
		podEventAdapters: map[string]pluginadapters.PodEventPluginAdapter{pluginNameAWS: mockHandler},
	}

	// Create running workspace pod
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podNameWorkspaceSuffix,
			Namespace: testNamespaceName,
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: testWorkspaceName,
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
			Name:      podNameWorkspaceSuffix,
			Namespace: testNamespaceName,
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
			Name:      testWorkspaceName,
			Namespace: testNamespaceName,
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
			Name:      podNameWorkspaceSuffix,
			Namespace: testNamespaceName,
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: testWorkspaceName,
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
		podEventAdapters: map[string]pluginadapters.PodEventPluginAdapter{pluginNameAWS: &mockPodEventHandler{}},
	}

	deletionTime := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podNameWorkspaceSuffix,
			Namespace: testNamespaceName,
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: testWorkspaceName,
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
			Name:      podNameWorkspaceSuffix,
			Namespace: testNamespaceName,
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: testWorkspaceName,
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
					Name:      testWorkspaceName,
					Namespace: testNamespaceName,
				},
			}

			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testStrategyName,
					Namespace: testNamespaceName,
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
				podEventAdapters: map[string]pluginadapters.PodEventPluginAdapter{pluginNameAWS: mockHandler},
			}

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-pod",
					Namespace: testNamespaceName,
					Labels: map[string]string{
						workspaceutil.LabelWorkspaceName: testWorkspaceName,
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
			Namespace: testNamespace,
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
		podEventAdapters: map[string]pluginadapters.PodEventPluginAdapter{pluginNameAWS: mockHandler},
	}

	deletionTime := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podNameWorkspaceSuffix,
			Namespace: testNamespaceName,
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: testWorkspaceName,
				LabelAccessStrategyName:          "aws-access-strategy",
				LabelAccessStrategyNamespace:     testNamespace,
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
			Namespace: testNamespace,
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
		podEventAdapters: map[string]pluginadapters.PodEventPluginAdapter{pluginNameAWS: mockHandler},
	}

	deletionTime := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podNameWorkspaceSuffix,
			Namespace: testNamespaceName,
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: testWorkspaceName,
				LabelAccessStrategyName:          "some-other-access-strategy",
				LabelAccessStrategyNamespace:     testNamespace,
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
		podEventAdapters: map[string]pluginadapters.PodEventPluginAdapter{pluginNameAWS: mockHandler},
	}

	deletionTime := metav1.Now()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podNameWorkspaceSuffix,
			Namespace: testNamespaceName,
			Labels: map[string]string{
				workspaceutil.LabelWorkspaceName: testWorkspaceName,
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
		Reason:  ConditionTypeStopped,
		Message: "Pod was Preempted by scheduler",
	}

	if event.InvolvedObject.Kind != "Pod" ||
		event.Reason != ConditionTypeStopped ||
		!strings.Contains(event.Message, "Preempted") {
		t.Error("Should detect preemption event")
	}

	podName := event.InvolvedObject.Name
	if strings.HasPrefix(podName, "jupyter-") {
		parts := strings.Split(podName, "-")
		if len(parts) >= 4 {
			workspaceName := strings.Join(parts[1:len(parts)-2], "-")
			if workspaceName != testWorkspaceName {
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

// newPreemptionEvent builds a k8s Event that looks like a scheduler preemption of
// the given pod.
func newPreemptionEvent(podName, namespace string) *corev1.Event {
	return &corev1.Event{
		InvolvedObject: corev1.ObjectReference{
			Kind:      KindPod,
			Name:      podName,
			Namespace: namespace,
		},
		Reason:  DesiredStateStopped,
		Message: "Pod was Preempted by scheduler",
	}
}

func TestHandleKubernetesEvents_IgnoresNonEventObject(t *testing.T) {
	handler := &PodEventHandler{client: fake.NewClientBuilder().Build()}

	// Passing a Pod (not an Event) should be ignored.
	result := handler.HandleKubernetesEvents(context.Background(), &corev1.Pod{})

	if result != nil {
		t.Errorf("expected nil for non-Event object, got %v", result)
	}
}

func TestHandleKubernetesEvents_NonPreemptionEventIgnored(t *testing.T) {
	handler := &PodEventHandler{client: fake.NewClientBuilder().Build()}

	event := &corev1.Event{
		InvolvedObject: corev1.ObjectReference{Kind: KindPod, Name: "jupyter-ws-abc123-xyz789", Namespace: testNamespaceName},
		Reason:         "Scheduled",
		Message:        "Successfully assigned pod to node",
	}

	result := handler.HandleKubernetesEvents(context.Background(), event)

	if result != nil {
		t.Errorf("expected nil for non-preemption event, got %v", result)
	}
}

func TestHandleKubernetesEvents_NonJupyterPodIgnored(t *testing.T) {
	handler := &PodEventHandler{client: fake.NewClientBuilder().Build()}

	result := handler.HandleKubernetesEvents(context.Background(), newPreemptionEvent("other-pod-abc123-xyz789", testNamespaceName))

	if result != nil {
		t.Errorf("expected nil for non-jupyter pod, got %v", result)
	}
}

func TestHandleKubernetesEvents_TooFewNameParts(t *testing.T) {
	handler := &PodEventHandler{client: fake.NewClientBuilder().Build()}

	// "jupyter-ws-xyz789" splits into 3 parts (< 4), so it is skipped.
	result := handler.HandleKubernetesEvents(context.Background(), newPreemptionEvent("jupyter-ws-xyz789", testNamespaceName))

	if result != nil {
		t.Errorf("expected nil when pod name has too few parts, got %v", result)
	}
}

func TestHandleKubernetesEvents_PreemptionUpdatesWorkspace(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	workspace := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testWorkspaceName,
			Namespace: testNamespaceName,
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DesiredStatus: DesiredStateRunning,
		},
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(workspace).Build()
	handler := &PodEventHandler{client: fakeClient}

	// Pod name "jupyter-test-workspace-abc123-xyz789" → workspace "test-workspace".
	event := newPreemptionEvent("jupyter-test-workspace-abc123-xyz789", testNamespaceName)

	result := handler.HandleKubernetesEvents(context.Background(), event)

	// Should return a reconcile request for the resolved workspace.
	if len(result) != 1 {
		t.Fatalf("expected 1 reconcile request, got %d", len(result))
	}
	if result[0].Name != testWorkspaceName || result[0].Namespace != testNamespaceName {
		t.Errorf("unexpected reconcile request: %+v", result[0].NamespacedName)
	}

	// The workspace should be flipped to Stopped and annotated with the preemption reason.
	updated := &workspacev1alpha1.Workspace{}
	if err := fakeClient.Get(context.Background(), client.ObjectKey{Name: testWorkspaceName, Namespace: testNamespaceName}, updated); err != nil {
		t.Fatalf("failed to get workspace: %v", err)
	}
	if updated.Spec.DesiredStatus != DesiredStateStopped {
		t.Errorf("expected DesiredStatus %q, got %q", DesiredStateStopped, updated.Spec.DesiredStatus)
	}
	if updated.Annotations[PreemptionReasonAnnotation] != PreemptedReason {
		t.Errorf("expected preemption annotation %q, got %q", PreemptedReason, updated.Annotations[PreemptionReasonAnnotation])
	}
}

func TestUpdateWorkspaceDesiredStatus_WorkspaceNotFound(t *testing.T) {
	// A missing workspace is logged and returns without error (no panic).
	handler := &PodEventHandler{client: fake.NewClientBuilder().Build()}

	handler.updateWorkspaceDesiredStatus(context.Background(), "missing-workspace", testNamespaceName, DesiredStateStopped)
}
