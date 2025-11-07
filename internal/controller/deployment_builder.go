package controller

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

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
func (db *DeploymentBuilder) BuildDeployment(ctx context.Context, workspace *workspacev1alpha1.Workspace, resolvedTemplate *ResolvedTemplate) (*appsv1.Deployment, error) {
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

// BuildDeploymentWithAccessStrategy creates a Deployment resource with access strategy applied
// This is a helper function that should be called from ResourceManager
func (db *DeploymentBuilder) BuildDeploymentWithAccessStrategy(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	resolvedTemplate *ResolvedTemplate,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) (*appsv1.Deployment, error) {
	// Build the base deployment
	deployment, err := db.BuildDeployment(ctx, workspace, resolvedTemplate)
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
func (db *DeploymentBuilder) buildDeploymentSpec(workspace *workspacev1alpha1.Workspace, resolvedTemplate *ResolvedTemplate, resources corev1.ResourceRequirements) appsv1.DeploymentSpec {
	// Single replica for Jupyter workspaces (stateful, user-specific workloads)
	replicas := int32(1)

	return appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: GenerateLabels(workspace.Name),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: db.buildPodLabels(workspace),
			},
			Spec: db.buildPodSpec(workspace, resolvedTemplate, resources),
		},
	}
}

// buildPodSpec creates the pod specification
func (db *DeploymentBuilder) buildPodSpec(workspace *workspacev1alpha1.Workspace, resolvedTemplate *ResolvedTemplate, resources corev1.ResourceRequirements) corev1.PodSpec {
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{
			db.buildPrimaryContainer(workspace, resolvedTemplate, resources),
		},
	}

	storageConfig := ResolveStorageConfig(workspace, resolvedTemplate)
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

	// Set node selector - workspace spec takes precedence over template
	if len(workspace.Spec.NodeSelector) > 0 {
		podSpec.NodeSelector = workspace.Spec.NodeSelector
	} else if resolvedTemplate != nil && len(resolvedTemplate.NodeSelector) > 0 {
		podSpec.NodeSelector = resolvedTemplate.NodeSelector
	}

	// Set affinity - workspace spec takes precedence over template
	if workspace.Spec.Affinity != nil {
		podSpec.Affinity = workspace.Spec.Affinity
	} else if resolvedTemplate != nil && resolvedTemplate.Affinity != nil {
		podSpec.Affinity = resolvedTemplate.Affinity
	}

	// Set tolerations - workspace spec takes precedence over template
	if len(workspace.Spec.Tolerations) > 0 {
		podSpec.Tolerations = workspace.Spec.Tolerations
	} else if resolvedTemplate != nil && len(resolvedTemplate.Tolerations) > 0 {
		podSpec.Tolerations = resolvedTemplate.Tolerations
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
func (db *DeploymentBuilder) buildPrimaryContainer(workspace *workspacev1alpha1.Workspace, resolvedTemplate *ResolvedTemplate, resources corev1.ResourceRequirements) corev1.Container {
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
		Command:         ResolveContainerCommand(workspace, resolvedTemplate),
		Args:            ResolveContainerArgs(workspace, resolvedTemplate),
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

	storageConfig := ResolveStorageConfig(workspace, resolvedTemplate)
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
func (db *DeploymentBuilder) parseResourceRequirements(workspace *workspacev1alpha1.Workspace, resolvedTemplate *ResolvedTemplate) corev1.ResourceRequirements {
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

// NeedsUpdate checks if the existing deployment needs to be updated based on workspace changes
func (db *DeploymentBuilder) NeedsUpdate(
	ctx context.Context,
	existingDeployment *appsv1.Deployment,
	workspace *workspacev1alpha1.Workspace,
	resolvedTemplate *ResolvedTemplate,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) (bool, error) {
	// Build the desired deployment spec with access strategy applied
	desiredDeployment, err := db.BuildDeploymentWithAccessStrategy(ctx, workspace, resolvedTemplate, accessStrategy)
	if err != nil {
		return false, fmt.Errorf("failed to build desired deployment: %w", err)
	}

	// Compare pod template specs using semantic equality
	return !equality.Semantic.DeepEqual(existingDeployment.Spec.Template.Spec, desiredDeployment.Spec.Template.Spec), nil
}
