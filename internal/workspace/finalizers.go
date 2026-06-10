/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package workspace

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// SafelyAddFinalizerToAccessStrategy attempts to add the named finalizer to an AccessStrategy,
// handling conflict errors by checking if the finalizer was added in a concurrent operation.
//
// finalizerName selects which protection finalizer to manage: AccessStrategyFinalizerName for
// workspace references, or AccessStrategyTemplateFinalizerName for template references. The two are
// managed independently so the access strategy survives until all referrer types release it.
func SafelyAddFinalizerToAccessStrategy(
	ctx context.Context,
	logger logr.Logger,
	k8sClient client.Client,
	accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
	finalizerName string) error {

	// Check if finalizer is already present
	if controllerutil.ContainsFinalizer(accessStrategy, finalizerName) {
		logger.V(1).Info("Finalizer already present on AccessStrategy", "finalizer", finalizerName)
		return nil
	}

	// Add the finalizer
	logger.Info("Adding finalizer to AccessStrategy", "finalizer", finalizerName)
	controllerutil.AddFinalizer(accessStrategy, finalizerName)

	// Try to update
	updateErr := k8sClient.Update(ctx, accessStrategy)
	if updateErr == nil {
		logger.Info("Successfully added finalizer to AccessStrategy", "finalizer", finalizerName)
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
	if controllerutil.ContainsFinalizer(latestAccessStrategy, finalizerName) {
		logger.Info("Finalizer already present in latest version of AccessStrategy", "finalizer", finalizerName)
		return nil
	}

	// Finalizer is not present in the latest version, return the conflict error
	logger.Error(updateErr, "Finalizer not present in latest version, returning conflict error")
	return updateErr
}

// EnsureAccessStrategyFinalizerByRef adds the named protection finalizer to the AccessStrategy
// identified by name/namespace, if it does not already carry it. It is the shared lazy-finalizer
// primitive used by referrers to mark an AccessStrategy as in-use; callers pass the finalizer name for
// their referrer type (e.g. AccessStrategyTemplateFinalizerName for templates).
//
// mustExist controls how a missing AccessStrategy is treated:
//   - true (admission webhook): a missing AccessStrategy is an error, so create/update of a referrer
//     pointing at a non-existent AccessStrategy is rejected. This is the layer that actually prevents
//     dangling references from being persisted.
//   - false (controller backfill): a missing AccessStrategy is tolerated and skipped. A referrer only
//     reaches the controller with a dangling reference if the webhook was bypassed (failurePolicy:
//     Ignore - pod down, racing, or a pre-existing resource), and by then it is already in etcd. The
//     controller cannot un-persist it or conjure the AccessStrategy, so returning an error here would
//     only hot-loop the reconcile on an unfixable condition with no upside. Instead we skip; the
//     AccessStrategy controller backfills the finalizer if the AccessStrategy is created later.
//
// Tolerating the dangling reference on the controller path does not leak to runtime: a workspace that
// inherits such a reference (directly or via a template's defaultAccessStrategy) is rejected fail-fast by
// the workspace admission webhook, which validates AccessStrategy existence unconditionally under
// failurePolicy: fail. The dangling referrer is therefore inert - it cannot back a running workspace.
//
// Removal is intentionally not handled here - it stays lazy in the AccessStrategy controller, which can
// see when this referrer type no longer references the AccessStrategy.
func EnsureAccessStrategyFinalizerByRef(
	ctx context.Context,
	logger logr.Logger,
	k8sClient client.Client,
	accessStrategyName string,
	accessStrategyNamespace string,
	finalizerName string,
	mustExist bool) error {

	if accessStrategyName == "" {
		return nil
	}

	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{}
	err := k8sClient.Get(ctx, types.NamespacedName{
		Name:      accessStrategyName,
		Namespace: accessStrategyNamespace,
	}, accessStrategy)
	if err != nil {
		if errors.IsNotFound(err) {
			if mustExist {
				logger.Info("Referenced AccessStrategy not found",
					"accessStrategy", accessStrategyName, "namespace", accessStrategyNamespace)
				return fmt.Errorf("referenced AccessStrategy %s not found in namespace %s",
					accessStrategyName, accessStrategyNamespace)
			}
			// AccessStrategy doesn't exist yet; the AccessStrategy controller will backfill the
			// finalizer once it observes this referrer. Not an error on the backfill path.
			logger.V(1).Info("AccessStrategy not found while ensuring finalizer, skipping",
				"accessStrategy", accessStrategyName, "namespace", accessStrategyNamespace)
			return nil
		}
		return fmt.Errorf("failed to get AccessStrategy %s/%s: %w",
			accessStrategyNamespace, accessStrategyName, err)
	}

	if controllerutil.ContainsFinalizer(accessStrategy, finalizerName) {
		return nil
	}

	return SafelyAddFinalizerToAccessStrategy(ctx, logger, k8sClient, accessStrategy, finalizerName)
}
