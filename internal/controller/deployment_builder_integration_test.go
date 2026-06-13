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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// ---------------------------------------------------------------------------
// Shared test helpers
//
// The integration strategy test constants (testParamClusterName,
// testKindRayCluster, testEnvRayAddress) are declared in
// integration_resolver_test.go and shared package-wide.
// ---------------------------------------------------------------------------

func baseDeployment() *appsv1.Deployment {
	return &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "workspace", Image: "jupyter/base:latest"},
					},
				},
			},
		},
	}
}

func containerNames(containers []corev1.Container) map[string]bool {
	names := make(map[string]bool, len(containers))
	for _, c := range containers {
		names[c.Name] = true
	}
	return names
}

func volumeNames(volumes []corev1.Volume) map[string]bool {
	names := make(map[string]bool, len(volumes))
	for _, v := range volumes {
		names[v.Name] = true
	}
	return names
}

func envByName(env []corev1.EnvVar) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		m[e.Name] = e.Value
	}
	return m
}

// ---------------------------------------------------------------------------
// Unit tests: mergeIntegrationModifications (merge logic)
// ---------------------------------------------------------------------------

func TestMergeIntegrationModifications(t *testing.T) {
	db := &DeploymentBuilder{}

	t.Run("nil deployment returns error", func(t *testing.T) {
		err := db.mergeIntegrationModifications(nil, &workspacev1alpha1.PodModifications{})
		require.Error(t, err)
	})

	t.Run("nil mods is a no-op", func(t *testing.T) {
		deployment := baseDeployment()
		err := db.mergeIntegrationModifications(deployment, nil)
		require.NoError(t, err)
		assert.Len(t, deployment.Spec.Template.Spec.Containers, 1)
	})

	t.Run("appends additional containers after primary", func(t *testing.T) {
		deployment := baseDeployment()
		mods := &workspacev1alpha1.PodModifications{
			AdditionalContainers: []corev1.Container{{Name: "sidecar", Image: "side:1"}},
		}
		err := db.mergeIntegrationModifications(deployment, mods)
		require.NoError(t, err)
		require.Len(t, deployment.Spec.Template.Spec.Containers, 2)
		assert.Equal(t, "workspace", deployment.Spec.Template.Spec.Containers[0].Name)
		assert.Equal(t, "sidecar", deployment.Spec.Template.Spec.Containers[1].Name)
	})

	t.Run("appends volumes", func(t *testing.T) {
		deployment := baseDeployment()
		mods := &workspacev1alpha1.PodModifications{
			Volumes: []corev1.Volume{{Name: "ray-tmp"}},
		}
		err := db.mergeIntegrationModifications(deployment, mods)
		require.NoError(t, err)
		require.Len(t, deployment.Spec.Template.Spec.Volumes, 1)
		assert.Equal(t, "ray-tmp", deployment.Spec.Template.Spec.Volumes[0].Name)
	})

	t.Run("appends init containers", func(t *testing.T) {
		deployment := baseDeployment()
		mods := &workspacev1alpha1.PodModifications{
			InitContainers: []corev1.Container{{Name: "setup", Image: "init:1"}},
		}
		err := db.mergeIntegrationModifications(deployment, mods)
		require.NoError(t, err)
		require.Len(t, deployment.Spec.Template.Spec.InitContainers, 1)
	})

	t.Run("appends volume mounts to primary container", func(t *testing.T) {
		deployment := baseDeployment()
		mods := &workspacev1alpha1.PodModifications{
			PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
				VolumeMounts: []corev1.VolumeMount{{Name: "ray-tmp", MountPath: "/tmp/ray"}},
			},
		}
		err := db.mergeIntegrationModifications(deployment, mods)
		require.NoError(t, err)
		require.Len(t, deployment.Spec.Template.Spec.Containers[0].VolumeMounts, 1)
	})

	t.Run("no primary container returns error", func(t *testing.T) {
		deployment := &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Containers: []corev1.Container{}},
				},
			},
		}
		mods := &workspacev1alpha1.PodModifications{
			PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
				VolumeMounts: []corev1.VolumeMount{{Name: "x", MountPath: "/x"}},
			},
		}
		err := db.mergeIntegrationModifications(deployment, mods)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no primary container")
	})
}

// ---------------------------------------------------------------------------
// Compound merge scenarios
// ---------------------------------------------------------------------------

