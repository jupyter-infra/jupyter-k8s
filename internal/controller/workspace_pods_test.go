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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

func TestWorkspaceRunningPod(t *testing.T) {
	workspace := &workspacev1alpha1.Workspace{ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName, Namespace: testNamespaceName}}
	pod := func(name string, labels map[string]string, phase corev1.PodPhase) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: testNamespaceName, Labels: labels},
			Status:     corev1.PodStatus{Phase: phase},
		}
	}
	now := metav1.Now()
	terminating := pod("terminating", GenerateLabels(testWorkspaceName), corev1.PodRunning)
	terminating.DeletionTimestamp = &now
	terminating.Finalizers = []string{stepTestFinalizer}

	tests := []struct {
		name     string
		objects  []client.Object
		listErr  error
		expected string
	}{
		{name: "the running pod", objects: []client.Object{pod("running", GenerateLabels(testWorkspaceName), corev1.PodRunning)}, expected: "running"},
		{name: "a pending pod is not running", objects: []client.Object{pod("pending", GenerateLabels(testWorkspaceName), corev1.PodPending)}},
		{name: "a terminating pod is skipped", objects: []client.Object{terminating}},
		{name: "the running pod is preferred over the terminating one",
			objects:  []client.Object{terminating, pod("running", GenerateLabels(testWorkspaceName), corev1.PodRunning)},
			expected: "running"},
		{name: "another workspace's pod is not consulted", objects: []client.Object{pod("other", GenerateLabels(stepTestOtherName), corev1.PodRunning)}},
		{name: "a list failure is returned", objects: []client.Object{pod("running", GenerateLabels(testWorkspaceName), corev1.PodRunning)}, listErr: errInjected},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c client.Client = fake.NewClientBuilder().WithScheme(crudScheme(t)).WithObjects(tt.objects...).Build()
			if tt.listErr != nil {
				c = &MockClient{Client: c, listFunc: func(context.Context, client.ObjectList, ...client.ListOption) error { return tt.listErr }}
			}
			found, err := workspaceRunningPod(context.Background(), c, workspace)
			switch {
			case tt.listErr != nil:
				require.ErrorIs(t, err, tt.listErr)
			case tt.expected == "":
				require.ErrorIs(t, err, errNoRunningPod)
			default:
				require.NoError(t, err)
				assert.Equal(t, tt.expected, found.Name)
			}
		})
	}
}
