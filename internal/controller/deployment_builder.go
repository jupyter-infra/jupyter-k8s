package controller

import (
	"context"
	"fmt"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DeploymentBuilder handles creation of Deployment resources for Workspace
type DeploymentBuilder struct {
	scheme           *runtime.Scheme
	options          WorkspaceControllerOptions
	imageResolver    *ImageResolver
	templateResolver *TemplateResolver
}

// NewDeploymentBuilder creates a new DeploymentBuilder
func NewDeploymentBuilder(scheme *runtime.Scheme, options WorkspaceControllerOptions, k8sClient client.Client) *DeploymentBuilder {
	return &DeploymentBuilder{
		scheme:           scheme,
		options:          options,
		imageResolver:    NewImageResolver(options.ApplicationImagesRegistry),
		templateResolver: NewTemplateResolver(k8sClient),
	}
}

// BuildDeployment creates a Deployment resource for the given Workspace
// Note: Template validation should happen in StateMachine before calling this
func (db *DeploymentBuilder) BuildDeployment(ctx context.Context, workspace *workspacesv1alpha1.Workspace, resolvedTemplate *ResolvedTemplate) (*appsv1.Deployment, error) {
	resources := db.parseResourceRequirements(workspace, resolvedTemplate)

	deployment := &appsv1.Deployment{
		ObjectMeta: db.buildObjectMeta(workspace),
		Spec:       db.buildDeploymentSpec(workspace, resolvedTemplate, resources),
	}

	if err := controllerutil.SetControllerReference(workspace, deployment, db.scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return deployment, nil
}

// buildObjectMeta creates the metadata for the Deployment
func (db *DeploymentBuilder) buildObjectMeta(workspace *workspacesv1alpha1.Workspace) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      GenerateDeploymentName(workspace.Name),
		Namespace: workspace.Namespace,
		Labels:    GenerateLabels(workspace.Name),
	}
}

// buildDeploymentSpec creates the deployment specification
func (db *DeploymentBuilder) buildDeploymentSpec(workspace *workspacesv1alpha1.Workspace, resolvedTemplate *ResolvedTemplate, resources corev1.ResourceRequirements) appsv1.DeploymentSpec {
	// Single replica for Jupyter workspaces (stateful, user-specific workloads)
	replicas := int32(1)

	return appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: GenerateLabels(workspace.Name),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: GenerateLabels(workspace.Name),
			},
			Spec: db.buildPodSpec(workspace, resolvedTemplate, resources),
		},
	}
}

// buildPodSpec creates the pod specification
func (db *DeploymentBuilder) buildPodSpec(workspace *workspacesv1alpha1.Workspace, resolvedTemplate *ResolvedTemplate, resources corev1.ResourceRequirements) corev1.PodSpec {
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{
			db.buildPrimaryContainer(workspace, resolvedTemplate, resources),
		},
	}

	storageConfig := db.getStorageConfig(workspace, resolvedTemplate)
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

	// Set node selector if specified
	if len(workspace.Spec.NodeSelector) > 0 {
		podSpec.NodeSelector = workspace.Spec.NodeSelector
	}

	return podSpec
}

// buildPrimaryContainer creates the container specification
func (db *DeploymentBuilder) buildPrimaryContainer(workspace *workspacesv1alpha1.Workspace, resolvedTemplate *ResolvedTemplate, resources corev1.ResourceRequirements) corev1.Container {
	var image string
	if resolvedTemplate != nil && resolvedTemplate.Image != "" {
		image = resolvedTemplate.Image
	} else {
		image = db.imageResolver.ResolveImage(workspace)
	}

	container := corev1.Container{
		Name:            "workspace",
		Image:           image,
		ImagePullPolicy: db.options.ApplicationImagesPullPolicy,
		Command:         db.getContainerCommand(workspace),
		Args:            db.getContainerArgs(workspace),
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

	if resolvedTemplate != nil && len(resolvedTemplate.EnvironmentVariables) > 0 {
		container.Env = resolvedTemplate.EnvironmentVariables
	}

	storageConfig := db.getStorageConfig(workspace, resolvedTemplate)
	if storageConfig != nil {
		container.VolumeMounts = []corev1.VolumeMount{
			{
				Name:      "workspace-storage",
				MountPath: DefaultMountPath,
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

// getContainerCommand returns the command for the container
func (db *DeploymentBuilder) getContainerCommand(workspace *workspacesv1alpha1.Workspace) []string {
	// Use ContainerConfig command if specified
	if workspace.Spec.ContainerConfig != nil && len(workspace.Spec.ContainerConfig.Command) > 0 {
		return workspace.Spec.ContainerConfig.Command
	}
	// Return nil to use Docker ENTRYPOINT
	return nil
}

// getContainerArgs returns the args for the container
func (db *DeploymentBuilder) getContainerArgs(workspace *workspacesv1alpha1.Workspace) []string {
	// Use ContainerConfig args if specified
	if workspace.Spec.ContainerConfig != nil && len(workspace.Spec.ContainerConfig.Args) > 0 {
		return workspace.Spec.ContainerConfig.Args
	}
	// Return nil to use Docker CMD
	return nil
}

// parseResourceRequirements extracts and validates resource requirements
func (db *DeploymentBuilder) parseResourceRequirements(workspace *workspacesv1alpha1.Workspace, resolvedTemplate *ResolvedTemplate) corev1.ResourceRequirements {
	if resolvedTemplate != nil {
		return resolvedTemplate.Resources
	}

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

// getStorageConfig determines storage configuration from workspace or template
func (db *DeploymentBuilder) getStorageConfig(workspace *workspacesv1alpha1.Workspace, resolvedTemplate *ResolvedTemplate) *workspacesv1alpha1.StorageConfig {
	// Workspace storage takes precedence
	if workspace.Spec.Storage != nil {
		return &workspacesv1alpha1.StorageConfig{
			DefaultSize: workspace.Spec.Storage.Size,
		}
	}

	if resolvedTemplate != nil && resolvedTemplate.StorageConfiguration != nil {
		return resolvedTemplate.StorageConfiguration
	}

	return nil
}
