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

// BuildService creates a Service resource for the given Workspace
func (sb *ServiceBuilder) BuildService(workspace *workspacev1alpha1.Workspace) (*corev1.Service, error) {
	service := &corev1.Service{
		ObjectMeta: sb.buildObjectMeta(workspace),
		Spec:       sb.buildServiceSpec(workspace),
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
func (sb *ServiceBuilder) buildServiceSpec(workspace *workspacev1alpha1.Workspace) corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type:     corev1.ServiceTypeClusterIP,
		Selector: GenerateLabels(workspace.Name),
		Ports: []corev1.ServicePort{
			{
				Name:       "http",
				Port:       JupyterPort,
				TargetPort: intstr.FromInt(JupyterPort),
				Protocol:   corev1.ProtocolTCP,
			},
		},
	}
}

// NeedsUpdate checks if the existing service needs to be updated based on workspace changes
func (sb *ServiceBuilder) NeedsUpdate(ctx context.Context, existingService *corev1.Service, workspace *workspacev1alpha1.Workspace) (bool, error) {
	// Build the desired service spec
	desiredService, err := sb.BuildService(workspace)
	if err != nil {
		return false, fmt.Errorf("failed to build desired service: %w", err)
	}

	// Compare service specs using semantic equality
	return !equality.Semantic.DeepEqual(existingService.Spec, desiredService.Spec), nil
}

// UpdateServiceSpec updates the existing service with the desired spec
func (sb *ServiceBuilder) UpdateServiceSpec(ctx context.Context, existingService *corev1.Service, workspace *workspacev1alpha1.Workspace) error {
	// Build the desired service spec
	desiredService, err := sb.BuildService(workspace)
	if err != nil {
		return fmt.Errorf("failed to build desired service: %w", err)
	}

	// Update the service spec while preserving metadata like resourceVersion
	existingService.Spec = desiredService.Spec

	return nil
}
