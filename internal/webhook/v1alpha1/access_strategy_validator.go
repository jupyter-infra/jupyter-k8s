/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// AccessStrategyValidator handles access strategy validation for webhooks
type AccessStrategyValidator struct {
	client client.Client
}

// NewAccessStrategyValidator creates a new AccessStrategyValidator
func NewAccessStrategyValidator(k8sClient client.Client) *AccessStrategyValidator {
	return &AccessStrategyValidator{
		client: k8sClient,
	}
}

// ValidateAccessStrategyResources validates workspace resources against access strategy constraints
func (asv *AccessStrategyValidator) ValidateAccessStrategyResources(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	if workspace.Spec.AccessStrategy == nil {
		return nil
	}

	strategy, err := asv.fetchAccessStrategy(ctx, workspace)
	if err != nil {
		return err
	}

	if strategy == nil {
		return nil // Access strategy not found, skip validation
	}

	return asv.validateResourceBounds(workspace, strategy)
}

// fetchAccessStrategy retrieves an access strategy by name
func (asv *AccessStrategyValidator) fetchAccessStrategy(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
	strategy := &workspacev1alpha1.WorkspaceAccessStrategy{}
	if err := asv.client.Get(ctx, types.NamespacedName{
		Name:      workspace.Spec.AccessStrategy.Name,
		Namespace: workspace.Namespace,
	}, strategy); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return nil, nil // Access strategy not found, skip validation
		}
		return nil, fmt.Errorf("failed to get access strategy %s: %w", workspace.Spec.AccessStrategy.Name, err)
	}
	return strategy, nil
}

// validateResourceBounds validates workspace resources against access strategy bounds
func (asv *AccessStrategyValidator) validateResourceBounds(workspace *workspacev1alpha1.Workspace, strategy *workspacev1alpha1.WorkspaceAccessStrategy) error {
	if strategy.Spec.ResourceBounds == nil || workspace.Spec.Resources == nil {
		return nil
	}

	contextName := fmt.Sprintf("access strategy '%s'", strategy.Name)
	violations := validateResourceBounds(*workspace.Spec.Resources, strategy.Spec.ResourceBounds, contextName)

	if len(violations) > 0 {
		return fmt.Errorf("workspace violates access strategy '%s' resource bounds: %s",
			workspace.Spec.AccessStrategy.Name, formatViolations(violations))
	}

	return nil
}
