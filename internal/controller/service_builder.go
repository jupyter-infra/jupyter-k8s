package controller

import (
	"fmt"

	"github.com/jupyter-k8s/jupyter-k8s/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ServiceBuilder handles creation of Service resources for JupyterServer
type ServiceBuilder struct {
	scheme *runtime.Scheme
}

// NewServiceBuilder creates a new ServiceBuilder
func NewServiceBuilder(scheme *runtime.Scheme) *ServiceBuilder {
	return &ServiceBuilder{
		scheme: scheme,
	}
}

// BuildService creates a Service resource for the given JupyterServer
func (sb *ServiceBuilder) BuildService(jupyterServer *v1alpha1.JupyterServer) (*corev1.Service, error) {
	service := &corev1.Service{
		ObjectMeta: sb.buildObjectMeta(jupyterServer),
		Spec:       sb.buildServiceSpec(jupyterServer),
	}

	// Set owner reference for garbage collection
	if err := controllerutil.SetControllerReference(jupyterServer, service, sb.scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return service, nil
}

// buildObjectMeta creates the metadata for the Service
func (sb *ServiceBuilder) buildObjectMeta(jupyterServer *v1alpha1.JupyterServer) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:      GenerateServiceName(jupyterServer.Name),
		Namespace: jupyterServer.Namespace,
		Labels:    GenerateLabels(jupyterServer.Name),
	}
}

// buildServiceSpec creates the service specification
func (sb *ServiceBuilder) buildServiceSpec(jupyterServer *v1alpha1.JupyterServer) corev1.ServiceSpec {
	return corev1.ServiceSpec{
		Type:     corev1.ServiceTypeClusterIP,
		Selector: GenerateLabels(jupyterServer.Name),
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
