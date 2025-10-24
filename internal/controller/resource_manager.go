package controller

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// ResourceManager handles CRUD operations for Kubernetes resources
type ResourceManager struct {
	client                 client.Client
	scheme                 *runtime.Scheme
	deploymentBuilder      *DeploymentBuilder
	serviceBuilder         *ServiceBuilder
	pvcBuilder             *PVCBuilder
	accessResourcesBuilder *AccessResourcesBuilder
	statusManager          *StatusManager
}

// NewResourceManager creates a new ResourceManager
func NewResourceManager(
	k8sClient client.Client,
	scheme *runtime.Scheme,
	deploymentBuilder *DeploymentBuilder,
	serviceBuilder *ServiceBuilder,
	pvcBuilder *PVCBuilder,
	accessResourcesBuilder *AccessResourcesBuilder,
	statusManager *StatusManager,
) *ResourceManager {
	return &ResourceManager{
		client:                 k8sClient,
		scheme:                 scheme,
		deploymentBuilder:      deploymentBuilder,
		serviceBuilder:         serviceBuilder,
		pvcBuilder:             pvcBuilder,
		accessResourcesBuilder: accessResourcesBuilder,
		statusManager:          statusManager,
	}
}

// GetDeployment retrieves the deployment for a Workspace
func (rm *ResourceManager) getDeployment(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	deploymentName := GenerateDeploymentName(workspace.Name)

	err := rm.client.Get(ctx, types.NamespacedName{
		Name:      deploymentName,
		Namespace: workspace.Namespace,
	}, deployment)

	return deployment, err
}

// GetService retrieves the service for a Workspace
func (rm *ResourceManager) getService(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.Service, error) {
	service := &corev1.Service{}
	serviceName := GenerateServiceName(workspace.Name)

	err := rm.client.Get(ctx, types.NamespacedName{
		Name:      serviceName,
		Namespace: workspace.Namespace,
	}, service)

	return service, err
}

// getPVC retrieves the PVC for a Workspace
func (rm *ResourceManager) getPVC(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	pvcName := GeneratePVCName(workspace.Name)

	err := rm.client.Get(ctx, types.NamespacedName{
		Name:      pvcName,
		Namespace: workspace.Namespace,
	}, pvc)

	return pvc, err
}

// CreateDeployment creates a new deployment for the Workspace
func (rm *ResourceManager) createDeployment(ctx context.Context, workspace *workspacev1alpha1.Workspace, resolvedTemplate *ResolvedTemplate) (*appsv1.Deployment, error) {
	logger := logf.FromContext(ctx)

	deployment, err := rm.deploymentBuilder.BuildDeployment(ctx, workspace, resolvedTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to build deployment: %w", err)
	}

	// Modify deployment build per AccessStrategy
	accessStrategy, getAccessStrategyErr := rm.GetAccessStrategyForWorkspace(ctx, workspace)
	if getAccessStrategyErr != nil {
		return nil, fmt.Errorf("failed to retrieve access strategy for deployment: %w", getAccessStrategyErr)
	}
	if accessStrategy != nil {
		if err := rm.deploymentBuilder.ApplyAccessStrategyToDeployment(deployment, workspace, accessStrategy); err != nil {
			return nil, fmt.Errorf("failed to apply access strategy to deployment: %w", getAccessStrategyErr)
		}
	}

	// Apply the changes to deployment
	logger.Info("Creating Deployment",
		"deployment", deployment.Name,
		"namespace", deployment.Namespace)
	if err := rm.client.Create(ctx, deployment); err != nil {
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	return deployment, nil
}

// CreateService creates a new service for the Workspace
func (rm *ResourceManager) createService(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.Service, error) {
	logger := logf.FromContext(ctx)

	service, err := rm.serviceBuilder.BuildService(workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to build service: %w", err)
	}

	logger.Info("Creating Service",
		"service", service.Name,
		"namespace", service.Namespace)

	if err := rm.client.Create(ctx, service); err != nil {
		return nil, fmt.Errorf("failed to create service: %w", err)
	}

	return service, nil
}

// createPVC creates a new PVC for the Workspace
func (rm *ResourceManager) createPVC(ctx context.Context, workspace *workspacev1alpha1.Workspace, resolvedTemplate *ResolvedTemplate) (*corev1.PersistentVolumeClaim, error) {
	logger := logf.FromContext(ctx)

	pvc, err := rm.pvcBuilder.BuildPVC(workspace, resolvedTemplate)
	if err != nil {
		return nil, fmt.Errorf("failed to build PVC: %w", err)
	}

	if pvc == nil {
		return nil, nil // No storage requested
	}

	logger.Info("Creating PVC",
		"pvc", pvc.Name,
		"namespace", pvc.Namespace)

	if err := rm.client.Create(ctx, pvc); err != nil {
		return nil, fmt.Errorf("failed to create PVC: %w", err)
	}

	return pvc, nil
}

// DeleteDeployment deletes the deployment for a Workspace
func (rm *ResourceManager) deleteDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	logger := logf.FromContext(ctx)

	logger.Info("Deleting Deployment",
		"deployment", deployment.Name,
		"namespace", deployment.Namespace)

	if err := rm.client.Delete(ctx, deployment); err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	return nil
}

