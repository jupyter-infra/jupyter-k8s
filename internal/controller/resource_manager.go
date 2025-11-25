/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"

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
func (rm *ResourceManager) createDeployment(ctx context.Context, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (*appsv1.Deployment, error) {
	logger := logf.FromContext(ctx)

	deployment, err := rm.deploymentBuilder.BuildDeploymentWithAccessStrategy(ctx, workspace, accessStrategy)
	if err != nil {
		return nil, fmt.Errorf("failed to build deployment: %w", err)
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
func (rm *ResourceManager) createPVC(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.PersistentVolumeClaim, error) {
	logger := logf.FromContext(ctx)

	pvc, err := rm.pvcBuilder.BuildPVC(workspace)
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

// EnsureDeploymentExists creates a deployment if it doesn't exist, or updates it if the pod spec differs
func (rm *ResourceManager) EnsureDeploymentExists(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (*appsv1.Deployment, error) {
	deployment, err := rm.getDeployment(ctx, workspace)
	if err != nil {
		if errors.IsNotFound(err) {
			return rm.createDeployment(ctx, workspace, accessStrategy)
		}
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}

	return rm.ensureDeploymentUpToDate(ctx, deployment, workspace, accessStrategy)
}

// ensureDeploymentUpToDate checks if deployment needs update and updates it if necessary
func (rm *ResourceManager) ensureDeploymentUpToDate(ctx context.Context, deployment *appsv1.Deployment, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (*appsv1.Deployment, error) {
	// Only perform updates when workspace is available to avoid interfering with creation
	if !rm.statusManager.IsWorkspaceAvailable(workspace) {
		return deployment, nil
	}

	// Use the provided accessStrategy instead of fetching it again
	if accessStrategy == nil && workspace.Spec.AccessStrategy != nil {
		// This should not happen as it should be provided already, but handle it gracefully
		var err error
		accessStrategy, err = rm.GetAccessStrategyForWorkspace(ctx, workspace)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve access strategy for comparison: %w", err)
		}
	}

	needsUpdate, err := rm.deploymentBuilder.NeedsUpdate(ctx, deployment, workspace, accessStrategy)
	if err != nil {
		return nil, fmt.Errorf("failed to check if deployment needs update: %w", err)
	}

	if needsUpdate {
		return rm.updateDeployment(ctx, deployment, workspace, accessStrategy)
	}

	return deployment, nil
}

// updateDeployment updates an existing deployment with new pod spec
func (rm *ResourceManager) updateDeployment(ctx context.Context, deployment *appsv1.Deployment, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (*appsv1.Deployment, error) {
	logger := logf.FromContext(ctx)

	// Use the provided accessStrategy instead of fetching it again
	if accessStrategy == nil && workspace.Spec.AccessStrategy != nil {
		// This should not happen as it should be provided already, but handle it gracefully
		var err error
		accessStrategy, err = rm.GetAccessStrategyForWorkspace(ctx, workspace)
		if err != nil {
			return nil, fmt.Errorf("failed to retrieve access strategy for deployment: %w", err)
		}
	}

	// Update the deployment spec using the builder with access strategy
	updatedDeployment, err := rm.deploymentBuilder.BuildDeploymentWithAccessStrategy(ctx, workspace, accessStrategy)
	if err != nil {
		return nil, fmt.Errorf("failed to build updated deployment: %w", err)
	}

	// Update the existing deployment spec while preserving metadata like resourceVersion
	deployment.Spec = updatedDeployment.Spec

	logger.Info("Updating Deployment",
		"deployment", deployment.Name,
		"namespace", deployment.Namespace)

	if err := rm.client.Update(ctx, deployment); err != nil {
		return nil, fmt.Errorf("failed to update deployment: %w", err)
	}

	return deployment, nil
}

// EnsureServiceExists creates a service if it doesn't exist, or updates it if the spec differs
func (rm *ResourceManager) EnsureServiceExists(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.Service, error) {
	service, err := rm.getService(ctx, workspace)
	if err != nil {
		if errors.IsNotFound(err) {
			return rm.createService(ctx, workspace)
		}
		return nil, fmt.Errorf("failed to get service: %w", err)
	}

	return rm.ensureServiceUpToDate(ctx, service, workspace)
}

// ensureServiceUpToDate checks if service needs update and updates it if necessary
func (rm *ResourceManager) ensureServiceUpToDate(ctx context.Context, service *corev1.Service, workspace *workspacev1alpha1.Workspace) (*corev1.Service, error) {
	// Only perform updates when workspace is available to avoid interfering with creation
	if !rm.statusManager.IsWorkspaceAvailable(workspace) {
		return service, nil
	}

	needsUpdate, err := rm.serviceBuilder.NeedsUpdate(ctx, service, workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to check if service needs update: %w", err)
	}

	if needsUpdate {
		return rm.updateService(ctx, service, workspace)
	}

	return service, nil
}

// updateService updates an existing service with new spec
func (rm *ResourceManager) updateService(ctx context.Context, service *corev1.Service, workspace *workspacev1alpha1.Workspace) (*corev1.Service, error) {
	logger := logf.FromContext(ctx)

	// Update the service spec using the builder
	if err := rm.serviceBuilder.UpdateServiceSpec(ctx, service, workspace); err != nil {
		return nil, fmt.Errorf("failed to update service spec: %w", err)
	}

	logger.Info("Updating Service",
		"service", service.Name,
		"namespace", service.Namespace)

	if err := rm.client.Update(ctx, service); err != nil {
		return nil, fmt.Errorf("failed to update service: %w", err)
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

// EnsurePVCExists creates a PVC if it doesn't exist, or updates it if the spec differs
// It uses workspace storage if specified
func (rm *ResourceManager) EnsurePVCExists(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*corev1.PersistentVolumeClaim, error) {
	// Check if storage is needed from workspace
	hasStorage := workspace.Spec.Storage != nil

	if !hasStorage {
		return nil, nil // No storage requested
	}

	pvc, err := rm.getPVC(ctx, workspace)
	if err != nil {
		if errors.IsNotFound(err) {
			return rm.createPVC(ctx, workspace)
		}
		return nil, fmt.Errorf("failed to get PVC: %w", err)
	}

	return rm.ensurePVCUpToDate(ctx, pvc, workspace)
}

// ensurePVCUpToDate checks if PVC needs update and updates it if necessary
func (rm *ResourceManager) ensurePVCUpToDate(ctx context.Context, pvc *corev1.PersistentVolumeClaim, workspace *workspacev1alpha1.Workspace) (*corev1.PersistentVolumeClaim, error) {
	// Only perform updates when workspace is available to avoid interfering with creation
	if !rm.statusManager.IsWorkspaceAvailable(workspace) {
		return pvc, nil
	}

	needsUpdate, err := rm.pvcBuilder.NeedsUpdate(ctx, pvc, workspace)
	if err != nil {
		return nil, fmt.Errorf("failed to check if PVC needs update: %w", err)
	}

	if needsUpdate {
		return rm.updatePVC(ctx, pvc, workspace)
	}

	return pvc, nil
}

// updatePVC updates an existing PVC with new spec
func (rm *ResourceManager) updatePVC(ctx context.Context, pvc *corev1.PersistentVolumeClaim, workspace *workspacev1alpha1.Workspace) (*corev1.PersistentVolumeClaim, error) {
	logger := logf.FromContext(ctx)

	// Update the PVC spec using the builder
	if err := rm.pvcBuilder.UpdatePVCSpec(ctx, pvc, workspace); err != nil {
		return nil, fmt.Errorf("failed to update PVC spec: %w", err)
	}

	logger.Info("Updating PVC",
		"pvc", pvc.Name,
		"namespace", pvc.Namespace)

	if err := rm.client.Update(ctx, pvc); err != nil {
		return nil, fmt.Errorf("failed to update PVC: %w", err)
	}

	return pvc, nil
}

// CleanupAllResources performs comprehensive cleanup of all workspace resources
func (rm *ResourceManager) CleanupAllResources(ctx context.Context, workspace *workspacev1alpha1.Workspace) (bool, error) {
	logger := logf.FromContext(ctx)

	// Delete access strategy resources first
	accessError := rm.EnsureAccessResourcesDeleted(ctx, workspace)
	if accessError != nil {
		logger.Error(accessError, "Failed to delete access strategy resources")
		// Continue with other deletions, don't block on access strategy
	}

	// Delete deployment
	_, err := rm.EnsureDeploymentDeleted(ctx, workspace)
	if err != nil {
		return false, err
	}

	// Delete service
	_, err = rm.EnsureServiceDeleted(ctx, workspace)
	if err != nil {
		return false, err
	}

	// Delete PVC
	_, err = rm.EnsurePVCDeleted(ctx, workspace)
	if err != nil {
		return false, err
	}

	// Check if all resources are fully deleted using helper function
	if rm.AreAllResourcesDeleted(ctx, workspace) {
		logger.Info("All resources successfully deleted")
		return true, nil
	}

	// Resources still being deleted - requeue to check again
	return false, nil
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
