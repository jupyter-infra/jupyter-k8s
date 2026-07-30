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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResourceManager_createPVC(t *testing.T) {
	scheme := crudScheme(t)

	storageWorkspace := func() *workspacev1alpha1.Workspace {
		ws := crudWorkspace(false)
		ws.Spec.Storage = &workspacev1alpha1.StorageSpec{
			Size: resource.MustParse("1Gi"),
		}
		return ws
	}

	t.Run("happy path creates the PVC", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		pvc, err := rm.createPVC(context.Background(), storageWorkspace())

		require.NoError(t, err)
		require.NotNil(t, pvc)
		assert.Equal(t, GeneratePVCName(testWorkspaceName), pvc.Name)
	})

	t.Run("returns nil when no storage is requested", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		pvc, err := rm.createPVC(context.Background(), crudWorkspace(false))

		require.NoError(t, err)
		assert.Nil(t, pvc)
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

		_, err := rm.createPVC(context.Background(), storageWorkspace())

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create PVC")
	})
}

func TestResourceManager_EnsurePVCExists(t *testing.T) {
	scheme := crudScheme(t)

	t.Run("returns nil when no storage is requested", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		pvc, err := rm.EnsurePVCExists(context.Background(), crudWorkspace(false))

		require.NoError(t, err)
		assert.Nil(t, pvc)
	})

	t.Run("creates the PVC when absent", func(t *testing.T) {
		ws := crudWorkspace(false)
		ws.Spec.Storage = &workspacev1alpha1.StorageSpec{Size: resource.MustParse("1Gi")}
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		pvc, err := rm.EnsurePVCExists(context.Background(), ws)

		require.NoError(t, err)
		require.NotNil(t, pvc)
		assert.Equal(t, GeneratePVCName(ws.Name), pvc.Name)
	})

	t.Run("updates the PVC when available and size differs", func(t *testing.T) {
		ws := crudWorkspace(true)
		ws.Spec.Storage = &workspacev1alpha1.StorageSpec{Size: resource.MustParse("2Gi")}

		// Persist a PVC provisioned at a smaller size so NeedsUpdate is true.
		smaller := crudWorkspace(true)
		smaller.Spec.Storage = &workspacev1alpha1.StorageSpec{Size: resource.MustParse("1Gi")}
		rmBuild := newResourceManagerForCRUD(fake.NewClientBuilder().WithScheme(scheme).Build(), scheme)
		pvc, err := rmBuild.pvcBuilder.BuildPVC(smaller)
		require.NoError(t, err)

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		got, err := rm.EnsurePVCExists(context.Background(), ws)

		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("returns error when Get fails with a non-NotFound error", func(t *testing.T) {
		ws := crudWorkspace(false)
		ws.Spec.Storage = &workspacev1alpha1.StorageSpec{Size: resource.MustParse("1Gi")}
		base := fake.NewClientBuilder().WithScheme(scheme).Build()
		mock := &MockClient{
			Client: base,
			getFunc: func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
				return apierrors.NewInternalError(errInjected)
			},
		}
		rm := newResourceManagerForCRUD(mock, scheme)

		_, err := rm.EnsurePVCExists(context.Background(), ws)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get PVC")
	})
}

func TestResourceManager_ensurePVCUpToDate(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(true)
	ws.Spec.Storage = &workspacev1alpha1.StorageSpec{Size: resource.MustParse("1Gi")}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GeneratePVCName(ws.Name),
			Namespace: ws.Namespace,
		},
	}

	t.Run("skips update when workspace is not available", func(t *testing.T) {
		notAvail := crudWorkspace(false)
		notAvail.Spec.Storage = &workspacev1alpha1.StorageSpec{Size: resource.MustParse("1Gi")}
		base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
		mock := &MockClient{
			Client: base,
			updateFunc: func(context.Context, client.Object, ...client.UpdateOption) error {
				return errInjected
			},
		}
		rm := newResourceManagerForCRUD(mock, scheme)

		got, err := rm.ensurePVCUpToDate(context.Background(), pvc, notAvail)

		require.NoError(t, err)
		assert.Same(t, pvc, got)
	})

	t.Run("returns error when NeedsUpdate fails", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
		rm := newResourceManagerBrokenBuilders(c)

		_, err := rm.ensurePVCUpToDate(context.Background(), pvc, ws)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check if PVC needs update")
	})
}

func TestResourceManager_updatePVC(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(true)
	ws.Spec.Storage = &workspacev1alpha1.StorageSpec{Size: resource.MustParse("1Gi")}
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GeneratePVCName(ws.Name),
			Namespace: ws.Namespace,
		},
	}

	t.Run("returns error when UpdatePVCSpec fails", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
		rm := newResourceManagerBrokenBuilders(c)

		_, err := rm.updatePVC(context.Background(), pvc, ws)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update PVC spec")
	})

	t.Run("returns error when Update fails", func(t *testing.T) {
		base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
		mock := &MockClient{
			Client: base,
			updateFunc: func(context.Context, client.Object, ...client.UpdateOption) error {
				return errInjected
			},
		}
		rm := newResourceManagerForCRUD(mock, scheme)

		_, err := rm.updatePVC(context.Background(), pvc, ws)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update PVC")
	})
}

func TestResourceManager_EnsurePVCDeleted(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(false)

	t.Run("returns nil when PVC is already absent", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		pvc, err := rm.EnsurePVCDeleted(context.Background(), ws)

		require.NoError(t, err)
		assert.Nil(t, pvc)
	})

	t.Run("deletes an existing PVC", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GeneratePVCName(ws.Name),
				Namespace: ws.Namespace,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		got, err := rm.EnsurePVCDeleted(context.Background(), ws)

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

		_, err := rm.EnsurePVCDeleted(context.Background(), ws)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get PVC")
	})
}
