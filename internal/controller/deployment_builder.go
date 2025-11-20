/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package controller

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DeploymentBuilder handles creation of Deployment resources for Workspace
type DeploymentBuilder struct {
	scheme        *runtime.Scheme
	options       WorkspaceControllerOptions
	imageResolver *ImageResolver
}

// NewDeploymentBuilder creates a new DeploymentBuilder
func NewDeploymentBuilder(scheme *runtime.Scheme, options WorkspaceControllerOptions, k8sClient client.Client) *DeploymentBuilder {
	return &DeploymentBuilder{
		scheme:        scheme,
		options:       options,
		imageResolver: NewImageResolver(options.ApplicationImagesRegistry),
	}
}

// BuildDeployment creates a Deployment resource for the given Workspace
func (db *DeploymentBuilder) BuildDeployment(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*appsv1.Deployment, error) {
	resources := db.parseResourceRequirements(workspace)

	deployment := &appsv1.Deployment{
		ObjectMeta: db.buildObjectMeta(workspace),
		Spec:       db.buildDeploymentSpec(workspace, resources),
	}

	if err := controllerutil.SetControllerReference(workspace, deployment, db.scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return deployment, nil
}

// BuildDeploymentWithAccessStrategy creates a Deployment resource with access strategy applied
// This is a helper function that should be called from ResourceManager
func (db *DeploymentBuilder) BuildDeploymentWithAccessStrategy(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) (*appsv1.Deployment, error) {
	// Build the base deployment
	deployment, err := db.BuildDeployment(ctx, workspace)
	if err != nil {
		return nil, err
	}

	if accessStrategy != nil {
		if err := db.ApplyAccessStrategyToDeployment(deployment, workspace, accessStrategy); err != nil {
			return nil, fmt.Errorf("failed to apply access strategy to deployment: %w", err)
		}
	}

	return deployment, nil
}

// buildObjectMeta creates the metadata for the Deployment
func (db *DeploymentBuilder) buildObjectMeta(workspace *workspacev1alpha1.Workspace) metav1.ObjectMeta {
	labels := GenerateLabels(workspace.Name)

	// Copy all workspace labels to deployment
	if workspace.Labels != nil {
		for key, value := range workspace.Labels {
			labels[key] = value
		}
	}

	return metav1.ObjectMeta{
		Name:      GenerateDeploymentName(workspace.Name),
		Namespace: workspace.Namespace,
		Labels:    labels,
	}
}

// buildPodLabels creates labels for pod template, including workspace labels
func (db *DeploymentBuilder) buildPodLabels(workspace *workspacev1alpha1.Workspace) map[string]string {
	labels := GenerateLabels(workspace.Name)

	// Copy all workspace labels to pod
	if workspace.Labels != nil {
		for key, value := range workspace.Labels {
			labels[key] = value
		}
	}

	return labels
}

// buildDeploymentSpec creates the deployment specification
func (db *DeploymentBuilder) buildDeploymentSpec(workspace *workspacev1alpha1.Workspace, resources corev1.ResourceRequirements) appsv1.DeploymentSpec {
	// Single replica for Jupyter workspaces (stateful, user-specific workloads)
	replicas := int32(1)

	return appsv1.DeploymentSpec{
		Replicas: &replicas,
		Strategy: appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		},
		Selector: &metav1.LabelSelector{
			MatchLabels: GenerateLabels(workspace.Name),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: db.buildPodLabels(workspace),
			},
			Spec: db.buildPodSpec(workspace, resources),
		},
	}
}

