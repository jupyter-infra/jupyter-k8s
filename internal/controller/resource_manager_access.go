package controller

import (
	"context"
	"fmt"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// GetAccessStrategyForWorkspace retrieves the AccessStrategy specified in the Workspace.Spec
// Or nil if no AccessStrategy is specified.
func (rm *ResourceManager) GetAccessStrategyForWorkspace(
	ctx context.Context,
	workspace *workspacesv1alpha1.Workspace,
) (*workspacesv1alpha1.WorkspaceAccessStrategy, error) {
	logger := logf.FromContext(ctx)

	accessStrategyRef := workspace.Spec.AccessStrategy
	if accessStrategyRef == nil {
		// no-op: no AccessStrategy
		return nil, nil
	}

	// Determine namespace for the AccessStrategy
	accessStrategyNamespace := workspace.Namespace
	if workspace.Spec.AccessStrategy != nil && workspace.Spec.AccessStrategy.Namespace != "" {
		accessStrategyNamespace = workspace.Spec.AccessStrategy.Namespace
	}

	// Get the AccessStrategy
	accessStrategy := &workspacesv1alpha1.WorkspaceAccessStrategy{}
	err := rm.client.Get(ctx, types.NamespacedName{
		Name:      accessStrategyRef.Name,
		Namespace: accessStrategyNamespace,
	}, accessStrategy)

	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("Access strategy not found", "name", accessStrategyRef.Name, "namespace", accessStrategyNamespace)
			return nil, fmt.Errorf("access strategy %s not found in namespace %s", accessStrategyRef.Name, accessStrategyNamespace)
		}
		return nil, fmt.Errorf("failed to get access strategy: %w", err)
	}

	return accessStrategy, nil
}

// EnsureAccessResourcesExist creates or updates routing resources for the Workspace
func (rm *ResourceManager) EnsureAccessResourcesExist(
	ctx context.Context,
	workspace *workspacesv1alpha1.Workspace,
	accessStrategy *workspacesv1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
) error {
	// Determine namespace for the AccessResources based on the namespace
	// specified in the AccessStrategy;
	// or fallback to the Workspace's namespace.
	accessResourceNamespace := workspace.Namespace
	if accessStrategy.Spec.RoutesNamespace != "" {
		accessResourceNamespace = accessStrategy.Spec.RoutesNamespace
	}

	// ensure each of the resources defined in the accessStrategy exists
	for _, resourceTemplate := range accessStrategy.Spec.RoutesResourceTemplates {
		// Apply resource
		err := rm.ensureAccessResourceExists(ctx, workspace, accessStrategy, service, &resourceTemplate, accessResourceNamespace)
		if err != nil {
			return err
		}
	}

	return nil
}

