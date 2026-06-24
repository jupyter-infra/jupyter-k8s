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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newUnstructured builds a fetchable unstructured object for the fake client.
func newUnstructured(apiVersion, kind, name, namespace string, content map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": namespace,
		},
	}}
	for k, v := range content {
		obj.Object[k] = v
	}
	return obj
}

// fakeClientWithObjects constructs a fake client seeded with the given unstructured objects,
// registering their GVKs on the scheme.
func fakeClientWithObjects(t *testing.T, gvks []schema.GroupVersionKind, objs ...*unstructured.Unstructured) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, workspacev1alpha1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	for _, gvk := range gvks {
		if scheme.Recognizes(gvk) {
			continue
		}
		scheme.AddKnownTypeWithName(gvk, &unstructured.Unstructured{})
		listGVK := gvk
		listGVK.Kind = gvk.Kind + "List"
		scheme.AddKnownTypeWithName(listGVK, &unstructured.UnstructuredList{})
	}
	builder := fake.NewClientBuilder().WithScheme(scheme)
	for _, o := range objs {
		builder = builder.WithObjects(o)
	}
	return builder.Build()
}

// newTestWI builds a WorkspaceIntegration shell (as the controller would create it) pointing at
// the ray-integration template with the cluster-name parameter, owned by my-workspace in user-ns.
func newTestWI() *workspacev1alpha1.WorkspaceIntegration {
	return &workspacev1alpha1.WorkspaceIntegration{
		ObjectMeta: metav1.ObjectMeta{Name: "my-workspace-ray-integration", Namespace: "user-ns"},
		Spec: workspacev1alpha1.WorkspaceIntegrationSpec{
			TemplateRef: workspacev1alpha1.IntegrationTemplateRef{
				Name: "ray-integration",
				Parameters: []workspacev1alpha1.IntegrationParameter{
					{Name: testParamClusterName, Value: "my-ray-cluster"},
				},
			},
			WorkspaceRef: workspacev1alpha1.WorkspaceRef{Name: "my-workspace", Namespace: "user-ns"},
		},
	}
}

// firstMergeEnv returns the PrimaryContainerModifications.MergeEnv from a resolved WI spec, or nil.
func firstMergeEnv(wi *workspacev1alpha1.WorkspaceIntegration) []workspacev1alpha1.AccessEnvTemplate {
	if wi.Spec.DeploymentModifications == nil ||
		wi.Spec.DeploymentModifications.PodModifications == nil ||
		wi.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications == nil {
		return nil
	}
	return wi.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications.MergeEnv
}

// firstAdditionalContainers returns the resolved additional containers, or nil.
func firstAdditionalContainers(wi *workspacev1alpha1.WorkspaceIntegration) []corev1.Container {
	if wi.Spec.DeploymentModifications == nil || wi.Spec.DeploymentModifications.PodModifications == nil {
		return nil
	}
	return wi.Spec.DeploymentModifications.PodModifications.AdditionalContainers
}