// buildPodSpec creates the pod specification
func (db *DeploymentBuilder) buildPodSpec(workspace *workspacev1alpha1.Workspace, resources corev1.ResourceRequirements) corev1.PodSpec {
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{
			db.buildPrimaryContainer(workspace, resources),
		},
	}

	storageConfig := ResolveStorageConfig(workspace)
	if storageConfig != nil {
		podSpec.Volumes = []corev1.Volume{
			{
				Name: "workspace-storage",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: GeneratePVCName(workspace.Name),
					},
				},
			},
		}
	}

	// Add additional volumes from spec
	for _, vol := range workspace.Spec.Volumes {
		if vol.Name == "workspace-storage" {
			// Skip if name conflicts with primary storage
			continue
		}
		podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
			Name: vol.Name,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: vol.PersistentVolumeClaimName,
				},
			},
		})
	}

	// Set scheduling fields from workspace spec
	if len(workspace.Spec.NodeSelector) > 0 {
		podSpec.NodeSelector = workspace.Spec.NodeSelector
	}

	if workspace.Spec.Affinity != nil {
		podSpec.Affinity = workspace.Spec.Affinity
	}

	if len(workspace.Spec.Tolerations) > 0 {
		podSpec.Tolerations = workspace.Spec.Tolerations
	}

	if workspace.Spec.ServiceAccountName != "" {
		podSpec.ServiceAccountName = workspace.Spec.ServiceAccountName
	}

	// Apply pod security context
	if workspace.Spec.PodSecurityContext != nil {
		podSpec.SecurityContext = workspace.Spec.PodSecurityContext
	}

	return podSpec
}

// buildPrimaryContainer creates the container specification
func (db *DeploymentBuilder) buildPrimaryContainer(workspace *workspacev1alpha1.Workspace, resources corev1.ResourceRequirements) corev1.Container {
	image := db.imageResolver.ResolveImage(workspace)

	// Get command and args from workspace spec if specified
	var command []string
	var args []string
	if workspace.Spec.ContainerConfig != nil {
		command = workspace.Spec.ContainerConfig.Command
		args = workspace.Spec.ContainerConfig.Args
	}

	container := corev1.Container{
		Name:            "workspace",
		Image:           image,
		ImagePullPolicy: db.options.ApplicationImagesPullPolicy,
		Command:         command,
		Args:            args,
		Lifecycle:       workspace.Spec.Lifecycle,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: JupyterPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: resources,
		// Default environment variables
		Env: []corev1.EnvVar{},
		// TODO: Add probes
	}

	storageConfig := ResolveStorageConfig(workspace)
	if storageConfig != nil {
		container.VolumeMounts = []corev1.VolumeMount{
			{
				Name:      "workspace-storage",
				MountPath: storageConfig.MountPath,
			},
		}
	}

	// Add additional volume mounts from spec
	for _, vol := range workspace.Spec.Volumes {
		if vol.Name == "workspace-storage" {
			// Skip if name conflicts with primary storage
			continue
		}
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      vol.Name,
			MountPath: vol.MountPath,
		})
	}

	return container
}

// parseResourceRequirements extracts and validates resource requirements
func (db *DeploymentBuilder) parseResourceRequirements(workspace *workspacev1alpha1.Workspace) corev1.ResourceRequirements {
	defaultCPU := resource.MustParse(DefaultCPURequest)
	defaultMemory := resource.MustParse(DefaultMemoryRequest)

	// Use provided resources if available, otherwise use defaults
	if workspace.Spec.Resources != nil {
		result := *workspace.Spec.Resources
		if result.Requests == nil {
			result.Requests = corev1.ResourceList{
				corev1.ResourceCPU:    defaultCPU,
				corev1.ResourceMemory: defaultMemory,
			}
		}

		return result
	}

	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    defaultCPU,
			corev1.ResourceMemory: defaultMemory,
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    defaultCPU,
			corev1.ResourceMemory: defaultMemory,
		},
	}
}

// NeedsUpdate checks if the existing deployment needs to be updated based on workspace changes
func (db *DeploymentBuilder) NeedsUpdate(
	ctx context.Context,
	existingDeployment *appsv1.Deployment,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) (bool, error) {
	// Build the desired deployment spec with access strategy applied
	desiredDeployment, err := db.BuildDeploymentWithAccessStrategy(ctx, workspace, accessStrategy)
	if err != nil {
		return false, fmt.Errorf("failed to build desired deployment: %w", err)
	}

	// Compare pod template specs using semantic equality
	return !equality.Semantic.DeepEqual(existingDeployment.Spec.Template.Spec, desiredDeployment.Spec.Template.Spec), nil
}