// applyResource creates or updates a resource
func (rm *ResourceManager) ensureAccessResourceExists(
	ctx context.Context,
	workspace *workspacesv1alpha1.Workspace,
	accessStrategy *workspacesv1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
	resourceTemplate *workspacesv1alpha1.AccessResourceTemplate,
	accessResourceNamespace string,
) error {
	logger := logf.FromContext(ctx)

	// Check if the resource exists
	var accessResourceStatus *workspacesv1alpha1.AccessResourceStatus
	statusIdx := -1
	lookupName := fmt.Sprintf("%s-%s", resourceTemplate.NamePrefix, workspace.Name)

	for idx, existingResourceStatus := range workspace.Status.AccessResources {
		if existingResourceStatus.Kind == resourceTemplate.Kind && existingResourceStatus.Name == lookupName && existingResourceStatus.Namespace == accessResourceNamespace {
			accessResourceStatus = &existingResourceStatus
			statusIdx = idx
			break
		}
	}

	// CASE 1: resource exists in status
	if accessResourceStatus != nil {
		existingObj := &unstructured.Unstructured{}
		existingObj.SetKind(accessResourceStatus.Kind)
		lookupError := rm.client.Get(ctx, types.NamespacedName{
			Namespace: accessResourceStatus.Name,
			Name:      accessResourceStatus.Namespace,
		}, existingObj)

		if lookupError == nil {
			// resource exists, all good
			return nil
		} else if !errors.IsNotFound(lookupError) {
			// problem getting the resource: exit
			// leave the status as is
			return fmt.Errorf("error getting access resource: %w", lookupError)
		}
		// otherwise the status is incorrect
		// remove the status entry
		workspace.Status.AccessResources = append(
			workspace.Status.AccessResources[:statusIdx], workspace.Status.AccessResources[statusIdx+1:]...)

		// continue to create the resource (case 2)
	}
	// END OF CASE1: resource exists in status

	// CASE 2: resource doesn't exist, try to create it
	// First, build the resource
	obj, err := rm.accessResourcesBuilder.BuildUnstructuredResource(*resourceTemplate, workspace, accessStrategy, service)
	if err != nil {
		return fmt.Errorf("failed to build resource: %w", err)
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(workspace, obj, rm.scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := rm.client.Create(ctx, obj); err != nil {
		// If resource already exists, try update
		if errors.IsAlreadyExists(err) {
			existingObj := &unstructured.Unstructured{}
			existingObj.SetGroupVersionKind(obj.GroupVersionKind())
			workspace.Status.AccessResources = append(workspace.Status.AccessResources, workspacesv1alpha1.AccessResourceStatus{
				Kind:      obj.GetKind(),
				Name:      obj.GetName(),
				Namespace: obj.GetNamespace(),
			})
			if err := rm.client.Get(ctx, types.NamespacedName{
				Namespace: obj.GetNamespace(),
				Name:      obj.GetName(),
			}, existingObj); err != nil {
				return fmt.Errorf("failed to get existing resource: %w", err)
			}

			// Copy resource version to ensure proper update
			obj.SetResourceVersion(existingObj.GetResourceVersion())

			// Update resource
			if err := rm.client.Update(ctx, obj); err != nil {
				return fmt.Errorf("failed to update resource: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create resource: %w", err)
		}
	}
	workspace.Status.AccessResources = append(workspace.Status.AccessResources, workspacesv1alpha1.AccessResourceStatus{
		Kind:      obj.GetKind(),
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	})
	logger.Info("Applied resource",
		"kind", obj.GetKind(),
		"name", obj.GetName(),
		"namespace", obj.GetNamespace())
	return nil
}

// EnsureAccessResourcesDeleted removes routing resources for the Workspace
func (rm *ResourceManager) EnsureAccessResourcesDeleted(
	ctx context.Context,
	workspace *workspacesv1alpha1.Workspace,
) error {
	logger := logf.FromContext(ctx)
	if len(workspace.Status.AccessResources) == 0 {
		// no-op: nothing to delete
		return nil
	}

	// Remove all existing AccessResources as tracked in Status
	for len(workspace.Status.AccessResources) > 0 {
		// pop the last index
		accessResource := workspace.Status.AccessResources[len(workspace.Status.AccessResources)-1]

		existingAccessResource := &unstructured.Unstructured{}
		existingAccessResource.SetGroupVersionKind(existingAccessResource.GroupVersionKind())
		getAccessResourceErr := rm.client.Get(ctx, types.NamespacedName{
			Name:      accessResource.Name,
			Namespace: accessResource.Namespace,
		}, existingAccessResource)

		if getAccessResourceErr != nil {
			if errors.IsNotFound(getAccessResourceErr) {
				logger.Info("AccessResource '%s' in namespace '%s' already deleted.", accessResource.Name, accessResource.Namespace)
				workspace.Status.AccessResources = workspace.Status.AccessResources[:len(workspace.Status.AccessResources)-1]
				continue
			}
			return fmt.Errorf("failed to retrieve AccessResource: %w", getAccessResourceErr)
		}

		// Delete resource
		if err := rm.client.Delete(ctx, existingAccessResource); err != nil {
			if errors.IsNotFound(err) {
				// Resource doesn't exist, nothing to do
				workspace.Status.AccessResources = workspace.Status.AccessResources[:len(workspace.Status.AccessResources)-1]
				continue
			}
			return fmt.Errorf("failed to delete resource: %w", err)
		}
		logger.Info("Deleted resource",
			"kind", accessResource.Kind,
			"name", accessResource.Name,
			"namespace", accessResource.Namespace)
		workspace.Status.AccessResources = workspace.Status.AccessResources[:len(workspace.Status.AccessResources)-1]
	}

	return nil
}
