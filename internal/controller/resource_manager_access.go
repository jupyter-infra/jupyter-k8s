package controller

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// GetAccessStrategyForWorkspace retrieves the AccessStrategy specified in the Workspace.Spec
// Or nil if no AccessStrategy is specified.
func (rm *ResourceManager) GetAccessStrategyForWorkspace(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
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
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{}
	err := rm.client.Get(ctx, types.NamespacedName{
		Name:      accessStrategyRef.Name,
		Namespace: accessStrategyNamespace,
	}, accessStrategy)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("access strategy %s not found in namespace %s", accessStrategyRef.Name, accessStrategyNamespace)
		}
		return nil, fmt.Errorf("failed to get access strategy: %w", err)
	}

	return accessStrategy, nil
}

// EnsureAccessResourcesExist creates or updates routing resources for the Workspace
func (rm *ResourceManager) EnsureAccessResourcesExist(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
) error {
	logger := logf.FromContext(ctx)
	// The AccessResource MUST be in the Workspace namespace
	// in order for the Workspace is the owner of the AccessResource
	accessResourceNamespace := workspace.Namespace

	// Track which resources are defined in the current AccessStrategy
	currentResources := make(map[string]bool)

	// ensure each of the resources defined in the accessStrategy exists
	for _, resourceTemplate := range accessStrategy.Spec.AccessResourceTemplates {
		// Build the lookup name that will be stored in status
		lookupName := fmt.Sprintf("%s-%s", resourceTemplate.NamePrefix, workspace.Name)
		// Track this resource as defined in the current AccessStrategy
		resourceKey := fmt.Sprintf("%s/%s/%s", resourceTemplate.Kind, lookupName, accessResourceNamespace)
		currentResources[resourceKey] = true

		// Apply resource
		err := rm.ensureAccessResourceExists(ctx, workspace, accessStrategy, service, &resourceTemplate, accessResourceNamespace)
		if err != nil {
			return err
		}
	}

	// Check for resources that exist in status but are no longer in the AccessStrategy
	// These need to be cleaned up
	var resourcesToDelete []workspacev1alpha1.AccessResourceStatus
	var resourcesToKeep []workspacev1alpha1.AccessResourceStatus

	for _, resource := range workspace.Status.AccessResources {
		resourceKey := fmt.Sprintf("%s/%s/%s", resource.Kind, resource.Name, resource.Namespace)
		if !currentResources[resourceKey] {
			// This resource is no longer in the AccessStrategy, mark for deletion
			resourcesToDelete = append(resourcesToDelete, resource)
			logger.Info("Found access resource to delete (no longer in AccessStrategy)",
				"kind", resource.Kind,
				"name", resource.Name,
				"namespace", resource.Namespace)
		} else {
			// Keep resources that are still in the AccessStrategy
			resourcesToKeep = append(resourcesToKeep, resource)
		}
	}

	// Delete resources that are no longer in the AccessStrategy
	resourcesDeleted := false
	for _, resource := range resourcesToDelete {
		resourceCopy := resource // Create a copy to avoid pointer issues in the loop
		removed, err := rm.ensureAccessResourceDeleted(ctx, &resourceCopy)
		if err != nil {
			return fmt.Errorf("failed to delete removed access resource: %w", err)
		}
		if removed {
			resourcesDeleted = true
			logger.Info("Deleted access resource that was removed from AccessStrategy",
				"kind", resourceCopy.Kind,
				"name", resourceCopy.Name,
				"namespace", resourceCopy.Namespace)
		}
	}

	// Only update the status array if resources were actually deleted
	if resourcesDeleted {
		workspace.Status.AccessResources = resourcesToKeep
	}

	return nil
}

