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

	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
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
