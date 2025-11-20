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

package extensionapi

import (
	"context"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// getWorkspace fetches a workspace from Kubernetes
func (s *ExtensionServer) getWorkspace(namespace, workspaceName string) (*workspacev1alpha1.Workspace, error) {
	var workspace workspacev1alpha1.Workspace
	err := s.k8sClient.Get(context.Background(),
		client.ObjectKey{Namespace: namespace, Name: workspaceName},
		&workspace)
	if err != nil {
		return nil, err
	}
	return &workspace, nil
}

// getAccessStrategy fetches the AccessStrategy for the workspace
func (s *ExtensionServer) getAccessStrategy(workspace *workspacev1alpha1.Workspace) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
	if workspace.Spec.AccessStrategy == nil {
		return nil, nil // No AccessStrategy configured
	}

	// Determine namespace for the AccessStrategy (same logic as controller)
	accessStrategyNamespace := workspace.Namespace
	if workspace.Spec.AccessStrategy.Namespace != "" {
		accessStrategyNamespace = workspace.Spec.AccessStrategy.Namespace
	}

	var accessStrategy workspacev1alpha1.WorkspaceAccessStrategy
	err := s.k8sClient.Get(context.Background(),
		client.ObjectKey{
			Name:      workspace.Spec.AccessStrategy.Name,
			Namespace: accessStrategyNamespace,
		},
		&accessStrategy)
	if err != nil {
		return nil, err
	}
	return &accessStrategy, nil
}
