/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
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
