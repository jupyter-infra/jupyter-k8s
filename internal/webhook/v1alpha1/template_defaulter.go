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

// DefaultApplicator applies defaults for a specific field or group of fields
type DefaultApplicator func(workspace *workspacev1alpha1.Workspace, template *workspacev1alpha1.WorkspaceTemplate)

// TemplateDefaulter handles applying template defaults to workspaces
type TemplateDefaulter struct {
	client                   client.Client
	defaultTemplateNamespace string
}

// NewTemplateDefaulter creates a new TemplateDefaulter
func NewTemplateDefaulter(k8sClient client.Client, defaultTemplateNamespace string) *TemplateDefaulter {
	return &TemplateDefaulter{
		client:                   k8sClient,
		defaultTemplateNamespace: defaultTemplateNamespace,
	}
}

// defaultApplicators is the registry of all default applicators
var defaultApplicators = []DefaultApplicator{
	applyCoreDefaults,
	applyResourceDefaults,
	applyStorageDefaults,
	applySchedulingDefaults,
	applyMetadataDefaults,
	applyAccessStrategyDefaults,
	applyLifecycleDefaults,
	applySecurityDefaults,
}

// ApplyTemplateDefaults applies template defaults to workspace
func (td *TemplateDefaulter) ApplyTemplateDefaults(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	if workspace.Spec.TemplateRef == nil || workspace.Spec.TemplateRef.Name == "" {
		return nil
	}

	template, err := td.fetchTemplate(ctx, *workspace.Spec.TemplateRef, workspace.Namespace)
	if err != nil {
		return err
	}

	// Apply all defaults using registered applicators
	for _, applicator := range defaultApplicators {
		applicator(workspace, template)
	}

	return nil
}

// fetchTemplate retrieves a template by WorkspaceTemplateRef with fallback logic
func (td *TemplateDefaulter) fetchTemplate(ctx context.Context, templateRef workspacev1alpha1.TemplateRef, workspaceNamespace string) (*workspacev1alpha1.WorkspaceTemplate, error) {
	template := &workspacev1alpha1.WorkspaceTemplate{}

	// If namespace is explicitly specified, use it directly
	if templateRef.Namespace != "" {
		namespacedName := types.NamespacedName{
			Name:      templateRef.Name,
			Namespace: templateRef.Namespace,
		}
		if err := td.client.Get(ctx, namespacedName, template); err != nil {
			return nil, fmt.Errorf("failed to get template %s in namespace %s: %w", templateRef.Name, templateRef.Namespace, err)
		}
		return template, nil
	}

	// If namespace not specified, try workspace namespace first
	namespacedName := types.NamespacedName{
		Name:      templateRef.Name,
		Namespace: workspaceNamespace,
	}
	err := td.client.Get(ctx, namespacedName, template)
	if err == nil {
		return template, nil
	}

	// If not found in workspace namespace and default namespace is configured, try default namespace
	if td.defaultTemplateNamespace != "" && td.defaultTemplateNamespace != workspaceNamespace {
		namespacedName.Namespace = td.defaultTemplateNamespace
		if err := td.client.Get(ctx, namespacedName, template); err != nil {
			return nil, fmt.Errorf("failed to get template %s in workspace namespace %s or default namespace %s", templateRef.Name, workspaceNamespace, td.defaultTemplateNamespace)
		}
		return template, nil
	}

	// Return the original error if no fallback available
	return nil, fmt.Errorf("failed to get template %s in namespace %s: %w", templateRef.Name, workspaceNamespace, err)
}