// TestBuildWorkspaceIntegration_ResolvesToLiteralValues is the core admission-time behavior: the
// builder fetches the referenced RayCluster, resolves the template expressions, and writes the
// resolved output onto wi.Spec with env/sidecar fields holding literal values (no {{ }} remaining).
func TestBuildWorkspaceIntegration_ResolvesToLiteralValues(t *testing.T) {
	rayGVK := schema.GroupVersionKind{Group: "ray.io", Version: "v1", Kind: testKindRayCluster}
	rayCluster := newUnstructured("ray.io/v1", testKindRayCluster, "my-ray-cluster", "user-ns",
		map[string]interface{}{
			"status": map[string]interface{}{
				"head": map[string]interface{}{"serviceName": "my-ray-cluster-head-svc"},
			},
		})
	c := fakeClientWithObjects(t, []schema.GroupVersionKind{rayGVK}, rayCluster)

	template := &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ray-integration", Namespace: "user-ns"},
		Spec: workspacev1alpha1.WorkspaceIntegrationTemplateSpec{
			DisplayName: "Ray Cluster Integration",
			ResourceRefs: []workspacev1alpha1.ResourceRef{
				{ID: testRefIDRayCluster, APIVersion: "ray.io/v1", Kind: testKindRayCluster, Name: "{{ .Parameters.clusterName }}"},
			},
			DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
				PodModifications: &workspacev1alpha1.PodModifications{
					AdditionalContainers: []corev1.Container{
						{Name: "ray-sidecar", Image: "ray:2.9"},
					},
					PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
						MergeEnv: []workspacev1alpha1.AccessEnvTemplate{
							{Name: testEnvRayAddress, ValueTemplate: `{{ resource "rayCluster" "{.status.head.serviceName}" }}`},
						},
					},
				},
			},
		},
	}

	wi := newTestWI()
	require.NoError(t, BuildWorkspaceIntegration(context.Background(), c, wi, template))

	containers := firstAdditionalContainers(wi)
	require.Len(t, containers, 1)
	assert.Equal(t, "ray-sidecar", containers[0].Name)
	env := firstMergeEnv(wi)
	require.Len(t, env, 1)
	assert.Equal(t, testEnvRayAddress, env[0].Name)
	assert.Equal(t, "my-ray-cluster-head-svc", env[0].ValueTemplate,
		"env value must be the resolved literal, not a {{ }} expression")
}

// TestBuildWorkspaceIntegration_FreezesStatusProbe verifies the template's statusProbe is frozen
// onto wi.Spec.StatusProbe so the controller's liveness probe never re-reads the template.
func TestBuildWorkspaceIntegration_FreezesStatusProbe(t *testing.T) {
	c := fakeClientWithObjects(t, nil)
	template := &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ray-integration", Namespace: "user-ns"},
		Spec: workspacev1alpha1.WorkspaceIntegrationTemplateSpec{
			DisplayName: "Ray Cluster Integration",
			StatusProbe: &workspacev1alpha1.IntegrationStatusProbe{
				Exec:          &corev1.ExecAction{Command: []string{"ray", "status"}},
				PeriodSeconds: 30,
			},
		},
	}

	wi := newTestWI()
	require.NoError(t, BuildWorkspaceIntegration(context.Background(), c, wi, template))
	require.NotNil(t, wi.Spec.StatusProbe, "statusProbe must be frozen onto the resolved spec")
	require.NotNil(t, wi.Spec.StatusProbe.Exec)
	assert.Equal(t, []string{"ray", "status"}, wi.Spec.StatusProbe.Exec.Command)
	assert.Equal(t, int32(30), wi.Spec.StatusProbe.PeriodSeconds)

	// Deep copy, not aliasing: mutating the built probe must not affect the template.
	wi.Spec.StatusProbe.PeriodSeconds = 99
	assert.Equal(t, int32(30), template.Spec.StatusProbe.PeriodSeconds, "built probe must be a deep copy of the template's")
}

// TestBuildWorkspaceIntegration_FreezesShareProcessNamespace verifies the template's pod-level
// shareProcessNamespace toggle is carried onto the resolved spec.
func TestBuildWorkspaceIntegration_FreezesShareProcessNamespace(t *testing.T) {
	c := fakeClientWithObjects(t, nil)
	enabled := true
	template := &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ray-integration", Namespace: "user-ns"},
		Spec: workspacev1alpha1.WorkspaceIntegrationTemplateSpec{
			DisplayName:           "Ray Cluster Integration",
			ShareProcessNamespace: &enabled,
		},
	}

	wi := newTestWI()
	require.NoError(t, BuildWorkspaceIntegration(context.Background(), c, wi, template))
	require.NotNil(t, wi.Spec.ShareProcessNamespace)
	assert.True(t, *wi.Spec.ShareProcessNamespace)
}

