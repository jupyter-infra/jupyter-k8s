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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResourceManager_createService(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(false)

	t.Run("happy path creates and returns the service", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		svc, err := rm.createService(context.Background(), ws)

		require.NoError(t, err)
		assert.Equal(t, GenerateServiceName(ws.Name), svc.Name)

		// Confirm it was actually persisted.
		got := &corev1.Service{}
		require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(svc), got))
	})

	t.Run("returns error when BuildService fails", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerBrokenBuilders(c)

		_, err := rm.createService(context.Background(), ws)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to build service")
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

		_, err := rm.createService(context.Background(), ws)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create service")
	})
}

func TestResourceManager_deleteService(t *testing.T) {
	scheme := crudScheme(t)

	t.Run("returns error when Delete fails", func(t *testing.T) {
		base := fake.NewClientBuilder().WithScheme(scheme).Build()
		mock := &MockClient{
			Client: base,
			deleteFunc: func(context.Context, client.Object, ...client.DeleteOption) error {
				return errInjected
			},
		}
		rm := newResourceManagerForCRUD(mock, scheme)

		err := rm.deleteService(context.Background(), &corev1.Service{})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete service")
	})
}

func TestResourceManager_updateService(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(true)

	t.Run("happy path updates the service", func(t *testing.T) {
		rmBuild := newResourceManagerForCRUD(fake.NewClientBuilder().WithScheme(scheme).Build(), scheme)
		svc, err := rmBuild.serviceBuilder.BuildService(ws)
		require.NoError(t, err)

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		got, err := rm.updateService(context.Background(), svc, ws)

		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("returns error when Update fails", func(t *testing.T) {
		rmBuild := newResourceManagerForCRUD(fake.NewClientBuilder().WithScheme(scheme).Build(), scheme)
		svc, err := rmBuild.serviceBuilder.BuildService(ws)
		require.NoError(t, err)

		base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()
		mock := &MockClient{
			Client: base,
			updateFunc: func(context.Context, client.Object, ...client.UpdateOption) error {
				return errInjected
			},
		}
		rm := newResourceManagerForCRUD(mock, scheme)

		_, err = rm.updateService(context.Background(), svc, ws)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update service")
	})

	t.Run("returns error when UpdateServiceSpec fails", func(t *testing.T) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateServiceName(ws.Name),
				Namespace: ws.Namespace,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()
		rm := newResourceManagerBrokenBuilders(c)

		_, err := rm.updateService(context.Background(), svc, ws)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update service spec")
	})
}

func TestResourceManager_ensureServiceUpToDate(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(true)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateServiceName(ws.Name),
			Namespace: ws.Namespace,
		},
	}

	t.Run("returns error when NeedsUpdate fails", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()
		rm := newResourceManagerBrokenBuilders(c)

		_, err := rm.ensureServiceUpToDate(context.Background(), svc, ws)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check if service needs update")
	})
}

func TestResourceManager_EnsureServiceExists(t *testing.T) {
	scheme := crudScheme(t)

	t.Run("creates the service when absent", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		svc, err := rm.EnsureServiceExists(context.Background(), crudWorkspace(false))

		require.NoError(t, err)
		assert.Equal(t, GenerateServiceName(testWorkspaceName), svc.Name)
	})

	t.Run("updates the service when available and spec differs", func(t *testing.T) {
		ws := crudWorkspace(true)
		// Persist a service whose spec differs from the desired one so NeedsUpdate is true.
		rmBuild := newResourceManagerForCRUD(fake.NewClientBuilder().WithScheme(scheme).Build(), scheme)
		svc, err := rmBuild.serviceBuilder.BuildService(ws)
		require.NoError(t, err)
		svc.Spec.Ports[0].Port = 9999 // drift from the builder's desired port

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		got, err := rm.EnsureServiceExists(context.Background(), ws)

		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("returns error when Get fails with a non-NotFound error", func(t *testing.T) {
		base := fake.NewClientBuilder().WithScheme(scheme).Build()
		mock := &MockClient{
			Client: base,
			getFunc: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
				return apierrors.NewInternalError(errInjected)
			},
		}
		rm := newResourceManagerForCRUD(mock, scheme)

		_, err := rm.EnsureServiceExists(context.Background(), crudWorkspace(false))

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get service")
	})
}

func TestResourceManager_EnsureServiceDeleted(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(false)
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GenerateServiceName(ws.Name),
			Namespace: ws.Namespace,
		},
	}

	t.Run("returns error when Delete fails", func(t *testing.T) {
		base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()
		mock := &MockClient{
			Client: base,
			deleteFunc: func(context.Context, client.Object, ...client.DeleteOption) error {
				return errInjected
			},
		}
		rm := newResourceManagerForCRUD(mock, scheme)

		_, err := rm.EnsureServiceDeleted(context.Background(), ws)

		require.Error(t, err)
	})
}
