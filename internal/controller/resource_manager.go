package controller

import (
	"context"
	"fmt"

	"github.com/jupyter-k8s/jupyter-k8s/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ResourceManager handles CRUD operations for Kubernetes resources
type ResourceManager struct {
	client            client.Client
	deploymentBuilder *DeploymentBuilder
	serviceBuilder    *ServiceBuilder
	statusManager     *StatusManager
}

// NewResourceManager creates a new ResourceManager
func NewResourceManager(client client.Client, deploymentBuilder *DeploymentBuilder, serviceBuilder *ServiceBuilder, statusManager *StatusManager) *ResourceManager {
	return &ResourceManager{
		client:            client,
		deploymentBuilder: deploymentBuilder,
		serviceBuilder:    serviceBuilder,
		statusManager:     statusManager,
	}
}

// GetDeployment retrieves the deployment for a JupyterServer
func (rm *ResourceManager) GetDeployment(ctx context.Context, jupyterServer *v1alpha1.JupyterServer) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	deploymentName := GenerateDeploymentName(jupyterServer.Name)

	err := rm.client.Get(ctx, types.NamespacedName{
		Name:      deploymentName,
		Namespace: jupyterServer.Namespace,
	}, deployment)

	return deployment, err
}

// GetService retrieves the service for a JupyterServer
func (rm *ResourceManager) GetService(ctx context.Context, jupyterServer *v1alpha1.JupyterServer) (*corev1.Service, error) {
	service := &corev1.Service{}
	serviceName := GenerateServiceName(jupyterServer.Name)

	err := rm.client.Get(ctx, types.NamespacedName{
		Name:      serviceName,
		Namespace: jupyterServer.Namespace,
	}, service)

	return service, err
}

// CreateDeployment creates a new deployment for the JupyterServer
func (rm *ResourceManager) CreateDeployment(ctx context.Context, jupyterServer *v1alpha1.JupyterServer) (*appsv1.Deployment, error) {
	logger := log.FromContext(ctx)

	deployment, err := rm.deploymentBuilder.BuildDeployment(jupyterServer)
	if err != nil {
		return nil, fmt.Errorf("failed to build deployment: %w", err)
	}

	logger.Info("Creating Deployment",
		"deployment", deployment.Name,
		"namespace", deployment.Namespace)

	if err := rm.client.Create(ctx, deployment); err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	logger.Info("Successfully created Deployment",
		"deployment", deployment.Name)

	return deployment, nil
}

// CreateService creates a new service for the JupyterServer
func (rm *ResourceManager) CreateService(ctx context.Context, jupyterServer *v1alpha1.JupyterServer) (*corev1.Service, error) {
	logger := log.FromContext(ctx)

	service, err := rm.serviceBuilder.BuildService(jupyterServer)
	if err != nil {
		return nil, fmt.Errorf("failed to build service: %w", err)
	}

	logger.Info("Creating Service",
		"service", service.Name,
		"namespace", service.Namespace)

	if err := rm.client.Create(ctx, service); err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	logger.Info("Successfully created Service",
		"service", service.Name)

	return service, nil
}

// DeleteDeployment deletes the deployment for a JupyterServer
func (rm *ResourceManager) DeleteDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	logger := log.FromContext(ctx)

	logger.Info("Deleting Deployment",
		"deployment", deployment.Name,
		"namespace", deployment.Namespace)

	if err := rm.client.Delete(ctx, deployment); err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	logger.Info("Successfully deleted Deployment",
		"deployment", deployment.Name)

	return nil
}

// DeleteService deletes the service for a JupyterServer
func (rm *ResourceManager) DeleteService(ctx context.Context, service *corev1.Service) error {
	logger := log.FromContext(ctx)

	logger.Info("Deleting Service",
		"service", service.Name,
		"namespace", service.Namespace)

	if err := rm.client.Delete(ctx, service); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	logger.Info("Successfully deleted Service",
		"service", service.Name)

	return nil
}

// EnsureDeploymentExists creates a deployment if it doesn't exist
func (rm *ResourceManager) EnsureDeploymentExists(ctx context.Context, jupyterServer *v1alpha1.JupyterServer) (*appsv1.Deployment, error) {
	deployment, err := rm.GetDeployment(ctx, jupyterServer)
	if err != nil {
		if errors.IsNotFound(err) {
			return rm.CreateDeployment(ctx, jupyterServer)
		}
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	return deployment, nil
}

// EnsureServiceExists creates a service if it doesn't exist
func (rm *ResourceManager) EnsureServiceExists(ctx context.Context, jupyterServer *v1alpha1.JupyterServer) (*corev1.Service, error) {
	service, err := rm.GetService(ctx, jupyterServer)
	if err != nil {
		if errors.IsNotFound(err) {
			return rm.CreateService(ctx, jupyterServer)
		}
		return nil, fmt.Errorf("failed to get service: %w", err)
	}
	return service, nil
}