// TestBuildWorkspaceIntegration_WorkspaceRefNamespaceDefaultsToWI verifies the workspace namespace
// used for resolution context defaults to the WorkspaceIntegration's own namespace when
// workspaceRef.namespace is empty, and that a resourceRef with no namespace is fetched there.
func TestBuildWorkspaceIntegration_WorkspaceRefNamespaceDefaultsToWI(t *testing.T) {
	rayGVK := schema.GroupVersionKind{Group: "ray.io", Version: "v1", Kind: testKindRayCluster}
	rayCluster := newUnstructured("ray.io/v1", testKindRayCluster, "my-ray-cluster", "wi-ns",
		map[string]interface{}{"spec": map[string]interface{}{"foo": "bar"}})
	c := fakeClientWithObjects(t, []schema.GroupVersionKind{rayGVK}, rayCluster)

	wi := newTestWI()
	wi.Namespace = "wi-ns"
	wi.Spec.WorkspaceRef.Namespace = "" // force defaulting to the WI's own namespace

	template := &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ray-integration", Namespace: "wi-ns"},
		Spec: workspacev1alpha1.WorkspaceIntegrationTemplateSpec{
			DisplayName: "Ray Cluster Integration",
			ResourceRefs: []workspacev1alpha1.ResourceRef{
				{ID: testRefIDRayCluster, APIVersion: "ray.io/v1", Kind: testKindRayCluster, Name: "{{ .Parameters.clusterName }}"},
			},
			DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
				PodModifications: &workspacev1alpha1.PodModifications{
					PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
						MergeEnv: []workspacev1alpha1.AccessEnvTemplate{
							{Name: "RAY_CLUSTER", ValueTemplate: "{{ .Parameters.clusterName }}"},
						},
					},
				},
			},
		},
	}

	require.NoError(t, BuildWorkspaceIntegration(context.Background(), c, wi, template))
	env := firstMergeEnv(wi)
	require.Len(t, env, 1)
	assert.Equal(t, "my-ray-cluster", env[0].ValueTemplate)
}

// TestBuildWorkspaceIntegration_NotFoundFailsClosed verifies the build fails (so admission rejects)
// when the referenced resource does not exist, naming the failing ref, and leaves the wi spec
// output fields untouched.
func TestBuildWorkspaceIntegration_NotFoundFailsClosed(t *testing.T) {
	rayGVK := schema.GroupVersionKind{Group: "ray.io", Version: "v1", Kind: testKindRayCluster}
	c := fakeClientWithObjects(t, []schema.GroupVersionKind{rayGVK}) // nothing seeded

	template := &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ray-integration", Namespace: "user-ns"},
		Spec: workspacev1alpha1.WorkspaceIntegrationTemplateSpec{
			DisplayName: "Ray Cluster Integration",
			ResourceRefs: []workspacev1alpha1.ResourceRef{
				{ID: testRefIDRayCluster, APIVersion: "ray.io/v1", Kind: testKindRayCluster, Name: "{{ .Parameters.clusterName }}"},
			},
			DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
				PodModifications: &workspacev1alpha1.PodModifications{
					AdditionalContainers: []corev1.Container{{Name: "ray-sidecar", Image: "ray:2.9"}},
				},
			},
		},
	}

	wi := newTestWI()
	err := BuildWorkspaceIntegration(context.Background(), c, wi, template)
	require.Error(t, err)
	assert.Nil(t, wi.Spec.DeploymentModifications, "failed build must leave the output spec untouched")
	assert.Contains(t, err.Error(), `resourceRef "rayCluster"`)
	assert.Contains(t, err.Error(), "not found")
}

// TestBuildWorkspaceIntegration_NoModifications verifies a template with no deployment
// modifications resolves cleanly, leaving DeploymentModifications nil.
func TestBuildWorkspaceIntegration_NoModifications(t *testing.T) {
	c := fakeClientWithObjects(t, nil)
	template := &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ray-integration", Namespace: "user-ns"},
		Spec:       workspacev1alpha1.WorkspaceIntegrationTemplateSpec{DisplayName: "Ray Cluster Integration"},
	}

	wi := newTestWI()
	require.NoError(t, BuildWorkspaceIntegration(context.Background(), c, wi, template))
	assert.Nil(t, wi.Spec.DeploymentModifications)
	assert.Empty(t, firstAdditionalContainers(wi))
	assert.Empty(t, firstMergeEnv(wi))
}

