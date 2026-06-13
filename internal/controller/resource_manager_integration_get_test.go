/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetIntegrationStrategyForWorkspace_NilRef(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	rm := &ResourceManager{client: client}

	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "user-ns"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName:         "ws",
			IntegrationStrategy: nil,
		},
	}

	result, err := rm.GetIntegrationStrategyForWorkspace(context.Background(), ws)
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestGetIntegrationStrategyForWorkspace_ExplicitNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	strategy := &workspacev1alpha1.WorkspaceIntegrationStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: "ray-connector", Namespace: "jupyter-system"},
		Spec: workspacev1alpha1.WorkspaceIntegrationStrategySpec{
			DisplayName: "Ray Connector",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(strategy).Build()
	rm := &ResourceManager{client: client}

	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "user-ns"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "ws",
			IntegrationStrategy: &workspacev1alpha1.IntegrationStrategyRef{
				Name:      "ray-connector",
				Namespace: "jupyter-system",
			},
		},
	}

	result, err := rm.GetIntegrationStrategyForWorkspace(context.Background(), ws)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ray-connector", result.Name)
	assert.Equal(t, "jupyter-system", result.Namespace)
}

func TestGetIntegrationStrategyForWorkspace_DefaultsToWorkspaceNamespace(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	strategy := &workspacev1alpha1.WorkspaceIntegrationStrategy{
		ObjectMeta: metav1.ObjectMeta{Name: "ray-connector", Namespace: "user-ns"},
		Spec: workspacev1alpha1.WorkspaceIntegrationStrategySpec{
			DisplayName: "Ray Connector",
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(strategy).Build()
	rm := &ResourceManager{client: client}

	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "user-ns"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "ws",
			IntegrationStrategy: &workspacev1alpha1.IntegrationStrategyRef{
				Name: "ray-connector",
				// Namespace omitted — should default to workspace namespace
			},
		},
	}

	result, err := rm.GetIntegrationStrategyForWorkspace(context.Background(), ws)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ray-connector", result.Name)
	assert.Equal(t, "user-ns", result.Namespace)
}

func TestGetIntegrationStrategyForWorkspace_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))

	client := fake.NewClientBuilder().WithScheme(scheme).Build()
	rm := &ResourceManager{client: client}

	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws", Namespace: "user-ns"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "ws",
			IntegrationStrategy: &workspacev1alpha1.IntegrationStrategyRef{
				Name:      "missing-strategy",
				Namespace: "jupyter-system",
			},
		},
	}

	result, err := rm.GetIntegrationStrategyForWorkspace(context.Background(), ws)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
	assert.Contains(t, err.Error(), "missing-strategy")
	assert.Contains(t, err.Error(), "jupyter-system")
	assert.Nil(t, result)
}
