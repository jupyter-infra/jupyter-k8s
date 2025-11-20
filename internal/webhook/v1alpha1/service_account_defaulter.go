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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	webhookconst "github.com/jupyter-infra/jupyter-k8s/internal/webhook"
)

// ServiceAccountDefaulter handles finding default service accounts
type ServiceAccountDefaulter struct {
	client client.Client
}

// NewServiceAccountDefaulter creates a new ServiceAccountDefaulter
func NewServiceAccountDefaulter(k8sClient client.Client) *ServiceAccountDefaulter {
	return &ServiceAccountDefaulter{
		client: k8sClient,
	}
}

// GetDefaultServiceAccount returns the default service account name for a namespace
func GetDefaultServiceAccount(ctx context.Context, k8sClient client.Client, namespace string) (string, error) {
	serviceAccounts := &corev1.ServiceAccountList{}
	if err := k8sClient.List(ctx, serviceAccounts, client.InNamespace(namespace), client.MatchingLabels{
		webhookconst.DefaultServiceAccountLabel: "true",
	}); err != nil {
		return "", fmt.Errorf("failed to list service accounts: %w", err)
	}

	switch len(serviceAccounts.Items) {
	case 0:
		return "default", nil
	case 1:
		return serviceAccounts.Items[0].Name, nil
	default:
		return "", fmt.Errorf("multiple service accounts found with default label in namespace %s", namespace)
	}
}

// ApplyServiceAccountDefaults applies default service account to workspace if not specified
func (sad *ServiceAccountDefaulter) ApplyServiceAccountDefaults(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	if workspace.Spec.ServiceAccountName != "" {
		return nil
	}

	defaultSA, err := GetDefaultServiceAccount(ctx, sad.client, workspace.GetNamespace())
	if err != nil {
		return err
	}

	workspace.Spec.ServiceAccountName = defaultSA
	return nil
}
