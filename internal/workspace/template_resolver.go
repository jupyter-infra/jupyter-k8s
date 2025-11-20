package workspace

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// TemplateResolver handles centralized template resolution with namespace fallback logic
type TemplateResolver struct {
	client                   client.Client
	defaultTemplateNamespace string
}

// NewTemplateResolver creates a new TemplateResolver
func NewTemplateResolver(k8sClient client.Client, defaultTemplateNamespace string) *TemplateResolver {
	return &TemplateResolver{
		client:                   k8sClient,
		defaultTemplateNamespace: defaultTemplateNamespace,
	}
}

// ResolveTemplate finds a template using namespace fallback logic:
// 1. Try templateRef.namespace (if specified)
// 2. Try workspace.namespace (if templateRef.namespace empty)
// 3. Try defaultTemplateNamespace (if configured and previous failed)
func (tr *TemplateResolver) ResolveTemplate(ctx context.Context, templateRef *workspacev1alpha1.TemplateRef, workspaceNamespace string) (*workspacev1alpha1.WorkspaceTemplate, error) {
	if templateRef == nil {
		return nil, fmt.Errorf("templateRef is nil")
	}

	// Determine template namespace using fallback logic
	templateNamespace := templateRef.Namespace
	if templateNamespace == "" {
		templateNamespace = workspaceNamespace
	}

	// Try to get template from determined namespace
	template := &workspacev1alpha1.WorkspaceTemplate{}
	templateKey := client.ObjectKey{Name: templateRef.Name, Namespace: templateNamespace}
	err := tr.client.Get(ctx, templateKey, template)

	// If not found and we have a default namespace, try there
	if apierrors.IsNotFound(err) && tr.defaultTemplateNamespace != "" && templateNamespace != tr.defaultTemplateNamespace {
		templateKey = client.ObjectKey{Name: templateRef.Name, Namespace: tr.defaultTemplateNamespace}
		if fallbackErr := tr.client.Get(ctx, templateKey, template); fallbackErr == nil {
			return template, nil
		} else {
			return nil, fmt.Errorf("failed to get template %s from namespace %s or fallback namespace %s: %w", templateRef.Name, templateNamespace, tr.defaultTemplateNamespace, fallbackErr)
		}
	}

	if err != nil {
		return nil, fmt.Errorf("failed to get template %s: %w", templateRef.Name, err)
	}
	return template, nil
}

// ResolveTemplateForWorkspace convenience method that extracts templateRef and namespace from workspace
func (tr *TemplateResolver) ResolveTemplateForWorkspace(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*workspacev1alpha1.WorkspaceTemplate, error) {
	if workspace.Spec.TemplateRef == nil {
		return nil, fmt.Errorf("workspace has no templateRef")
	}
	return tr.ResolveTemplate(ctx, workspace.Spec.TemplateRef, workspace.Namespace)
}
