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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestResourceManager_AreAllResourcesDeleted(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(false)

	t.Run("true when all resources are absent", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		assert.True(t, rm.AreAllResourcesDeleted(context.Background(), ws))
	})

	t.Run("false when the deployment still exists", func(t *testing.T) {
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateDeploymentName(ws.Name),
				Namespace: ws.Namespace,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		assert.False(t, rm.AreAllResourcesDeleted(context.Background(), ws))
	})

	t.Run("false when the service still exists", func(t *testing.T) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateServiceName(ws.Name),
				Namespace: ws.Namespace,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(svc).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		assert.False(t, rm.AreAllResourcesDeleted(context.Background(), ws))
	})

	t.Run("false when the PVC still exists", func(t *testing.T) {
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GeneratePVCName(ws.Name),
				Namespace: ws.Namespace,
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pvc).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		assert.False(t, rm.AreAllResourcesDeleted(context.Background(), ws))
	})

	t.Run("false when access resources remain in status", func(t *testing.T) {
		wsWithAccess := crudWorkspace(false)
		wsWithAccess.Status.AccessResources = []workspacev1alpha1.AccessResourceStatus{
			{Name: "some-route"},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		assert.False(t, rm.AreAllResourcesDeleted(context.Background(), wsWithAccess))
	})
}

func TestResourceManager_CleanupAllResources(t *testing.T) {
	scheme := crudScheme(t)
	ws := crudWorkspace(false)

	t.Run("returns true when everything is already deleted", func(t *testing.T) {
		c := fake.NewClientBuilder().WithScheme(scheme).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		done, err := rm.CleanupAllResources(context.Background(), ws)

		require.NoError(t, err)
		assert.True(t, done)
	})

	t.Run("returns false when resources are still being deleted", func(t *testing.T) {
		// A deployment with a deletion timestamp is "deleting", so cleanup issues no
		// new delete but AreAllResourcesDeleted reports not-yet-gone.
		now := metav1.Now()
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:              GenerateDeploymentName(ws.Name),
				Namespace:         ws.Namespace,
				DeletionTimestamp: &now,
				Finalizers:        []string{"keep-around/test"},
			},
		}
		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()
		rm := newResourceManagerForCRUD(c, scheme)

		done, err := rm.CleanupAllResources(context.Background(), ws)

		require.NoError(t, err)
		assert.False(t, done)
	})

	t.Run("propagates deployment deletion errors", func(t *testing.T) {
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateDeploymentName(ws.Name),
				Namespace: ws.Namespace,
			},
		}
		base := fake.NewClientBuilder().WithScheme(scheme).WithObjects(dep).Build()
		mock := &MockClient{
			Client: base,
			deleteFunc: func(context.Context, client.Object, ...client.DeleteOption) error {
				return errInjected
			},
		}
		rm := newResourceManagerForCRUD(mock, scheme)

		done, err := rm.CleanupAllResources(context.Background(), ws)

		require.Error(t, err)
		assert.False(t, done)
	})
}