// TestBuildWorkspaceIntegration_RebuildClearsStaleOutput locks in the unconditional-assignment
// contract: when a WorkspaceIntegration carrying output from a prior build is rebuilt against a
// template that no longer specifies those fields (e.g. after switching to a bare template), every
// output field is cleared rather than retaining a stale value.
func TestBuildWorkspaceIntegration_RebuildClearsStaleOutput(t *testing.T) {
	c := fakeClientWithObjects(t, nil)

	// A WI as if a prior build had already frozen output onto it.
	enabled := true
	wi := newTestWI()
	wi.Spec.ShareProcessNamespace = &enabled
	wi.Spec.StatusProbe = &workspacev1alpha1.IntegrationStatusProbe{
		Exec:          &corev1.ExecAction{Command: []string{"ray", "status"}},
		PeriodSeconds: 30,
	}
	wi.Spec.DeploymentModifications = &workspacev1alpha1.DeploymentModifications{
		PodModifications: &workspacev1alpha1.PodModifications{
			AdditionalContainers: []corev1.Container{{Name: "stale-sidecar", Image: "ray:2.8"}},
		},
	}

	// Rebuild against a template with no modifications, probe, or shareProcessNamespace.
	template := &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ray-integration", Namespace: "user-ns"},
		Spec:       workspacev1alpha1.WorkspaceIntegrationTemplateSpec{DisplayName: "Ray Cluster Integration"},
	}

	require.NoError(t, BuildWorkspaceIntegration(context.Background(), c, wi, template))
	assert.Nil(t, wi.Spec.ShareProcessNamespace, "stale shareProcessNamespace must be cleared on rebuild")
	assert.Nil(t, wi.Spec.StatusProbe, "stale statusProbe must be cleared on rebuild")
	assert.Nil(t, wi.Spec.DeploymentModifications, "stale deploymentModifications must be cleared on rebuild")
}

// TestBuildWorkspaceIntegration_NilGuards verifies the builder rejects nil inputs rather than
// panicking.
func TestBuildWorkspaceIntegration_NilGuards(t *testing.T) {
	c := fakeClientWithObjects(t, nil)

	err := BuildWorkspaceIntegration(context.Background(), c, nil, &workspacev1alpha1.WorkspaceIntegrationTemplate{})
	require.Error(t, err)

	err = BuildWorkspaceIntegration(context.Background(), c, newTestWI(), nil)
	require.Error(t, err)
}

// TestBuildWorkspaceIntegration_RejectsCrossNamespaceWorkspaceRef verifies the builder rejects a
// workspaceRef.namespace that differs from the WorkspaceIntegration's own namespace (cross-namespace
// references are unsupported), while allowing an empty namespace (defaults to own) and an explicit
// matching namespace.
func TestBuildWorkspaceIntegration_RejectsCrossNamespaceWorkspaceRef(t *testing.T) {
	c := fakeClientWithObjects(t, nil)
	template := &workspacev1alpha1.WorkspaceIntegrationTemplate{
		ObjectMeta: metav1.ObjectMeta{Name: "ray-integration", Namespace: "user-ns"},
		Spec:       workspacev1alpha1.WorkspaceIntegrationTemplateSpec{DisplayName: "Ray Cluster Integration"},
	}

	// Differing namespace -> rejected.
	wi := newTestWI() // wi.Namespace == "user-ns"
	wi.Spec.WorkspaceRef.Namespace = "other-ns"
	err := BuildWorkspaceIntegration(context.Background(), c, wi, template)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cross-namespace references are not supported")
	assert.Nil(t, wi.Spec.DeploymentModifications, "rejected build must not mutate output fields")

	// Explicit matching namespace -> allowed.
	wiMatch := newTestWI()
	wiMatch.Spec.WorkspaceRef.Namespace = wiMatch.Namespace
	require.NoError(t, BuildWorkspaceIntegration(context.Background(), c, wiMatch, template))

	// Empty namespace -> allowed (defaults to own).
	wiEmpty := newTestWI()
	wiEmpty.Spec.WorkspaceRef.Namespace = ""
	require.NoError(t, BuildWorkspaceIntegration(context.Background(), c, wiEmpty, template))
}
