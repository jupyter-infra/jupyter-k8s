package controller

import (
	"fmt"

	serversv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// DeploymentBuilder handles creation of Deployment resources for JupyterServer
type DeploymentBuilder struct {
	scheme *runtime.Scheme
}

// NewDeploymentBuilder creates a new DeploymentBuilder
func NewDeploymentBuilder(scheme *runtime.Scheme) *DeploymentBuilder {
	return &DeploymentBuilder{
		scheme: scheme,
	}
}

// BuildDeployment creates a Deployment resource for the given JupyterServer
func (db *DeploymentBuilder) BuildDeployment(jupyterServer *serversv1alpha1.JupyterServer) (*appsv1.Deployment, error) {
	resources := db.parseResourceRequirements(jupyterServer)
	deployment := &appsv1.Deployment{
		ObjectMeta: db.buildObjectMeta(jupyterServer),
		Spec:       db.buildDeploymentSpec(jupyterServer, resources),
	}

	// Set owner reference for garbage collection
	if err := controllerutil.SetControllerReference(jupyterServer, deployment, db.scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return deployment, nil
}

// buildObjectMeta creates the metadata for the Deployment
func (db *DeploymentBuilder) buildObjectMeta(jupyterServer *serversv1alpha1.JupyterServer) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      GenerateDeploymentName(jupyterServer.Name),
		Namespace: jupyterServer.Namespace,
		Labels:    GenerateLabels(jupyterServer.Name),
	}
}

// buildDeploymentSpec creates the deployment specification
func (db *DeploymentBuilder) buildDeploymentSpec(jupyterServer *serversv1alpha1.JupyterServer, resources corev1.ResourceRequirements) appsv1.DeploymentSpec {
	replicas := int32(1) // TODO: Make this configurable

	return appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: GenerateLabels(jupyterServer.Name),
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: GenerateLabels(jupyterServer.Name),
			},
			Spec: db.buildPodSpec(jupyterServer, resources),
		},
	}
}

// buildPodSpec creates the pod specification
func (db *DeploymentBuilder) buildPodSpec(jupyterServer *serversv1alpha1.JupyterServer, resources corev1.ResourceRequirements) corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{
			db.buildJupyterContainer(jupyterServer, resources),
		},
		// TODO: Add security context, service account, etc.
	}
}

// buildJupyterContainer creates the Jupyter container specification
func (db *DeploymentBuilder) buildJupyterContainer(jupyterServer *serversv1alpha1.JupyterServer, resources corev1.ResourceRequirements) corev1.Container {
	// Resolve image - either use direct image reference or lookup from predefined types
	image := jupyterServer.Spec.Image

	// If this is a built-in image shortcut, resolve to the full image path
	image = GetImagePath(image)

	// If no image is specified, use the default
	if image == "" {
		image = DefaultJupyterImage
	}

	return corev1.Container{
		Name:            "jupyter",
		Image:           image,
		ImagePullPolicy: corev1.PullNever, // Use local images for development
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: JupyterPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Resources: resources,
		// TODO: Add environment variables, volume mounts, probes
	}
}

// parseResourceRequirements extracts and validates resource requirements
func (db *DeploymentBuilder) parseResourceRequirements(jupyterServer *serversv1alpha1.JupyterServer) corev1.ResourceRequirements {
	// Set defaults
	defaultCPU := resource.MustParse(DefaultCPURequest)
	defaultMemory := resource.MustParse(DefaultMemoryRequest)

	// Use provided resources if available, otherwise use defaults
	if jupyterServer.Spec.Resources != nil {
		// Return the provided ResourceRequirements directly
		result := *jupyterServer.Spec.Resources
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
