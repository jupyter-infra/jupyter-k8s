/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

// GetIntegrationStrategyForWorkspace retrieves the IntegrationStrategy specified in the Workspace.Spec.
// Returns nil if no IntegrationStrategy is specified.
func (rm *ResourceManager) GetIntegrationStrategyForWorkspace(
	ctx context.Context,
	workspace *workspacev1alpha1.Workspace,
) (*workspacev1alpha1.WorkspaceIntegrationStrategy, error) {
	ref := workspace.Spec.IntegrationStrategy
	if ref == nil {
		// no-op: no IntegrationStrategy
		return nil, nil
	}

	// Determine namespace for the IntegrationStrategy
	integrationStrategyNamespace := workspace.Namespace
	if ref.Namespace != "" {
		integrationStrategyNamespace = ref.Namespace
	}

	// Get the IntegrationStrategy
	strategy := &workspacev1alpha1.WorkspaceIntegrationStrategy{}
	err := rm.client.Get(ctx, types.NamespacedName{
		Name:      ref.Name,
		Namespace: integrationStrategyNamespace,
	}, strategy)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("integration strategy %s not found in namespace %s", ref.Name, integrationStrategyNamespace)
		}
		return nil, fmt.Errorf("failed to get integration strategy: %w", err)
	}

	return strategy, nil
}
