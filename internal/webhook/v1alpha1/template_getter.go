/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package v1alpha1

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	webhookconst "github.com/jupyter-infra/jupyter-k8s/internal/webhook"
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