// applyResource creates or updates a resource
func (rm *ResourceManager) ensureAccessResourceExists(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	service *corev1.Service,
	resourceTemplate *workspacev1alpha1.AccessResourceTemplate,
	accessResourceNamespace string,
) error {
	logger := logf.FromContext(ctx)

	// Check if the resource exists
	var accessResourceStatus *workspacev1alpha1.AccessResourceStatus
	statusIdx := -1
	lookupName := fmt.Sprintf("%s-%s", resourceTemplate.NamePrefix, workspace.Name)

	for idx, existingResourceStatus := range workspace.Status.AccessResources {
		if existingResourceStatus.Kind == resourceTemplate.Kind && existingResourceStatus.Name == lookupName && existingResourceStatus.Namespace == accessResourceNamespace {
			accessResourceStatus = &existingResourceStatus
			statusIdx = idx
			break
		}
	}

	removedFromStatus := false

	// CASE 1: resource exists in status
	if accessResourceStatus != nil {
		existingObj := &unstructured.Unstructured{}

		// Set the GVK before getting the resource
		gvk := rm.getGroupVersionKind(accessResourceStatus.APIVersion, accessResourceStatus.Kind)
		existingObj.SetGroupVersionKind(gvk)

		lookupError := rm.client.Get(ctx, types.NamespacedName{
			Namespace: accessResourceStatus.Namespace,
			Name:      accessResourceStatus.Name,
		}, existingObj)

		if lookupError == nil {
			// Resource exists, but we need to check if it matches the current AccessStrategy template
			// Build the expected resource from the current template
			expectedObj, err := rm.accessResourcesBuilder.BuildUnstructuredResource(*resourceTemplate, workspace, accessStrategy, service)
			if err != nil {
				return fmt.Errorf("failed to build expected resource: %w", err)
			}

			// Compare the specs to detect changes
			existingSpec, existingFound, err := unstructured.NestedFieldCopy(existingObj.Object, "spec")
			if err != nil {
				return fmt.Errorf("error getting existing spec: %w", err)
			}

			expectedSpec, expectedFound, err := unstructured.NestedFieldCopy(expectedObj.Object, "spec")
			if err != nil {
				return fmt.Errorf("error getting expected spec: %w", err)
			}

			// If specs are different, update the resource
			if existingFound && expectedFound && !reflect.DeepEqual(existingSpec, expectedSpec) {
				logger.Info("AccessResource spec doesn't match template, updating",
					"kind", existingObj.GetKind(),
					"name", existingObj.GetName(),
					"namespace", existingObj.GetNamespace())

				// Preserve metadata from existing object
				expectedObj.SetResourceVersion(existingObj.GetResourceVersion())

				// Update the resource
				if err := rm.client.Update(ctx, expectedObj); err != nil {
					return fmt.Errorf("failed to update access resource: %w", err)
				}

				logger.Info("Updated AccessResource to match template",
					"kind", expectedObj.GetKind(),
					"name", expectedObj.GetName(),
					"namespace", expectedObj.GetNamespace())
			}

			// Resource exists and is now up to date
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
		removedFromStatus = true

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

	// Check if this resource already exists in the status
	addToStatus := accessResourceStatus == nil || removedFromStatus

	if err := rm.client.Create(ctx, obj); err != nil {
		// If resource already exists, try update
		if errors.IsAlreadyExists(err) {
			existingObj := &unstructured.Unstructured{}
			existingObj.SetGroupVersionKind(obj.GroupVersionKind())

			// Get the existing resource
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

	// Only add to status after successful update if it doesn't already exist
	if addToStatus {
		workspace.Status.AccessResources = append(workspace.Status.AccessResources, workspacev1alpha1.AccessResourceStatus{
			Kind:       obj.GetKind(),
			APIVersion: obj.GetAPIVersion(),
			Name:       obj.GetName(),
			Namespace:  obj.GetNamespace(),
		})
	}
	logger.Info("Applied resource",
		"kind", obj.GetKind(),
		"name", obj.GetName(),
		"namespace", obj.GetNamespace())
	return nil
}

// getGroupVersionKind parses the API version and returns a GroupVersionKind struct
func (rm *ResourceManager) getGroupVersionKind(apiVersion string, kind string) schema.GroupVersionKind {
	var group, version string
	parts := strings.Split(apiVersion, "/")
	if len(parts) == 2 {
		group = parts[0]
		version = parts[1]
	} else {
		version = apiVersion
	}

	return schema.GroupVersionKind{
		Group:   group,
		Version: version,
		Kind:    kind,
	}
}

// ensureAccessResourceDeleted queries and attempts to delete a specific accessResource
// from its reference in Workspace.Status
func (rm *ResourceManager) ensureAccessResourceDeleted(
	ctx context.Context,
	accessResource *workspacev1alpha1.AccessResourceStatus) (bool, error) {
	logger := logf.FromContext(ctx)
	existingAccessResource := &unstructured.Unstructured{}

	// Set the GVK before getting the resource
	gvk := rm.getGroupVersionKind(accessResource.APIVersion, accessResource.Kind)
	existingAccessResource.SetGroupVersionKind(gvk)

	getAccessResourceErr := rm.client.Get(ctx, types.NamespacedName{
		Name:      accessResource.Name,
		Namespace: accessResource.Namespace,
	}, existingAccessResource)

	if getAccessResourceErr != nil {
		if errors.IsNotFound(getAccessResourceErr) {
			logger.Info("AccessResource '%s' in namespace '%s' is deleted.", accessResource.Name, accessResource.Namespace)
			return true, nil
		}
		return false, fmt.Errorf("failed to retrieve AccessResource: %w", getAccessResourceErr)
	}

	// Delete resource
	if err := rm.client.Delete(ctx, existingAccessResource); err != nil {
		if errors.IsNotFound(err) {
			// Resource doesn't exist, nothing to do
			logger.Info("AccessResource '%s' in namespace '%s' is deleted.", accessResource.Name, accessResource.Namespace)
			return true, nil
		}
		return false, fmt.Errorf("failed to delete resource: %w", err)
	}
	logger.Info("Deleted resource",
		"kind", accessResource.Kind,
		"name", accessResource.Name,
		"namespace", accessResource.Namespace)
	return true, nil
}

// EnsureAccessResourcesDeleted removes routing resources for the Workspace.
func (rm *ResourceManager) EnsureAccessResourcesDeleted(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) error {
	if len(workspace.Status.AccessResources) == 0 {
		// no-op: nothing to delete
		return nil
	}

	copiedAccessResources := make([]workspacev1alpha1.AccessResourceStatus, len(workspace.Status.AccessResources))
	copy(copiedAccessResources, workspace.Status.AccessResources)

	// creates an empty slice with the same underlying array
	var filteredResources []workspacev1alpha1.AccessResourceStatus
	for _, accessResource := range copiedAccessResources {
		removed, err := rm.ensureAccessResourceDeleted(ctx, &accessResource)
		if err != nil {
			return err
		}
		if !removed {
			filteredResources = append(filteredResources, accessResource)
		}
	}

	// update the Status.AccessResources array
	workspace.Status.AccessResources = filteredResources

	return nil
}

// AreAccessResourcesDeleted returns true if the workspace.Status.AccessResources is no longer tracking resources.
func (rm *ResourceManager) AreAccessResourcesDeleted(workspace *workspacev1alpha1.Workspace) bool {
	return len(workspace.Status.AccessResources) == 0 // len(nil) returns 0
}
