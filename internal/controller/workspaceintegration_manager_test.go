/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func wiManagerTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
	return scheme
}

func wiManagerTestWorkspace(refNames ...string) *workspacev1alpha1.Workspace {
	refs := make([]workspacev1alpha1.IntegrationTemplateRef, 0, len(refNames))
	for _, n := range refNames {
		refs = append(refs, workspacev1alpha1.IntegrationTemplateRef{Name: n})
	}
	return &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-foo", Namespace: "user-ns", UID: "ws-foo-uid"},
		Spec:       workspacev1alpha1.WorkspaceSpec{IntegrationRefs: refs},
	}
}

// TestEnsureForWorkspace_CreatesShellWithOwnerRefAndLabel verifies a missing child is created with
// the workspace as controller owner, the identification label, and the right templateRef/workspaceRef
// -- and that the resolved output fields are left for the webhook (not set by the manager).
func TestEnsureForWorkspace_CreatesShellWithOwnerRefAndLabel(t *testing.T) {
	scheme := wiManagerTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := NewWorkspaceIntegrationManager(c, scheme)
	ws := wiManagerTestWorkspace("ray-integration")

	result, err := m.EnsureForWorkspace(context.Background(), ws)
	require.NoError(t, err)
	require.Len(t, result, 1)

	wi := result[0]
	assert.Equal(t, "workspace-ws-foo-ray-integration", wi.Name)
	assert.Equal(t, "user-ns", wi.Namespace)
	assert.Equal(t, "ws-foo", wi.Labels[workspaceutil.LabelWorkspaceName], "identification label (option B)")
	assert.Equal(t, "ray-integration", wi.Spec.TemplateRef.Name)
	assert.Equal(t, "ws-foo", wi.Spec.WorkspaceRef.Name)
	assert.Equal(t, "user-ns", wi.Spec.WorkspaceRef.Namespace)
	assert.Nil(t, wi.Spec.DeploymentModifications, "manager must not set resolved output -- the webhook owns it")
	assert.Nil(t, wi.Spec.StatusProbe, "manager must not set resolved output -- the webhook owns it")
	assert.Nil(t, wi.Spec.ShareProcessNamespace, "manager must not set resolved output -- the webhook owns it")

	require.Len(t, wi.OwnerReferences, 1)
	owner := wi.OwnerReferences[0]
	assert.Equal(t, "Workspace", owner.Kind)
	assert.Equal(t, "ws-foo", owner.Name)
	require.NotNil(t, owner.Controller)
	assert.True(t, *owner.Controller, "ownerRef must be a controller ref for GC + re-enqueue")
}

// TestEnsureForWorkspace_Idempotent verifies a second reconcile with no spec change creates nothing
// new and preserves the webhook-owned resolved output.
func TestEnsureForWorkspace_Idempotent(t *testing.T) {
	scheme := wiManagerTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := NewWorkspaceIntegrationManager(c, scheme)
	ws := wiManagerTestWorkspace("ray-integration")

	first, err := m.EnsureForWorkspace(context.Background(), ws)
	require.NoError(t, err)
	require.Len(t, first, 1)

	// Simulate the webhook having resolved the output fields on the persisted child.
	baked := first[0].DeepCopy()
	baked.Spec.DeploymentModifications = &workspacev1alpha1.DeploymentModifications{
		PodModifications: &workspacev1alpha1.PodModifications{
			PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
				MergeEnv: []workspacev1alpha1.AccessEnvTemplate{{Name: "RAY_ADDRESS", ValueTemplate: "svc"}},
			},
		},
	}
	require.NoError(t, c.Update(context.Background(), baked))

	second, err := m.EnsureForWorkspace(context.Background(), ws)
	require.NoError(t, err)
	require.Len(t, second, 1, "no duplicate child created")
	require.NotNil(t, second[0].Spec.DeploymentModifications, "manager must not clobber webhook-resolved output")
	assert.Equal(t, "svc",
		second[0].Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications.MergeEnv[0].ValueTemplate)
}

// TestEnsureForWorkspace_UpdatesOnTemplateDrift verifies that changing an integrationRef's template
// updates the existing child's templateRef (re-triggering the webhook bake).
func TestEnsureForWorkspace_UpdatesOnTemplateDrift(t *testing.T) {
	scheme := wiManagerTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := NewWorkspaceIntegrationManager(c, scheme)

	// First reconcile with a parameter value.
	ws := wiManagerTestWorkspace("ray-integration")
	ws.Spec.IntegrationRefs[0].Parameters = []workspacev1alpha1.IntegrationParameter{
		{Name: "clusterName", Value: "old-cluster"},
	}
	_, err := m.EnsureForWorkspace(context.Background(), ws)
	require.NoError(t, err)

	// Change the parameter (e.g. switch clusters) and re-reconcile.
	ws.Spec.IntegrationRefs[0].Parameters[0].Value = "new-cluster"
	result, err := m.EnsureForWorkspace(context.Background(), ws)
	require.NoError(t, err)
	require.Len(t, result, 1)
	require.Len(t, result[0].Spec.TemplateRef.Parameters, 1)
	assert.Equal(t, "new-cluster", result[0].Spec.TemplateRef.Parameters[0].Value,
		"templateRef drift must be propagated to the child so the webhook re-bakes")
}

// TestEnsureForWorkspace_DeletesStale verifies a child whose integrationRef was removed is deleted.
func TestEnsureForWorkspace_DeletesStale(t *testing.T) {
	scheme := wiManagerTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	m := NewWorkspaceIntegrationManager(c, scheme)

	// Start with one integration.
	ws := wiManagerTestWorkspace("ray-integration")
	_, err := m.EnsureForWorkspace(context.Background(), ws)
	require.NoError(t, err)

	// Remove all integrationRefs and re-reconcile -> the child should be deleted.
	ws.Spec.IntegrationRefs = nil
	result, err := m.EnsureForWorkspace(context.Background(), ws)
	require.NoError(t, err)
	assert.Empty(t, result, "stale child must be deleted when its integrationRef is removed")
}

// TestListForWorkspace_FiltersByLabel verifies listing returns only children labeled for the
// workspace, not children of a different workspace in the same namespace.
func TestListForWorkspace_FiltersByLabel(t *testing.T) {
	scheme := wiManagerTestScheme(t)

	mine := &workspacev1alpha1.WorkspaceIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "workspace-ws-foo-ray", Namespace: "user-ns",
			Labels: map[string]string{workspaceutil.LabelWorkspaceName: "ws-foo"},
		},
	}
	other := &workspacev1alpha1.WorkspaceIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "workspace-ws-bar-ray", Namespace: "user-ns",
			Labels: map[string]string{workspaceutil.LabelWorkspaceName: "ws-bar"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(mine, other).Build()
	m := NewWorkspaceIntegrationManager(c, scheme)

	result, err := m.ListForWorkspace(context.Background(), wiManagerTestWorkspace())
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "workspace-ws-foo-ray", result[0].Name)
}
