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
	client client.Client
}

// NewTemplateDefaulter creates a new TemplateDefaulter
func NewTemplateDefaulter(k8sClient client.Client) *TemplateDefaulter {
	return &TemplateDefaulter{
		client: k8sClient,
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
}

// ApplyTemplateDefaults applies template defaults to workspace
func (td *TemplateDefaulter) ApplyTemplateDefaults(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	if workspace.Spec.TemplateRef == nil {
		return nil
	}

	template, err := td.fetchTemplate(ctx, *workspace.Spec.TemplateRef)
	if err != nil {
		return err
	}

	// Apply all defaults using registered applicators
	for _, applicator := range defaultApplicators {
		applicator(workspace, template)
	}

	return nil
}

// fetchTemplate retrieves a template by name
func (td *TemplateDefaulter) fetchTemplate(ctx context.Context, templateName string) (*workspacev1alpha1.WorkspaceTemplate, error) {
	template := &workspacev1alpha1.WorkspaceTemplate{}
	if err := td.client.Get(ctx, types.NamespacedName{Name: templateName}, template); err != nil {
		return nil, fmt.Errorf("failed to get template %s: %w", templateName, err)
	}
	return template, nil
}
