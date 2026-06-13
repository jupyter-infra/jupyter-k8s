/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package workspace

import (
	"context"
	"fmt"
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetIntegrationStrategyRefNamespace(t *testing.T) {
	t.Run("returns workspace namespace when no integration strategy", func(t *testing.T) {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "ws-ns"},
		}
		assert.Equal(t, "ws-ns", GetIntegrationStrategyRefNamespace(ws))
	})

	t.Run("returns workspace namespace when ref namespace empty", func(t *testing.T) {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "ws-ns"},
			Spec: workspacev1alpha1.WorkspaceSpec{
				IntegrationStrategy: &workspacev1alpha1.IntegrationStrategyRef{Name: "strat"},
			},
		}
		assert.Equal(t, "ws-ns", GetIntegrationStrategyRefNamespace(ws))
	})

	t.Run("returns ref namespace when set", func(t *testing.T) {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "ws-ns"},
			Spec: workspacev1alpha1.WorkspaceSpec{
				IntegrationStrategy: &workspacev1alpha1.IntegrationStrategyRef{
					Name:      "strat",
					Namespace: "strat-ns",
				},
			},
		}
		assert.Equal(t, "strat-ns", GetIntegrationStrategyRefNamespace(ws))
	})
}

func TestListActiveWorkspacesByIntegrationStrategy_CallsListWithMatchLabels_ReturnWorkspaces(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	newWS := func(name, namespace string) *workspacev1alpha1.Workspace {
		return &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					LabelIntegrationStrategyName:      "test-integration-strategy",
					LabelIntegrationStrategyNamespace: "integration-strategy-namespace",
				},
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				IntegrationStrategy: &workspacev1alpha1.IntegrationStrategyRef{
					Name:      "test-integration-strategy",
					Namespace: "integration-strategy-namespace",
				},
			},
		}
	}

	ws1 := newWS("test-workspace-1", "default")
	ws2 := newWS("test-workspace-2", "default")

	// A deleted workspace that should be skipped.
	deletionTime := metav1.Now()
	wsDeleted := newWS("test-workspace-deleted", "default")
	wsDeleted.DeletionTimestamp = &deletionTime
	wsDeleted.Finalizers = []string{"test-finalizer"}

	// A workspace carrying the label but with a drifted spec reference - should be skipped.
	wsDrift := newWS("test-workspace-drift", "default")
	wsDrift.Spec.IntegrationStrategy.Name = "some-other-strategy"

	// MockClient captures the list options to verify label filtering.
	mockClient := &MockClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws1, ws2, wsDeleted, wsDrift).Build(),
	}

	_, err := ListActiveWorkspacesByIntegrationStrategy(
		context.Background(),
		mockClient,
		"test-integration-strategy",
		"integration-strategy-namespace")
	assert.NoError(t, err)

	assert.NotEmpty(t, mockClient.ListOptions, "List options should not be empty")
	labels := make(map[string]string)
	for _, opt := range mockClient.ListOptions {
		optString := fmt.Sprintf("%v", opt)
		if matches := getLabelsFromOption(optString); len(matches) > 0 {
			for k, v := range matches {
				labels[k] = v
			}
		}
	}
	assert.Equal(t, "test-integration-strategy", labels[LabelIntegrationStrategyName],
		"Should filter by integration strategy name")
	assert.Equal(t, "integration-strategy-namespace", labels[LabelIntegrationStrategyNamespace],
		"Should filter by integration strategy namespace")

	// Real client: verify filtering of deleted and drifted workspaces.
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws1, ws2, wsDeleted, wsDrift).Build()
	workspaces, err := ListActiveWorkspacesByIntegrationStrategy(
		context.Background(),
		fakeClient,
		"test-integration-strategy",
		"integration-strategy-namespace")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(workspaces), "Expected 2 active, matching workspaces")

	names := make(map[string]bool)
	for _, ws := range workspaces {
		names[ws.Name] = true
	}
	assert.True(t, names["test-workspace-1"])
	assert.True(t, names["test-workspace-2"])
	assert.False(t, names["test-workspace-deleted"], "Deleted workspace should be skipped")
	assert.False(t, names["test-workspace-drift"], "Drifted workspace should be skipped")
}

func TestListActiveWorkspacesByIntegrationStrategy_OnListError_ReturnErrors(t *testing.T) {
	mockClient := &MockClient{
		ListError: fmt.Errorf("mock list error"),
	}

	workspaces, err := ListActiveWorkspacesByIntegrationStrategy(
		context.Background(),
		mockClient,
		"test-integration-strategy",
		"integration-strategy-namespace")

	assert.Error(t, err)
	assert.Nil(t, workspaces)
	assert.Contains(t, err.Error(), "failed to list workspaces by IntegrationStrategy label: mock list error")
}

func TestGetWorkspaceReconciliationRequestsForIntegrationStrategy_ReturnRequests(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	newWS := func(name, namespace string) *workspacev1alpha1.Workspace {
		return &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					LabelIntegrationStrategyName:      "test-integration-strategy",
					LabelIntegrationStrategyNamespace: "integration-strategy-namespace",
				},
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				IntegrationStrategy: &workspacev1alpha1.IntegrationStrategyRef{
					Name:      "test-integration-strategy",
					Namespace: "integration-strategy-namespace",
				},
			},
		}
	}

	ws1 := newWS("test-workspace-1", "default")
	ws2 := newWS("test-workspace-2", "another-namespace")

	deletionTime := metav1.Now()
	wsDeleted := newWS("test-workspace-deleted", "default")
	wsDeleted.DeletionTimestamp = &deletionTime
	wsDeleted.Finalizers = []string{"test-finalizer"}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws1, ws2, wsDeleted).Build()

	requests, err := GetWorkspaceReconciliationRequestsForIntegrationStrategy(
		context.Background(),
		fakeClient,
		"test-integration-strategy",
		"integration-strategy-namespace")
	assert.NoError(t, err)
	assert.Equal(t, 2, len(requests), "Expected 2 reconciliation requests")

	found := make(map[string]bool)
	for _, req := range requests {
		found[req.Namespace+"/"+req.Name] = true
	}
	assert.True(t, found["default/test-workspace-1"])
	assert.True(t, found["another-namespace/test-workspace-2"])
	assert.False(t, found["default/test-workspace-deleted"], "Deleted workspace should be skipped")
}

func TestGetWorkspaceReconciliationRequestsForIntegrationStrategy_OnListError_ReturnErrors(t *testing.T) {
	mockClient := &MockClient{
		ListError: fmt.Errorf("mock list error"),
	}

	requests, err := GetWorkspaceReconciliationRequestsForIntegrationStrategy(
		context.Background(),
		mockClient,
		"test-integration-strategy",
		"integration-strategy-namespace")

	assert.Error(t, err)
	assert.Nil(t, requests)
	assert.Contains(t, err.Error(), "failed to list workspaces by integration strategy")
}
