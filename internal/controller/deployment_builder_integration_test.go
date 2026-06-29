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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// baseDeployment returns a minimal deployment with one primary "workspace" container, matching the
// shape the builder produces before integrations are applied.
func baseDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "workspace", Image: "jupyter:latest"}},
				},
			},
		},
	}
}

// wiChildSpec builds a resolved WorkspaceIntegration child for ws-foo/user-ns with the given spec
// (the webhook-owned output fields already populated) and the identification label set.
func wiChildSpec(name string, spec workspacev1alpha1.WorkspaceIntegrationSpec) *workspacev1alpha1.WorkspaceIntegration {
	return &workspacev1alpha1.WorkspaceIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: "user-ns",
			Labels: map[string]string{workspaceutil.LabelWorkspaceName: "ws-foo"},
		},
		Spec: spec,
	}
}

func integrationTestWorkspace() *workspacev1alpha1.Workspace {
	return &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{Name: "ws-foo", Namespace: "user-ns"},
		Spec: workspacev1alpha1.WorkspaceSpec{
			IntegrationRefs: []workspacev1alpha1.IntegrationTemplateRef{{Name: "ray-integration"}},
		},
	}
}

func newIntegrationDeploymentBuilder(t *testing.T, objs ...*workspacev1alpha1.WorkspaceIntegration) *DeploymentBuilder {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, o := range objs {
		builder = builder.WithObjects(o)
	}
	return NewDeploymentBuilder(scheme, WorkspaceControllerOptions{}, builder.Build())
}

// TestApplyIntegrations_AppendsSidecarAndMergesEnv is the core deployment-build behavior:
// the resolved child's sidecar/volume/env are merged into the pod.
func TestApplyIntegrations_AppendsSidecarAndMergesEnv(t *testing.T) {
	spec := workspacev1alpha1.WorkspaceIntegrationSpec{
		DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
			PodModifications: &workspacev1alpha1.PodModifications{
				AdditionalContainers: []corev1.Container{{Name: "ray-sidecar", Image: "ray:2.9"}},
				Volumes:              []corev1.Volume{{Name: "ray-tmp"}},
				PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
					VolumeMounts: []corev1.VolumeMount{{Name: "ray-tmp", MountPath: "/tmp/ray"}},
					// MergeEnv holds literal values post-resolution (ValueTemplate == literal).
					MergeEnv: []workspacev1alpha1.AccessEnvTemplate{
						{Name: "RAY_ADDRESS", ValueTemplate: "head-svc:10001"},
					},
				},
			},
		},
	}
	db := newIntegrationDeploymentBuilder(t, wiChildSpec("workspace-ws-foo-ray-integration", spec))

	dep := baseDeployment()
	require.NoError(t, db.applyIntegrationsToDeployment(context.Background(), dep, integrationTestWorkspace()))

	containers := dep.Spec.Template.Spec.Containers
	require.Len(t, containers, 2)
	assert.Equal(t, "workspace", containers[0].Name, "primary container stays first")
	assert.Equal(t, "ray-sidecar", containers[1].Name)
	require.Len(t, dep.Spec.Template.Spec.Volumes, 1)
	assert.Equal(t, "ray-tmp", dep.Spec.Template.Spec.Volumes[0].Name)

	// Primary container got the env + volume mount.
	require.Len(t, containers[0].Env, 1)
	assert.Equal(t, "RAY_ADDRESS", containers[0].Env[0].Name)
	assert.Equal(t, "head-svc:10001", containers[0].Env[0].Value)
	require.Len(t, containers[0].VolumeMounts, 1)
	assert.Equal(t, "/tmp/ray", containers[0].VolumeMounts[0].MountPath)
}

// TestApplyIntegrations_ShareProcessNamespace verifies the pod-level toggle is set.
func TestApplyIntegrations_ShareProcessNamespace(t *testing.T) {
	enabled := true
	spec := workspacev1alpha1.WorkspaceIntegrationSpec{ShareProcessNamespace: &enabled}
	db := newIntegrationDeploymentBuilder(t, wiChildSpec("workspace-ws-foo-ray-integration", spec))

	dep := baseDeployment()
	require.NoError(t, db.applyIntegrationsToDeployment(context.Background(), dep, integrationTestWorkspace()))

	require.NotNil(t, dep.Spec.Template.Spec.ShareProcessNamespace)
	assert.True(t, *dep.Spec.Template.Spec.ShareProcessNamespace)
}

// TestApplyIntegrations_SkipsUnresolvedChild verifies a child with no resolved output (a bare shell)
// is skipped (the deployment is built without it) rather than erroring or panicking.
func TestApplyIntegrations_SkipsUnresolvedChild(t *testing.T) {
	db := newIntegrationDeploymentBuilder(t, wiChildSpec("workspace-ws-foo-ray-integration",
		workspacev1alpha1.WorkspaceIntegrationSpec{}))

	dep := baseDeployment()
	require.NoError(t, db.applyIntegrationsToDeployment(context.Background(), dep, integrationTestWorkspace()))

	assert.Len(t, dep.Spec.Template.Spec.Containers, 1, "unresolved child contributes nothing")
}

// TestApplyIntegrations_NoRefsNoList verifies that a workspace with no integrationRefs
// applies nothing (and does not require any children to exist).
func TestApplyIntegrations_NoRefsNoList(t *testing.T) {
	db := newIntegrationDeploymentBuilder(t)

	ws := integrationTestWorkspace()
	ws.Spec.IntegrationRefs = nil

	dep := baseDeployment()
	require.NoError(t, db.applyIntegrationsToDeployment(context.Background(), dep, ws))
	assert.Len(t, dep.Spec.Template.Spec.Containers, 1)
}

// TestApplyIntegrations_OnlyOwnWorkspace verifies children of a different workspace in the
// same namespace are not applied (label-scoped list).
func TestApplyIntegrations_OnlyOwnWorkspace(t *testing.T) {
	mine := wiChildSpec("workspace-ws-foo-ray-integration", workspacev1alpha1.WorkspaceIntegrationSpec{
		DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
			PodModifications: &workspacev1alpha1.PodModifications{
				AdditionalContainers: []corev1.Container{{Name: "mine-sidecar"}},
			},
		},
	})
	other := &workspacev1alpha1.WorkspaceIntegration{
		ObjectMeta: metav1.ObjectMeta{
			Name: "workspace-ws-bar-ray-integration", Namespace: "user-ns",
			Labels: map[string]string{workspaceutil.LabelWorkspaceName: "ws-bar"},
		},
		Spec: workspacev1alpha1.WorkspaceIntegrationSpec{
			DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
				PodModifications: &workspacev1alpha1.PodModifications{
					AdditionalContainers: []corev1.Container{{Name: "other-sidecar"}},
				},
			},
		},
	}
	db := newIntegrationDeploymentBuilder(t, mine, other)

	dep := baseDeployment()
	require.NoError(t, db.applyIntegrationsToDeployment(context.Background(), dep, integrationTestWorkspace()))

	require.Len(t, dep.Spec.Template.Spec.Containers, 2)
	assert.Equal(t, "mine-sidecar", dep.Spec.Template.Spec.Containers[1].Name)
}