// DeleteService deletes the service for a Workspace
func (rm *ResourceManager) deleteService(ctx context.Context, service *corev1.Service) error {
	logger := logf.FromContext(ctx)

	logger.Info("Deleting Service",
		"service", service.Name,
		"namespace", service.Namespace)

	if err := rm.client.Delete(ctx, service); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	return nil
}

// IsDeploymentAvailable checks if the Deployment is considered available
// based on its status conditions
func (rm *ResourceManager) IsDeploymentAvailable(deployment *appsv1.Deployment) bool {
	// If deployment is nil, it's not available
	if deployment == nil {
		return false
	}

	// Check if the deployment has the Available condition set to True
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentAvailable {
			return condition.Status == corev1.ConditionTrue
		}
	}

	// Fallback: also check the replica counts to determine availability
	// This is useful if the conditions aren't updated yet but replicas are running
	return deployment.Status.AvailableReplicas > 0 &&
		deployment.Status.ReadyReplicas >= *deployment.Spec.Replicas
}

// IsDeploymentMissingOrDeleting checks if the Deployment is either missing (nil)
// or in the process of being deleted
func (rm *ResourceManager) IsDeploymentMissingOrDeleting(deployment *appsv1.Deployment) bool {
	// If deployment is nil, it's missing
	if deployment == nil {
		return true
	}

	// Check if the deployment has a deletion timestamp (is being deleted)
	return !deployment.DeletionTimestamp.IsZero()
}

// IsServiceAvailable checks if the Service has acquired an IP address
func (rm *ResourceManager) IsServiceAvailable(service *corev1.Service) bool {
	// If service is nil, it's not available
	if service == nil {
		return false
	}

	if service.Spec.Type == corev1.ServiceTypeLoadBalancer {
		return len(service.Status.LoadBalancer.Ingress) > 0
	}

	// For any other type of service, assume it is available as soon as it's created
	return true
}

// IsServiceMissingOrDeleting checks if the Service is either missing (nil)
// or in the process of being deleted
func (rm *ResourceManager) IsServiceMissingOrDeleting(service *corev1.Service) bool {
	return service == nil
}

// EnsureDeploymentExists creates a deployment if it doesn't exist
func (rm *ResourceManager) EnsureDeploymentExists(ctx context.Context, workspace *workspacev1alpha1.Workspace, resolvedTemplate *ResolvedTemplate) (*appsv1.Deployment, error) {
	deployment, err := rm.getDeployment(ctx, workspace)
	if err != nil {
		if errors.IsNotFound(err) {
			return rm.createDeployment(ctx, workspace, resolvedTemplate)
		}
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	return deployment, nil
}

// EnsureServiceExists creates a service if it doesn't exist
func (rm *ResourceManager) EnsureServiceExists(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.Service, error) {
	service, err := rm.getService(ctx, workspace)
	if err != nil {
		if errors.IsNotFound(err) {
			return rm.createService(ctx, workspace)
		}
		return nil, fmt.Errorf("failed to get service: %w", err)
	}
	return service, nil
}

// EnsureDeploymentDeleted initiates deletion, or returns the deployment if it is already being deleted
func (rm *ResourceManager) EnsureDeploymentDeleted(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*appsv1.Deployment, error) {
	deployment, err := rm.getDeployment(ctx, workspace)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	if !rm.IsDeploymentMissingOrDeleting(deployment) {
		return deployment, rm.deleteDeployment(ctx, deployment)
	}
	return deployment, nil
}

// EnsureServiceDeleted initiates deletion, or returns the service if it is already being deleted
func (rm *ResourceManager) EnsureServiceDeleted(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.Service, error) {
	service, err := rm.getService(ctx, workspace)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get service: %w", err)
	}
	if !rm.IsServiceMissingOrDeleting(service) {
		return service, rm.deleteService(ctx, service)
	}
	return service, nil
}

// EnsurePVCExists creates a PVC if it doesn't exist
// It uses workspace storage if specified, otherwise falls back to template storage configuration
func (rm *ResourceManager) EnsurePVCExists(ctx context.Context, workspace *workspacev1alpha1.Workspace, resolvedTemplate *ResolvedTemplate) (*corev1.PersistentVolumeClaim, error) {
	// Check if storage is needed from either workspace or template
	hasStorage := workspace.Spec.Storage != nil ||
		(resolvedTemplate != nil && resolvedTemplate.StorageConfiguration != nil)

	if !hasStorage {
		return nil, nil // No storage requested from either source
	}

	pvc, err := rm.getPVC(ctx, workspace)
	if err != nil {
		if errors.IsNotFound(err) {
			return rm.createPVC(ctx, workspace, resolvedTemplate)
		}
		return nil, fmt.Errorf("failed to get PVC: %w", err)
	}
	return pvc, nil
}
