/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"errors"
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// errInjected is a sentinel error used across the controller tests to drive
// client failure branches via MockClient.
var errInjected = errors.New("injected client failure")

// crudScheme returns a scheme with the types needed for ResourceManager CRUD tests.
func crudScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	return scheme
}

// crudWorkspace builds a minimal workspace for CRUD tests. When available is
// true, it carries an Available=True condition so the ensure*UpToDate paths run.
func crudWorkspace(available bool) *workspacev1alpha1.Workspace {
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testWorkspaceName,
			Namespace: testNamespaceName,
			UID:       testUID,
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: testWorkspaceDisplayName,
			Image:       testImage,
		},
	}
	if available {
		ws.Status.Conditions = []metav1.Condition{
			{Type: ConditionTypeAvailable, Status: metav1.ConditionTrue},
		}
	}
	return ws
}

// newResourceManagerForCRUD wires a ResourceManager with real builders around the
// provided client and scheme.
func newResourceManagerForCRUD(c client.Client, scheme *runtime.Scheme) *ResourceManager {
	return NewResourceManager(
		c,
		scheme,
		NewDeploymentBuilder(scheme, WorkspaceControllerOptions{}),
		NewServiceBuilder(scheme),
		NewPVCBuilder(scheme),
		NewAccessResourcesBuilder(),
		NewStatusManager(c),
	)
}

// newResourceManagerBrokenBuilders returns a ResourceManager whose builders use a
// scheme without the Workspace type, so any Build*/NeedsUpdate/UpdateSpec call
// fails at SetControllerReference. The client keeps a valid scheme so persisted
// core/apps objects can still be read back.
func newResourceManagerBrokenBuilders(c client.Client) *ResourceManager {
	broken := runtime.NewScheme()
	// Register only the resource types, NOT the Workspace owner type.
	_ = corev1.AddToScheme(broken)
	_ = appsv1.AddToScheme(broken)
	return NewResourceManager(
		c,
		broken,
		NewDeploymentBuilder(broken, WorkspaceControllerOptions{}),
		NewServiceBuilder(broken),
		NewPVCBuilder(broken),
		NewAccessResourcesBuilder(),
		NewStatusManager(c),
	)
}

// TestIsDeploymentRolloutComplete covers the probe gate's readiness test. The ObservedGeneration case is
// the one that matters in practice: attaching an integration rewrites the pod template, which bumps the
// generation while every replica count still describes the outgoing revision.
func TestIsDeploymentRolloutComplete(t *testing.T) {
	one := int32(1)
	deployment := func(mutate func(*appsv1.Deployment)) *appsv1.Deployment {
		d := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Generation: 4},
			Spec:       appsv1.DeploymentSpec{Replicas: &one},
			Status: appsv1.DeploymentStatus{
				ObservedGeneration: 4, Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1,
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
				},
			},
		}
		if mutate != nil {
			mutate(d)
		}
		return d
	}

	cases := []struct {
		name   string
		deploy *appsv1.Deployment
		want   bool
	}{
		{name: "settled rollout", deploy: deployment(nil), want: true},
		{
			name: "template just changed, generation not yet observed",
			// Available=True still describes the previous revision here, which is why the gate cannot
			// use IsDeploymentAvailable.
			deploy: deployment(func(d *appsv1.Deployment) { d.Generation = 5 }),
			want:   false,
		},
		{
			name:   "new replica not yet created",
			deploy: deployment(func(d *appsv1.Deployment) { d.Status.UpdatedReplicas = 0 }),
			want:   false,
		},
		{
			name:   "new replica created but not ready",
			deploy: deployment(func(d *appsv1.Deployment) { d.Status.ReadyReplicas = 0 }),
			want:   false,
		},
		{
			name: "old replica still terminating alongside the new one",
			// Recreate scales down before up, so Replicas above the spec means the swap is still underway.
			deploy: deployment(func(d *appsv1.Deployment) { d.Status.Replicas = 2 }),
			want:   false,
		},
		{name: caseNilDeployment, deploy: nil, want: false},
		{
			name:   "nil replica count",
			deploy: deployment(func(d *appsv1.Deployment) { d.Spec.Replicas = nil }),
			want:   false,
		},
	}

	rm := &ResourceManager{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, rm.IsDeploymentRolloutComplete(tc.deploy))
		})
	}
}
