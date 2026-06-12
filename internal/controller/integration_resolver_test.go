/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Shared integration strategy test constants. Declared here (the first integration
// test file) and referenced by the other integration test files in this package.
const (
	testParamClusterName = "clusterName"
	testKindRayCluster   = "RayCluster"
	testEnvRayAddress    = "RAY_ADDRESS"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func testResource() *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"headGroupSpec": map[string]interface{}{
					"template": map[string]interface{}{
						"spec": map[string]interface{}{
							"containers": []interface{}{
								map[string]interface{}{
									"image": "rayproject/ray:2.9.0",
								},
							},
						},
					},
				},
			},
			"status": map[string]interface{}{
				"head": map[string]interface{}{
					"serviceName": "my-cluster-head-svc",
				},
			},
		},
	}
}

func testTemplateData() IntegrationTemplateData {
	return IntegrationTemplateData{
		Workspace: IntegrationWorkspaceData{
			Name:      "my-workspace",
			Namespace: "user-ns",
		},
		Parameters: map[string]string{
			testParamClusterName: "my-ray-cluster",
			"region":             "us-west-2",
		},
	}
}

// ---------------------------------------------------------------------------
// ResolveField — table-driven
// ---------------------------------------------------------------------------

func TestResolveField(t *testing.T) {
	resolver := NewIntegrationResolver()
	resource := testResource()
	data := testTemplateData()

	tests := []struct {
		name        string
		field       string
		data        IntegrationTemplateData
		resource    *unstructured.Unstructured
		expected    string
		expectError bool
		errorSubstr string
	}{
		// Identity / pass-through
		{
			name:     "plain string (no expressions)",
			field:    "plain-string-no-templates",
			data:     data,
			resource: resource,
			expected: "plain-string-no-templates",
		},
		{
			name:     "empty string",
			field:    "",
			data:     data,
			resource: resource,
			expected: "",
		},
		{
			name:     "string with special chars but no expressions",
			field:    "127.0.0.1:6379/path?q=1",
			data:     data,
			resource: resource,
			expected: "127.0.0.1:6379/path?q=1",
		},
		// Workspace expressions
		{
			name:     "workspace name substitution",
			field:    "workspace-{{ .Workspace.Name }}",
			data:     data,
			resource: resource,
			expected: "workspace-my-workspace",
		},
		{
			name:     "workspace namespace substitution",
			field:    "ns={{ .Workspace.Namespace }}",
			data:     data,
			resource: resource,
			expected: "ns=user-ns",
		},
		// Parameter expressions
		{
			name:     "single parameter",
			field:    "cluster-{{ .Parameters.clusterName }}",
			data:     data,
			resource: resource,
			expected: "cluster-my-ray-cluster",
		},
		{
			name:     "multiple parameters",
			field:    "{{ .Parameters.clusterName }}-{{ .Parameters.region }}",
			data:     data,
			resource: resource,
			expected: "my-ray-cluster-us-west-2",
		},
		// Resource expressions
		{
			name:     "resource JSONPath — image",
			field:    `{{ resource "{.spec.headGroupSpec.template.spec.containers[0].image}" }}`,
			data:     data,
			resource: resource,
			expected: "rayproject/ray:2.9.0",
		},
		{
			name:     "resource JSONPath — service name",
			field:    `svc={{ resource "{.status.head.serviceName}" }}`,
			data:     data,
			resource: resource,
			expected: "svc=my-cluster-head-svc",
		},
		// Mixed (all expression types resolved in a single pass)
		{
			name:     "all three expression types in one field",
			field:    `{{ .Workspace.Name }}-{{ .Parameters.clusterName }}-{{ resource "{.status.head.serviceName}" }}`,
			data:     data,
			resource: resource,
			expected: "my-workspace-my-ray-cluster-my-cluster-head-svc",
		},
		{
			name:     "resource expression between workspace expressions",
			field:    `{{ .Workspace.Namespace }}/{{ resource "{.spec.headGroupSpec.template.spec.containers[0].image}" }}/end`,
			data:     data,
			resource: resource,
			expected: "user-ns/rayproject/ray:2.9.0/end",
		},
		// Error cases
		{
			name:        "resource expression without resource object",
			field:       `svc={{ resource "{.status.head.serviceName}" }}`,
			data:        data,
			resource:    nil,
			expectError: true,
			errorSubstr: "no resource available",
		},
		{
			name:        "nonexistent JSONPath",
			field:       `{{ resource "{.status.nonExistent}" }}`,
			data:        data,
			resource:    resource,
			expectError: true,
			errorSubstr: "nonExistent",
		},
		{
			name:        "missing parameter key",
			field:       `{{ .Parameters.doesNotExist }}`,
			data:        data,
			resource:    resource,
			expectError: true,
		},
		{
			name:        "invalid expression syntax",
			field:       "{{ .Workspace.Name }",
			data:        data,
			resource:    resource,
			expectError: true,
			errorSubstr: "invalid template syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolver.ResolveField(tt.field, tt.data, tt.resource)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorSubstr != "" {
					assert.Contains(t, err.Error(), tt.errorSubstr)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ResolveResourceLookup
// ---------------------------------------------------------------------------

func TestResolveResourceLookup(t *testing.T) {
	resolver := NewIntegrationResolver()
	data := testTemplateData()

	tests := []struct {
		name              string
		lookup            *workspacev1alpha1.ResourceLookup
		expectedName      string
		expectedNamespace string
		expectError       bool
		errorSubstr       string
	}{
		{
			name: "resolves name and namespace from expressions",
			lookup: &workspacev1alpha1.ResourceLookup{
				APIVersion: "ray.io/v1",
				Kind:       testKindRayCluster,
				Name:       "{{ .Parameters.clusterName }}",
				Namespace:  "{{ .Workspace.Namespace }}",
			},
			expectedName:      "my-ray-cluster",
			expectedNamespace: "user-ns",
		},
		{
			name: "missing parameter in name → error",
			lookup: &workspacev1alpha1.ResourceLookup{
				APIVersion: "ray.io/v1",
				Kind:       testKindRayCluster,
				Name:       "{{ .Parameters.nonexistent }}",
				Namespace:  "{{ .Workspace.Namespace }}",
			},
			expectError: true,
			errorSubstr: "resourceLookup name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, namespace, err := resolver.ResolveResourceLookup(tt.lookup, data)
			if tt.expectError {
				require.Error(t, err)
				if tt.errorSubstr != "" {
					assert.Contains(t, err.Error(), tt.errorSubstr)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedName, name)
				assert.Equal(t, tt.expectedNamespace, namespace)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ResolvePodModifications
// ---------------------------------------------------------------------------

func TestResolvePodModifications(t *testing.T) {
	resolver := NewIntegrationResolver()
	resource := testResource()
	data := testTemplateData()

	t.Run("resolves container image, args, and env", func(t *testing.T) {
		mods := &workspacev1alpha1.PodModifications{
			AdditionalContainers: []corev1.Container{
				{
					Name:  "sidecar",
					Image: `{{ resource "{.spec.headGroupSpec.template.spec.containers[0].image}" }}`,
					Args:  []string{`ray start --address={{ resource "{.status.head.serviceName}" }}:6379`},
					Env:   []corev1.EnvVar{{Name: "CLUSTER_NAME", Value: "{{ .Parameters.clusterName }}"}},
				},
			},
			PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
				MergeEnv: []workspacev1alpha1.AccessEnvTemplate{
					{Name: testEnvRayAddress, ValueTemplate: `{{ resource "{.status.head.serviceName}" }}:10001`},
				},
			},
		}

		resolved, err := resolver.ResolvePodModifications(mods, data, resource)
		require.NoError(t, err)
		require.NotNil(t, resolved)

		container := resolved.AdditionalContainers[0]
		assert.Equal(t, "rayproject/ray:2.9.0", container.Image)
		assert.Equal(t, "ray start --address=my-cluster-head-svc:6379", container.Args[0])
		assert.Equal(t, "my-ray-cluster", container.Env[0].Value)

		assert.Equal(t, "my-cluster-head-svc:10001",
			resolved.PrimaryContainerModifications.MergeEnv[0].ValueTemplate)
	})

	t.Run("nil input returns nil", func(t *testing.T) {
		resolved, err := resolver.ResolvePodModifications(nil, data, resource)
		require.NoError(t, err)
		assert.Nil(t, resolved)
	})

	t.Run("does not mutate original input", func(t *testing.T) {
		originalImage := `{{ resource "{.spec.headGroupSpec.template.spec.containers[0].image}" }}`
		mods := &workspacev1alpha1.PodModifications{
			AdditionalContainers: []corev1.Container{{Name: "sidecar", Image: originalImage}},
		}

		resolved, err := resolver.ResolvePodModifications(mods, data, resource)
		require.NoError(t, err)
		assert.Equal(t, "rayproject/ray:2.9.0", resolved.AdditionalContainers[0].Image)
		assert.Equal(t, originalImage, mods.AdditionalContainers[0].Image,
			"original input must not be mutated")
	})
}
