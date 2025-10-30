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

// EnsurePVCDeleted initiates PVC deletion (used during workspace deletion, not stop)
func (rm *ResourceManager) EnsurePVCDeleted(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.PersistentVolumeClaim, error) {
	pvc, err := rm.getPVC(ctx, workspace)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil // Already deleted
		}
		return nil, fmt.Errorf("failed to get PVC: %w", err)
	}

	if pvc != nil && pvc.DeletionTimestamp.IsZero() {
		logger := logf.FromContext(ctx)
		logger.Info("Deleting PVC", "pvc", pvc.Name, "namespace", pvc.Namespace)
		return pvc, rm.client.Delete(ctx, pvc)
	}

	return pvc, nil
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

// CleanupAllResources performs comprehensive cleanup of all workspace resources
func (rm *ResourceManager) CleanupAllResources(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	logger := logf.FromContext(ctx)

	// Delete access strategy resources first
	accessError := rm.EnsureAccessResourcesDeleted(ctx, workspace)
	if accessError != nil {
		logger.Error(accessError, "Failed to delete access strategy resources")
		// Continue with other deletions, don't block on access strategy
	}

	// Delete deployment (async operation)
	_, err := rm.EnsureDeploymentDeleted(ctx, workspace)
	if err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	// Delete service (async operation)
	_, err = rm.EnsureServiceDeleted(ctx, workspace)
	if err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	// Delete PVC (async operation - unlike stop, delete removes storage permanently)
	_, err = rm.EnsurePVCDeleted(ctx, workspace)
	if err != nil {
		return fmt.Errorf("failed to delete PVC: %w", err)
	}

	// Check if all resources are fully deleted using helper function
	if rm.AreAllResourcesDeleted(ctx, workspace) {
		logger.Info("All resources successfully deleted")
		return nil
	}

	// Resources still being deleted - requeue to check again
	return fmt.Errorf("resources still being deleted, will retry")
}

// AreAllResourcesDeleted checks if all workspace resources are fully removed (not found)
func (rm *ResourceManager) AreAllResourcesDeleted(ctx context.Context, workspace *workspacev1alpha1.Workspace) bool {
	// Check deployment - must be NotFound (fully deleted)
	_, err := rm.getDeployment(ctx, workspace)
	if err == nil || !errors.IsNotFound(err) {
		return false // Still exists or other error
	}

	// Check service - must be NotFound (fully deleted)
	_, err = rm.getService(ctx, workspace)
	if err == nil || !errors.IsNotFound(err) {
		return false // Still exists or other error
	}

	// Check PVC - must be NotFound (fully deleted)
	_, err = rm.getPVC(ctx, workspace)
	if err == nil || !errors.IsNotFound(err) {
		return false // Still exists or other error
	}

	// Check access resources are deleted
	if !rm.AreAccessResourcesDeleted(workspace) {
		return false
	}

	return true
}
