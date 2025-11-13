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

	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	webhookconst "github.com/jupyter-ai-contrib/jupyter-k8s/internal/webhook"
)

// TemplateGetter handles template retrieval and workspace mutation
type TemplateGetter struct {
	client client.Client
}

// NewTemplateGetter creates a new TemplateGetter instance
func NewTemplateGetter(c client.Client) *TemplateGetter {
	return &TemplateGetter{
		client: c,
	}
}

// ApplyTemplateName retrieves template and mutates workspace accordingly
func (tg *TemplateGetter) ApplyTemplateName(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	// Skip if workspace already has a template reference
	if workspace.Spec.TemplateRef != nil && workspace.Spec.TemplateRef.Name != "" {
		return nil
	}

	// List all WorkspaceTemplates with the default cluster template label
	templateList := &workspacev1alpha1.WorkspaceTemplateList{}
	err := tg.client.List(ctx, templateList, client.MatchingLabels{
		webhookconst.DefaultClusterTemplateLabel: "true",
	})
	if err != nil {
		return fmt.Errorf("failed to list templates with default-cluster-template label: %w", err)
	}

	// Check for multiple default templates
	if len(templateList.Items) > 1 {
		templateNames := getTemplateNames(templateList.Items)
		return fmt.Errorf("multiple templates found with default-cluster-template label: %v, expected exactly one", templateNames)
	}

	// If no default template found, continue without setting templateRef
	if len(templateList.Items) == 0 {
		return nil
	}

	// Set the template reference
	defaultTemplate := templateList.Items[0]
	workspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
		Name: defaultTemplate.Name,
	}

	return nil
}

// getTemplateNames extracts template names from a list of templates
func getTemplateNames(templates []workspacev1alpha1.WorkspaceTemplate) []string {
	names := make([]string, 0, len(templates))
	for _, template := range templates {
		names = append(names, template.Name)
	}
	return names
}
