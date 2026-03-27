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
	client                   client.Client
	defaultTemplateNamespace string
}

// NewTemplateGetter creates a new TemplateGetter instance
func NewTemplateGetter(client client.Client, defaultTemplateNamespace string) *TemplateGetter {
	return &TemplateGetter{
		client:                   client,
		defaultTemplateNamespace: defaultTemplateNamespace,
	}
}

// ApplyTemplateName finds the default template and sets it on the workspace.
// It searches the workspace's namespace first, then the shared namespace (defaultTemplateNamespace).
// A local default template always takes priority over the shared one.
func (tg *TemplateGetter) ApplyTemplateName(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	// Skip if workspace already has a template reference
	if workspace.Spec.TemplateRef != nil && workspace.Spec.TemplateRef.Name != "" {
		return nil
	}

	defaultLabel := client.MatchingLabels{webhookconst.DefaultClusterTemplateLabel: "true"}

	// Search the workspace's own namespace first
	template, err := tg.findDefaultTemplate(ctx, workspace.Namespace, defaultLabel)
	if err != nil {
		return err
	}

	// Fall back to the shared namespace if no local default was found
	if template == nil && tg.defaultTemplateNamespace != "" && tg.defaultTemplateNamespace != workspace.Namespace {
		template, err = tg.findDefaultTemplate(ctx, tg.defaultTemplateNamespace, defaultLabel)
		if err != nil {
			return err
		}
	}

	if template == nil {
		return nil
	}

	workspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
		Name:      template.Name,
		Namespace: template.Namespace,
	}

	return nil
}

// findDefaultTemplate searches for a single default-labeled template in the given namespace.
// Returns nil if no default template is found. Returns an error if multiple are found.
func (tg *TemplateGetter) findDefaultTemplate(ctx context.Context, namespace string, labels client.MatchingLabels) (*workspacev1alpha1.WorkspaceTemplate, error) {
	templateList := &workspacev1alpha1.WorkspaceTemplateList{}
	if err := tg.client.List(ctx, templateList, labels, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("failed to list default templates in namespace %s: %w", namespace, err)
	}

	if len(templateList.Items) == 0 {
		return nil, nil
	}

	if len(templateList.Items) > 1 {
		return nil, fmt.Errorf(
			"multiple templates found with default-cluster-template label in namespace %s: %v, expected exactly one",
			namespace, getTemplateNames(templateList.Items),
		)
	}

	return &templateList.Items[0], nil
}

// getTemplateNames extracts template names from a list of templates
func getTemplateNames(templates []workspacev1alpha1.WorkspaceTemplate) []string {
	names := make([]string, 0, len(templates))
	for _, template := range templates {
		names = append(names, template.Name)
	}
	return names
}
