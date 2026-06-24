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

// Shared test fixtures (testResource, testResources, testTemplateData, and the test* constants)
// live in integration_test_helpers_test.go.

// ---------------------------------------------------------------------------
// ResolveField — table-driven
// ---------------------------------------------------------------------------

func TestResolveField(t *testing.T) {
	resolver := NewIntegrationTemplateResolver()
	resources := testResources()
	data := testTemplateData()

	tests := []struct {
		name        string
		field       string
		data        IntegrationTemplateData
		resources   map[string]*unstructured.Unstructured
		expected    string
		expectError bool
		errorSubstr string
	}{
		// Identity / pass-through
		{
			name:      "plain string (no expressions)",
			field:     "plain-string-no-templates",
			data:      data,
			resources: resources,
			expected:  "plain-string-no-templates",
		},
		{
			name:      "empty string",
			field:     "",
			data:      data,
			resources: resources,
			expected:  "",
		},
		{
			name:      "string with special chars but no expressions",
			field:     "127.0.0.1:6379/path?q=1",
			data:      data,
			resources: resources,
			expected:  "127.0.0.1:6379/path?q=1",
		},
		// Workspace expressions
		{
			name:      "workspace name substitution",
			field:     "workspace-{{ .Workspace.Name }}",
			data:      data,
			resources: resources,
			expected:  "workspace-my-workspace",
		},
		{
			name:      "workspace namespace substitution",
			field:     "ns={{ .Workspace.Namespace }}",
			data:      data,
			resources: resources,
			expected:  "ns=user-ns",
		},
		// Parameter expressions
		{
			name:      "single parameter",
			field:     "cluster-{{ .Parameters.clusterName }}",
			data:      data,
			resources: resources,
			expected:  "cluster-my-ray-cluster",
		},
		{
			name:      "multiple parameters",
			field:     "{{ .Parameters.clusterName }}-{{ .Parameters.region }}",
			data:      data,
			resources: resources,
			expected:  "my-ray-cluster-us-west-2",
		},
		// Resource expressions (now id-addressed)
		{
			name:      "resource JSONPath — image",
			field:     `{{ resource "rayCluster" "{.spec.headGroupSpec.template.spec.containers[0].image}" }}`,
			data:      data,
			resources: resources,
			expected:  "rayproject/ray:2.9.0",
		},
		{
			name:      "resource JSONPath — service name",
			field:     `svc={{ resource "rayCluster" "{.status.head.serviceName}" }}`,
			data:      data,
			resources: resources,
			expected:  "svc=my-cluster-head-svc",
		},
		// Mixed (all expression types resolved in a single pass)
		{
			name:      "all three expression types in one field",
			field:     `{{ .Workspace.Name }}-{{ .Parameters.clusterName }}-{{ resource "rayCluster" "{.status.head.serviceName}" }}`,
			data:      data,
			resources: resources,
			expected:  "my-workspace-my-ray-cluster-my-cluster-head-svc",
		},
		{
			name:      "resource expression between workspace expressions",
			field:     `{{ .Workspace.Namespace }}/{{ resource "rayCluster" "{.spec.headGroupSpec.template.spec.containers[0].image}" }}/end`,
			data:      data,
			resources: resources,
			expected:  "user-ns/rayproject/ray:2.9.0/end",
		},
		// Error cases
		{
			name:        "resource expression with no resources available",
			field:       `svc={{ resource "rayCluster" "{.status.head.serviceName}" }}`,
			data:        data,
			resources:   nil,
			expectError: true,
			errorSubstr: `references id "rayCluster"`,
		},
		{
			name:        "resource expression references unknown id",
			field:       `{{ resource "doesNotExist" "{.status.head.serviceName}" }}`,
			data:        data,
			resources:   resources,
			expectError: true,
			errorSubstr: `references id "doesNotExist"`,
		},
		{
			name:        "nonexistent JSONPath",
			field:       `{{ resource "rayCluster" "{.status.nonExistent}" }}`,
			data:        data,
			resources:   resources,
			expectError: true,
			errorSubstr: "nonExistent",
		},
		{
			name:        "missing parameter key",
			field:       `{{ .Parameters.doesNotExist }}`,
			data:        data,
			resources:   resources,
			expectError: true,
		},
		{
			name:        "invalid expression syntax",
			field:       "{{ .Workspace.Name }",
			data:        data,
			resources:   resources,
			expectError: true,
			errorSubstr: "invalid template syntax",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolver.ResolveField(tt.field, tt.data, tt.resources)
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

// TestResolveFieldMultipleResources verifies a single field can read from more than one
// resourceRef by id — the core capability the list (vs single resourceLookup) unlocks.
func TestResolveFieldMultipleResources(t *testing.T) {
	resolver := NewIntegrationTemplateResolver()
	data := testTemplateData()

	resources := map[string]*unstructured.Unstructured{
		testRefIDRayCluster: testResource(),
		"headSvc": {
			Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"clusterIP": "10.0.0.5",
				},
			},
		},
	}

	field := `{{ resource "rayCluster" "{.status.head.serviceName}" }}@{{ resource "headSvc" "{.spec.clusterIP}" }}`
	result, err := resolver.ResolveField(field, data, resources)
	require.NoError(t, err)
	assert.Equal(t, "my-cluster-head-svc@10.0.0.5", result)
}

// ---------------------------------------------------------------------------
// ResolveResourceRef
// ---------------------------------------------------------------------------

func TestResolveResourceRef(t *testing.T) {
	resolver := NewIntegrationTemplateResolver()
	data := testTemplateData()

	tests := []struct {
		name              string
		ref               *workspacev1alpha1.ResourceRef
		expectedName      string
		expectedNamespace string
		expectError       bool
		errorSubstr       string
	}{
		{
			name: "resolves name and namespace from expressions",
			ref: &workspacev1alpha1.ResourceRef{
				ID:         testRefIDRayCluster,
				APIVersion: "ray.io/v1",
				Kind:       testKindRayCluster,
				Name:       "{{ .Parameters.clusterName }}",
				Namespace:  "{{ .Workspace.Namespace }}",
			},
			expectedName:      "my-ray-cluster",
			expectedNamespace: "user-ns",
		},
		{
			name: "missing parameter in name → error names the ref id",
			ref: &workspacev1alpha1.ResourceRef{
				ID:         testRefIDRayCluster,
				APIVersion: "ray.io/v1",
				Kind:       testKindRayCluster,
				Name:       "{{ .Parameters.nonexistent }}",
				Namespace:  "{{ .Workspace.Namespace }}",
			},
			expectError: true,
			errorSubstr: `resourceRef "rayCluster" name`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, namespace, err := resolver.ResolveResourceRef(tt.ref, data)
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
	resolver := NewIntegrationTemplateResolver()
	resources := testResources()
	data := testTemplateData()

	t.Run("resolves container image, args, and env", func(t *testing.T) {
		mods := &workspacev1alpha1.PodModifications{
			AdditionalContainers: []corev1.Container{
				{
					Name:  "sidecar",
					Image: `{{ resource "rayCluster" "{.spec.headGroupSpec.template.spec.containers[0].image}" }}`,
					Args:  []string{`ray start --address={{ resource "rayCluster" "{.status.head.serviceName}" }}:6379`},
					Env:   []corev1.EnvVar{{Name: "CLUSTER_NAME", Value: "{{ .Parameters.clusterName }}"}},
				},
			},
			PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
				MergeEnv: []workspacev1alpha1.AccessEnvTemplate{
					{Name: testEnvRayAddress, ValueTemplate: `{{ resource "rayCluster" "{.status.head.serviceName}" }}:10001`},
				},
			},
		}

		resolved, err := resolver.ResolvePodModifications(mods, data, resources)
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
		resolved, err := resolver.ResolvePodModifications(nil, data, resources)
		require.NoError(t, err)
		assert.Nil(t, resolved)
	})

	t.Run("does not mutate original input", func(t *testing.T) {
		originalImage := `{{ resource "rayCluster" "{.spec.headGroupSpec.template.spec.containers[0].image}" }}`
		mods := &workspacev1alpha1.PodModifications{
			AdditionalContainers: []corev1.Container{{Name: "sidecar", Image: originalImage}},
		}

		resolved, err := resolver.ResolvePodModifications(mods, data, resources)
		require.NoError(t, err)
		assert.Equal(t, "rayproject/ray:2.9.0", resolved.AdditionalContainers[0].Image)
		assert.Equal(t, originalImage, mods.AdditionalContainers[0].Image,
			"original input must not be mutated")
	})

	t.Run("resolves init container fields", func(t *testing.T) {
		mods := &workspacev1alpha1.PodModifications{
			InitContainers: []corev1.Container{
				{
					Name:    "init-ray",
					Image:   `{{ resource "rayCluster" "{.spec.headGroupSpec.template.spec.containers[0].image}" }}`,
					Command: []string{"/bin/sh", "-c"},
					Args:    []string{`wait-for {{ resource "rayCluster" "{.status.head.serviceName}" }}:6379`},
					Env:     []corev1.EnvVar{{Name: "CLUSTER_NAME", Value: "{{ .Parameters.clusterName }}"}},
				},
			},
		}

		resolved, err := resolver.ResolvePodModifications(mods, data, resources)
		require.NoError(t, err)
		require.NotNil(t, resolved)

		init := resolved.InitContainers[0]
		assert.Equal(t, "rayproject/ray:2.9.0", init.Image)
		assert.Equal(t, "wait-for my-cluster-head-svc:6379", init.Args[0])
		assert.Equal(t, "my-ray-cluster", init.Env[0].Value)
	})

	t.Run("resolves workingDir and volume mount paths", func(t *testing.T) {
		mods := &workspacev1alpha1.PodModifications{
			AdditionalContainers: []corev1.Container{
				{
					Name:       "sidecar",
					WorkingDir: "/home/{{ .Workspace.Name }}",
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:        "data",
							MountPath:   "/mnt/{{ .Parameters.clusterName }}",
							SubPath:     "{{ .Workspace.Namespace }}",
							SubPathExpr: `{{ resource "rayCluster" "{.status.head.serviceName}" }}`,
						},
					},
				},
			},
			PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
				VolumeMounts: []corev1.VolumeMount{
					{Name: "shared", MountPath: "/shared/{{ .Workspace.Name }}"},
				},
			},
		}

		resolved, err := resolver.ResolvePodModifications(mods, data, resources)
		require.NoError(t, err)
		require.NotNil(t, resolved)

		c := resolved.AdditionalContainers[0]
		assert.Equal(t, "/home/my-workspace", c.WorkingDir)
		assert.Equal(t, "/mnt/my-ray-cluster", c.VolumeMounts[0].MountPath)
		assert.Equal(t, "user-ns", c.VolumeMounts[0].SubPath)
		assert.Equal(t, "my-cluster-head-svc", c.VolumeMounts[0].SubPathExpr)

		assert.Equal(t, "/shared/my-workspace",
			resolved.PrimaryContainerModifications.VolumeMounts[0].MountPath)
	})
}
