/*
MIT License

Copyright (c) 2025 jupyter-ai-contrib

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
*/

package controller

import (
	"context"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TemplateResolver handles resolving WorkspaceTemplate references
// Note: This struct is retained for future use, though template defaulting
// now happens in webhooks. The workspace query functions have been moved
// to internal/workspace/queries.go for reuse across controllers and webhooks.
type TemplateResolver struct {
	client client.Client
}

// NewTemplateResolver creates a new TemplateResolver
func NewTemplateResolver(k8sClient client.Client) *TemplateResolver {
	return &TemplateResolver{
		client: k8sClient,
	}
}

// ListWorkspacesUsingTemplate returns all workspaces that reference the specified template
// This method delegates to the shared workspace query utilities
// Excludes workspaces that are being deleted (have deletionTimestamp set)
func (tr *TemplateResolver) ListWorkspacesUsingTemplate(ctx context.Context, templateName string) ([]workspacev1alpha1.Workspace, error) {
	// Delegate to shared workspace query function without pagination (for backward compatibility)
	workspaces, _, err := workspace.ListByTemplate(ctx, tr.client, templateName, "", 0)
	return workspaces, err
}
