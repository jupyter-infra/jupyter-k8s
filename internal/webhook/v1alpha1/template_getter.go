/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
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

	// Check namespace scope strategy to determine search scope
	scope, err := GetTemplateScopeStrategy(ctx, tg.client, workspace.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get template scope strategy: %w", err)
	}

	// List WorkspaceTemplates with the default cluster template label
	listOpts := []client.ListOption{
		client.MatchingLabels{webhookconst.DefaultClusterTemplateLabel: "true"},
	}
	// When namespace is scoped, only search within the workspace's namespace
	if scope == webhookconst.TemplateScopeNamespaced {
		listOpts = append(listOpts, client.InNamespace(workspace.Namespace))
	}

	templateList := &workspacev1alpha1.WorkspaceTemplateList{}
	if err := tg.client.List(ctx, templateList, listOpts...); err != nil {
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
