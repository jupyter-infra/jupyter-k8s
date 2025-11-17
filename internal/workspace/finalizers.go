package workspace

import (
	"context"

	"github.com/go-logr/logr"
	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// SafelyAddFinalizerToAccessStrategy attempts to add a finalizer to an AccessStrategy,
// handling conflict errors by checking if the finalizer was added in a concurrent operation.
func SafelyAddFinalizerToAccessStrategy(
	ctx context.Context,
	logger logr.Logger,
	k8sClient client.Client,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) error {

	// Check if finalizer is already present
	if controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName) {
		logger.V(1).Info("Finalizer already present on AccessStrategy")
		return nil
	}

	// Add the finalizer
	logger.Info("Adding finalizer to AccessStrategy")
	controllerutil.AddFinalizer(accessStrategy, AccessStrategyFinalizerName)

	// Try to update
	updateErr := k8sClient.Update(ctx, accessStrategy)
	if updateErr == nil {
		logger.Info("Successfully added finalizer to AccessStrategy")
		return nil
	}

	// Check if it's a conflict error
	if !errors.IsConflict(updateErr) {
		logger.Error(updateErr, "Failed to add finalizer to AccessStrategy (non-conflict error)")
		return updateErr
	}

	// Get the latest version
	latestAccessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{}
	if err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      accessStrategy.Name,
		Namespace: accessStrategy.Namespace,
	}, latestAccessStrategy); err != nil {
		logger.Error(err, "Failed to get latest AccessStrategy")
		return err
	}

	// Check if finalizer is already present in the latest version
	if controllerutil.ContainsFinalizer(latestAccessStrategy, AccessStrategyFinalizerName) {
		logger.Info("Finalizer already present in latest version of AccessStrategy")
		return nil
	}

	// Finalizer is not present in the latest version, return the conflict error
	logger.Error(updateErr, "Finalizer not present in latest version, returning conflict error")
	return updateErr
}

// SafelyRemoveFinalizerFromAccessStrategy attempts to remove a finalizer from an AccessStrategy,
// handling conflict errors by checking if the finalizer was removed in a concurrent operation.
//
// If deletedOk is true, NotFound errors are treated as success (the finalizer is effectively removed
// if the resource itself is gone).
func SafelyRemoveFinalizerFromAccessStrategy(
	ctx context.Context,
	logger logr.Logger,
	k8sClient client.Client,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	deletedOk bool) error {

	// Check if finalizer is present
	if !controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName) {
		logger.V(1).Info("Finalizer not present on AccessStrategy")
		return nil
	}

	// Remove the finalizer
	logger.Info("Removing finalizer from AccessStrategy")
	controllerutil.RemoveFinalizer(accessStrategy, AccessStrategyFinalizerName)

	// Try to update
	updateErr := k8sClient.Update(ctx, accessStrategy)
	if updateErr == nil {
		logger.Info("Successfully removed finalizer from AccessStrategy")
		return nil
	}

	// Handle NotFound error if deletedOk is true
	if errors.IsNotFound(updateErr) && deletedOk {
		logger.Info("AccessStrategy not found but deletedOk is true, considering finalizer removal successful")
		return nil
	}

	// Check if it's a conflict error
	if !errors.IsConflict(updateErr) {
		logger.Error(updateErr, "Failed to remove finalizer from AccessStrategy (non-conflict error)")
		return updateErr
	}

	// Get the latest version
	latestAccessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      accessStrategy.Name,
		Namespace: accessStrategy.Namespace,
	}, latestAccessStrategy)

	// Handle NotFound error during refresh if deletedOk is true
	if err != nil {
		if errors.IsNotFound(err) && deletedOk {
			logger.Info("AccessStrategy no longer exists but deletedOk is true, considering finalizer removal successful")
			return nil
		}
		logger.Error(err, "Failed to get latest AccessStrategy")
		return err
	}

	// Check if finalizer is already gone in the latest version
	if !controllerutil.ContainsFinalizer(latestAccessStrategy, AccessStrategyFinalizerName) {
		logger.Info("Finalizer already removed in latest version of AccessStrategy")
		return nil
	}

	// Finalizer is still present in the latest version, return the conflict error
	logger.Error(updateErr, "Finalizer still present in latest version, returning conflict error")
	return updateErr
}
