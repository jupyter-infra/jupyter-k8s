package controller

import (
	"fmt"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DeploymentBuilder handles creation of Deployment resources for Workspace
type DeploymentBuilder struct {
	scheme        *runtime.Scheme
	options       WorkspaceControllerOptions
	imageResolver *ImageResolver
}

// NewDeploymentBuilder creates a new DeploymentBuilder
func NewDeploymentBuilder(scheme *runtime.Scheme, options WorkspaceControllerOptions) *DeploymentBuilder {
	return &DeploymentBuilder{
		scheme:        scheme,
		options:       options,
		imageResolver: NewImageResolver(options.ApplicationImagesRegistry),
	}
}

// BuildDeployment creates a Deployment resource for the given Workspace
func (db *DeploymentBuilder) BuildDeployment(workspace *workspacesv1alpha1.Workspace) (*appsv1.Deployment, error) {
	resources := db.parseResourceRequirements(workspace)
	deployment := &appsv1.Deployment{
		ObjectMeta: db.buildObjectMeta(workspace),
		Spec:       db.buildDeploymentSpec(workspace, resources),
	}

	// Set owner reference for garbage collection
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
func (db *DeploymentBuilder) buildDeploymentSpec(workspace *workspacesv1alpha1.Workspace, resources corev1.ResourceRequirements) appsv1.DeploymentSpec {
	replicas := int32(1) // TODO: Make this configurable

	return appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: GenerateLabels(workspace.Name),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: GenerateLabels(workspace.Name),
			},
			Spec: db.buildPodSpec(workspace, resources),
		},
	}
}

// buildPodSpec creates the pod specification
func (db *DeploymentBuilder) buildPodSpec(workspace *workspacesv1alpha1.Workspace, resources corev1.ResourceRequirements) corev1.PodSpec {
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{
			db.buildJupyterContainer(workspace, resources),
		},
		// TODO: Add security context, service account, etc.
	}

	// Add volume if storage is configured
	if workspace.Spec.Storage != nil {
		podSpec.Volumes = []corev1.Volume{
			{
				Name: "jupyter-storage",
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: GeneratePVCName(workspace.Name),
					},
				},
			},
		}
	}

	return podSpec
}

// buildJupyterContainer creates the Jupyter container specification
func (db *DeploymentBuilder) buildJupyterContainer(workspace *workspacesv1alpha1.Workspace, resources corev1.ResourceRequirements) corev1.Container {
	// Use the image resolver to get the appropriate image reference
	image := db.imageResolver.ResolveImage(workspace)

	container := corev1.Container{
		Name:            "jupyter",
		Image:           image,
		ImagePullPolicy: db.options.ApplicationImagesPullPolicy,
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

	// Add volume mount if storage is configured
	if workspace.Spec.Storage != nil {
		mountPath := workspace.Spec.Storage.MountPath
		if mountPath == "" {
			mountPath = DefaultMountPath
		}

		container.VolumeMounts = []corev1.VolumeMount{
			{
				Name:      "jupyter-storage",
				MountPath: mountPath,
			},
		}
	}

	return container
}

// parseResourceRequirements extracts and validates resource requirements
func (db *DeploymentBuilder) parseResourceRequirements(workspace *workspacesv1alpha1.Workspace) corev1.ResourceRequirements {
	// Set defaults
	defaultCPU := resource.MustParse(DefaultCPURequest)
	defaultMemory := resource.MustParse(DefaultMemoryRequest)

	// Use provided resources if available, otherwise use defaults
	if workspace.Spec.Resources != nil {
		// Return the provided ResourceRequirements directly
		result := *workspace.Spec.Resources
		// If no requests are specified, set defaults
		if result.Requests == nil {
			result.Requests = corev1.ResourceList{
				corev1.ResourceCPU:    defaultCPU,
				corev1.ResourceMemory: defaultMemory,
			}
		}

		return result
	}

	// Return default resources if none specified
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