func TestDeploymentMergePreservesAllModifications(t *testing.T) {
	db := &DeploymentBuilder{}

	tests := []struct {
		name string
		mods *workspacev1alpha1.PodModifications
	}{
		{
			name: "single sidecar + volume + mount",
			mods: &workspacev1alpha1.PodModifications{
				AdditionalContainers: []corev1.Container{{Name: "ray-sidecar", Image: "ray:2.9"}},
				Volumes:              []corev1.Volume{{Name: "ray-tmp"}},
				PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
					VolumeMounts: []corev1.VolumeMount{{Name: "ray-tmp", MountPath: "/tmp/ray"}},
				},
			},
		},
		{
			name: "multiple sidecars + init + volumes",
			mods: &workspacev1alpha1.PodModifications{
				AdditionalContainers: []corev1.Container{
					{Name: "sidecar-a", Image: "a"}, {Name: "sidecar-b", Image: "b"},
				},
				InitContainers: []corev1.Container{{Name: "init-setup", Image: "init:1"}},
				Volumes:        []corev1.Volume{{Name: "vol-a"}, {Name: "vol-b"}},
				PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
					VolumeMounts: []corev1.VolumeMount{{Name: "vol-a", MountPath: "/a"}, {Name: "vol-b", MountPath: "/b"}},
				},
			},
		},
		{
			name: "empty modifications (no-op)",
			mods: &workspacev1alpha1.PodModifications{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := baseDeployment()
			err := db.mergeIntegrationModifications(deployment, tt.mods)
			require.NoError(t, err)

			gotContainers := containerNames(deployment.Spec.Template.Spec.Containers)
			assert.True(t, gotContainers["workspace"], "primary container preserved")
			for _, c := range tt.mods.AdditionalContainers {
				assert.True(t, gotContainers[c.Name])
			}
			gotInit := containerNames(deployment.Spec.Template.Spec.InitContainers)
			for _, c := range tt.mods.InitContainers {
				assert.True(t, gotInit[c.Name])
			}
			gotVols := volumeNames(deployment.Spec.Template.Spec.Volumes)
			for _, v := range tt.mods.Volumes {
				assert.True(t, gotVols[v.Name])
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Env var merge semantics
// ---------------------------------------------------------------------------

func TestEnvVarMergeSemantics(t *testing.T) {
	db := &DeploymentBuilder{}

	tests := []struct {
		name        string
		existingEnv []corev1.EnvVar
		mergeEnv    []workspacev1alpha1.AccessEnvTemplate
		wantEnv     map[string]string
	}{
		{
			name:        "appends new env vars",
			existingEnv: []corev1.EnvVar{{Name: "EXISTING", Value: "keep"}},
			mergeEnv:    []workspacev1alpha1.AccessEnvTemplate{{Name: "NEW", ValueTemplate: "new-val"}},
			wantEnv:     map[string]string{"EXISTING": "keep", "NEW": "new-val"},
		},
		{
			name:        "overrides existing env var",
			existingEnv: []corev1.EnvVar{{Name: "RAY_ADDRESS", Value: "old"}, {Name: "OTHER", Value: "unchanged"}},
			mergeEnv:    []workspacev1alpha1.AccessEnvTemplate{{Name: "RAY_ADDRESS", ValueTemplate: "auto"}},
			wantEnv:     map[string]string{"RAY_ADDRESS": "auto", "OTHER": "unchanged"},
		},
		{
			name:        "mix of override and append",
			existingEnv: []corev1.EnvVar{{Name: "A", Value: "old-a"}, {Name: "B", Value: "old-b"}},
			mergeEnv:    []workspacev1alpha1.AccessEnvTemplate{{Name: "A", ValueTemplate: "new-a"}, {Name: "C", ValueTemplate: "new-c"}},
			wantEnv:     map[string]string{"A": "new-a", "B": "old-b", "C": "new-c"},
		},
		{
			name:        "empty existing + merge adds all",
			existingEnv: nil,
			mergeEnv:    []workspacev1alpha1.AccessEnvTemplate{{Name: "X", ValueTemplate: "x"}, {Name: "Y", ValueTemplate: "y"}},
			wantEnv:     map[string]string{"X": "x", "Y": "y"},
		},
		{
			name:        "empty merge is no-op",
			existingEnv: []corev1.EnvVar{{Name: "KEEP", Value: "me"}},
			mergeEnv:    nil,
			wantEnv:     map[string]string{"KEEP": "me"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deployment := baseDeployment()
			deployment.Spec.Template.Spec.Containers[0].Env = tt.existingEnv
			mods := &workspacev1alpha1.PodModifications{
				PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{MergeEnv: tt.mergeEnv},
			}
			err := db.mergeIntegrationModifications(deployment, mods)
			require.NoError(t, err)

			resultEnv := envByName(deployment.Spec.Template.Spec.Containers[0].Env)
			for name, want := range tt.wantEnv {
				assert.Equal(t, want, resultEnv[name], "env %q", name)
			}
			assert.Len(t, deployment.Spec.Template.Spec.Containers[0].Env, len(tt.wantEnv))
		})
	}
}

// ---------------------------------------------------------------------------
// Pipeline tests are in resource_manager_integration_test.go
// ---------------------------------------------------------------------------
