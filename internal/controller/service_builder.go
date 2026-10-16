/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ServiceBuilder handles creation of Service resources for Workspace
type ServiceBuilder struct {
	scheme *runtime.Scheme
}

// NewServiceBuilder creates a new ServiceBuilder
func NewServiceBuilder(scheme *runtime.Scheme) *ServiceBuilder {
	return &ServiceBuilder{
		scheme: scheme,
	}
}

// BuildService creates a Service resource for the given Workspace.
//
// Alongside the application port, the Service publishes the sidecar container ports the access
// strategy opts into via podModifications.exposedPorts; a Service only forwards traffic on ports
// it explicitly declares. accessStrategy may be nil, in which case only the application port is
// exposed.
func (sb *ServiceBuilder) BuildService(
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) (*corev1.Service, error) {
	spec, err := buildServiceSpec(workspace, accessStrategy)
	if err != nil {
		return nil, err
	}

	service := &corev1.Service{
		ObjectMeta: sb.buildObjectMeta(workspace),
		Spec:       spec,
	}

	// Set owner reference for garbage collection
	if err := controllerutil.SetControllerReference(workspace, service, sb.scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return service, nil
}

// buildObjectMeta creates the metadata for the Service
func (sb *ServiceBuilder) buildObjectMeta(workspace *workspacev1alpha1.Workspace) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      GenerateServiceName(workspace.Name),
		Namespace: workspace.Namespace,
		Labels:    GenerateLabels(workspace.Name),
	}
}

// buildServiceSpec creates the service specification
func buildServiceSpec(
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) (corev1.ServiceSpec, error) {
	sidecarPorts, err := sidecarServicePorts(accessStrategy)
	if err != nil {
		return corev1.ServiceSpec{}, err
	}

	ports := make([]corev1.ServicePort, 0, 1+len(sidecarPorts))
	ports = append(ports, corev1.ServicePort{
		Name:       httpScheme,
		Port:       JupyterPort,
		TargetPort: intstr.FromInt(JupyterPort),
		Protocol:   corev1.ProtocolTCP,
	})
	ports = append(ports, sidecarPorts...)

	return corev1.ServiceSpec{
		Type:     corev1.ServiceTypeClusterIP,
		Selector: GenerateLabels(workspace.Name),
		Ports:    ports,
	}, nil
}

// sidecarServicePorts resolves the Service ports the access strategy opts into via
// podModifications.exposedPorts. Each entry names a container port to publish; a containerPort is
// informational and does not by itself grant reachability, so exposure is explicit rather than
// derived from every port a sidecar happens to declare.
//
// Every listed name must resolve to exactly one sidecar container port. An unknown name, a name
// declared by more than one container, a name that shadows the application port, and two exposed
// ports colliding on number are all rejected: the resulting Service would be wrong or ambiguous,
// so surfacing it as an error beats silently publishing something unintended.
func sidecarServicePorts(accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) ([]corev1.ServicePort, error) {
	if accessStrategy == nil ||
		accessStrategy.Spec.DeploymentModifications == nil ||
		accessStrategy.Spec.DeploymentModifications.PodModifications == nil {
		return nil, nil
	}

	podMods := accessStrategy.Spec.DeploymentModifications.PodModifications
	if len(podMods.ExposedPorts) == 0 {
		return nil, nil
	}

	ports := make([]corev1.ServicePort, 0, len(podMods.ExposedPorts))
	usedNumbers := map[int32]string{}

	for _, name := range podMods.ExposedPorts {
		if name == httpScheme {
			return nil, fmt.Errorf("exposed port %q collides with the reserved application port name", name)
		}

		containerPort, err := resolveContainerPort(podMods.AdditionalContainers, name)
		if err != nil {
			return nil, err
		}

		if containerPort.ContainerPort == JupyterPort {
			return nil, fmt.Errorf("exposed port %q targets port %d, reserved for the application", name, JupyterPort)
		}
		if other, taken := usedNumbers[containerPort.ContainerPort]; taken {
			return nil, fmt.Errorf("exposed ports %q and %q both target port %d", other, name, containerPort.ContainerPort)
		}
		usedNumbers[containerPort.ContainerPort] = name

		protocol := containerPort.Protocol
		if protocol == "" {
			protocol = corev1.ProtocolTCP
		}

		ports = append(ports, corev1.ServicePort{
			Name:       name,
			Port:       containerPort.ContainerPort,
			TargetPort: intstr.FromInt32(containerPort.ContainerPort),
			Protocol:   protocol,
		})
	}

	return ports, nil
}

// resolveContainerPort finds the single sidecar container port named name. A name that matches no
// declared port cannot be exposed; a name declared by more than one container is ambiguous —
// nothing says which the strategy meant — so both are rejected rather than guessed. Only exposed
// names are resolved, so an unrelated name collision among ports nobody exposes is left alone.
func resolveContainerPort(containers []corev1.Container, name string) (corev1.ContainerPort, error) {
	var match corev1.ContainerPort
	found := false

	for _, container := range containers {
		for _, containerPort := range container.Ports {
			if containerPort.Name != name {
				continue
			}
			if found {
				return corev1.ContainerPort{}, fmt.Errorf("exposed port %q is declared by more than one additional container", name)
			}
			match = containerPort
			found = true
		}
	}

	if !found {
		return corev1.ContainerPort{}, fmt.Errorf("exposed port %q matches no port declared by any additional container", name)
	}

	return match, nil
}

// NeedsUpdate checks if the existing service needs to be updated based on workspace changes.
//
// Only the fields the operator owns are compared. The rest of a live ServiceSpec is populated
// by the API server (ClusterIP, ClusterIPs, IPFamilies, IPFamilyPolicy, SessionAffinity,
// InternalTrafficPolicy), and a freshly built desired spec leaves all of those empty —
// comparing whole specs would therefore always report a difference.
func (sb *ServiceBuilder) NeedsUpdate(
	ctx context.Context,
	existingService *corev1.Service,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) (bool, error) {
	// Build the desired service spec
	desiredService, err := sb.BuildService(workspace, accessStrategy)
	if err != nil {
		return false, fmt.Errorf("failed to build desired service: %w", err)
	}

	return !serviceSpecMatches(&existingService.Spec, &desiredService.Spec), nil
}

// UpdateServiceSpec updates the existing service with the desired spec.
//
// Only the operator-owned fields are assigned. Overwriting the whole spec would clear
// server-populated immutable fields such as ClusterIP, which the API server then rejects.
func (sb *ServiceBuilder) UpdateServiceSpec(
	ctx context.Context,
	existingService *corev1.Service,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
) error {
	// Build the desired service spec
	desiredService, err := sb.BuildService(workspace, accessStrategy)
	if err != nil {
		return fmt.Errorf("failed to build desired service: %w", err)
	}

	applyServiceSpec(&existingService.Spec, &desiredService.Spec)

	return nil
}

// serviceSpecMatches reports whether the operator-owned fields of existing already match desired.
func serviceSpecMatches(existing, desired *corev1.ServiceSpec) bool {
	return existing.Type == desired.Type &&
		equality.Semantic.DeepEqual(existing.Selector, desired.Selector) &&
		equality.Semantic.DeepEqual(existing.Ports, desired.Ports)
}

// applyServiceSpec copies the operator-owned fields of desired onto existing,
// leaving every server-populated field untouched.
func applyServiceSpec(existing, desired *corev1.ServiceSpec) {
	existing.Type = desired.Type
	existing.Selector = desired.Selector
	existing.Ports = desired.Ports
}
