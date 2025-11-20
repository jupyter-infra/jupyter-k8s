/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package workspace

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetPodUIDFromWorkspaceName_Success(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			UID:       types.UID("test-pod-uid-123"),
			Labels: map[string]string{
				"workspace.jupyter.org/workspace-name": "test-workspace",
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(pod).Build()

	uid, err := GetPodUIDFromWorkspaceName(fakeClient, "test-workspace")

	assert.NoError(t, err)
	assert.Equal(t, "test-pod-uid-123", uid)
}

func TestGetPodUIDFromWorkspaceName_NoPodFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	uid, err := GetPodUIDFromWorkspaceName(fakeClient, "nonexistent-workspace")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no pod found for workspace")
	assert.Empty(t, uid)
}
