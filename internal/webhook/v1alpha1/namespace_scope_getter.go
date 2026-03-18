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

	webhookconst "github.com/jupyter-infra/jupyter-k8s/internal/webhook"
)

// GetTemplateScopeStrategy returns the template scope strategy for a namespace.
// Returns TemplateScopeCluster if the label is absent (backward compatible default).
// Returns an error for unrecognized label values.
func GetTemplateScopeStrategy(ctx context.Context, k8sClient client.Client, namespaceName string) (webhookconst.TemplateScopeStrategy, error) {
	ns := &corev1.Namespace{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Name: namespaceName}, ns); err != nil {
		return "", fmt.Errorf("failed to get namespace %s: %w", namespaceName, err)
	}

	value, exists := ns.Labels[webhookconst.TemplateScopeNamespaceLabel]
	if !exists || value == "" {
		return webhookconst.TemplateScopeCluster, nil
	}

	switch webhookconst.TemplateScopeStrategy(value) {
	case webhookconst.TemplateScopeNamespaced:
		return webhookconst.TemplateScopeNamespaced, nil
	case webhookconst.TemplateScopeCluster:
		return webhookconst.TemplateScopeCluster, nil
	default:
		return "", fmt.Errorf("unrecognized template-namespace-scope value %q on namespace %s: must be %q or %q",
			value, namespaceName, webhookconst.TemplateScopeNamespaced, webhookconst.TemplateScopeCluster)
	}
}
