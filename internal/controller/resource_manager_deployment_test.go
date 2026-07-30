/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResourceManager_getDeployment(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(false)

	t.Run("happy path returns the existing deployment", func(t *testing.T) {
		existing := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateDeploymentName(ws.Name),
				Namespace: ws.Namespace,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existing).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		got, err := rm.getDeployment(context.Background(), ws)

		require.NoError(t, err)
		assert.Equal(t, GenerateDeploymentName(ws.Name), got.Name)
	})

	t.Run("returns NotFound when the deployment is absent", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		_, err := rm.getDeployment(context.Background(), ws)

		require.Error(t, err)
		assert.True(t, apierrors.IsNotFound(err))
	})
}

func TestResourceManager_createDeployment(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(false)

	t.Run("happy path creates the deployment", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		dep, err := rm.createDeployment(context.Background(), ws, nil)

		require.NoError(t, err)
		assert.Equal(t, GenerateDeploymentName(ws.Name), dep.Name)
	})

	t.Run("returns error when Create fails", func(t *testing.T) {
		base := fake.NewClientBuilder().WithScheme(scheme).Build()
		mock := &MockClient{
			Client: base,
			createFunc: func(context.Context, client.Object, ...client.CreateOption) error {
				return errInjected
			},
		}
		rm := newResourceManagerForCRUD(mock, scheme)

		_, err := rm.createDeployment(context.Background(), ws, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create deployment")
	})
}

func TestResourceManager_deleteDeployment(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(false)

	t.Run("happy path deletes the deployment", func(t *testing.T) {
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateDeploymentName(ws.Name),
				Namespace: ws.Namespace,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		require.NoError(t, rm.deleteDeployment(context.Background(), dep))
	})

	t.Run("returns error when Delete fails", func(t *testing.T) {
		base := fake.NewClientBuilder().WithScheme(scheme).Build()
		mock := &MockClient{
			Client: base,
			deleteFunc: func(context.Context, client.Object, ...client.DeleteOption) error {
				return errInjected
			},
		}
		rm := newResourceManagerForCRUD(mock, scheme)

		err := rm.deleteDeployment(context.Background(), &appsv1.Deployment{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete deployment")
	})
}

func TestResourceManager_ensureDeploymentUpToDate(t *testing.T) {
	scheme := crudScheme(t)

	t.Run("skips update when workspace is not available", func(t *testing.T) {
		// A client whose Update always fails proves the update path is not reached.
		base := fake.NewClientBuilder().WithScheme(scheme).Build()
		mock := &MockClient{
			Client: base,
			updateFunc: func(context.Context, client.Object, ...client.UpdateOption) error {
				return errInjected
			},
		}
		rm := newResourceManagerForCRUD(mock, scheme)
		existing := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateDeploymentName(testWorkspaceName),
				Namespace: testNamespaceName,
			},
		}

		got, err := rm.ensureDeploymentUpToDate(context.Background(), existing, crudWorkspace(false), nil)

		require.NoError(t, err)
		assert.Same(t, existing, got)
	})

	t.Run("updates when available and spec differs", func(t *testing.T) {
		ws := crudWorkspace(true)
		// Persist a deployment built for a different image so NeedsUpdate is true.
		stale := crudWorkspace(true)
		stale.Spec.Image = "old-image"
		rmBuild := newResourceManagerForCRUD(fake.NewClientBuilder().WithScheme(scheme).Build(), scheme)
		staleDep, err := rmBuild.deploymentBuilder.BuildDeploymentWithAccessStrategy(context.Background(), stale, nil)
		require.NoError(t, err)

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(staleDep).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		got, err := rm.ensureDeploymentUpToDate(context.Background(), staleDep, ws, nil)

		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("returns error when NeedsUpdate fails", func(t *testing.T) {
		ws := crudWorkspace(true)
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateDeploymentName(ws.Name),
				Namespace: ws.Namespace,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()
		rm := newResourceManagerBrokenBuilders(c)

		_, err := rm.ensureDeploymentUpToDate(context.Background(), dep, ws, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check if deployment needs update")
	})
}

func TestResourceManager_updateDeployment(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(true)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateDeploymentName(ws.Name),
			Namespace: ws.Namespace,
		},
	}

	t.Run("returns error when BuildDeployment fails", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()
		rm := newResourceManagerBrokenBuilders(c)

		_, err := rm.updateDeployment(context.Background(), dep, ws, nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to build updated deployment")
	})
}
