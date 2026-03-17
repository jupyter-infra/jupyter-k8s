/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient

import (
	"context"

	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

// PluginRemoteAccessClient implements the controller's SSMRemoteAccessStrategyInterface
// by delegating cloud SDK operations to a plugin sidecar over HTTP.
type PluginRemoteAccessClient struct {
	client *PluginClient
}

// NewPluginRemoteAccessClient creates a new PluginRemoteAccessClient for the given plugin endpoint.
func NewPluginRemoteAccessClient(baseURL string) *PluginRemoteAccessClient {
	return &PluginRemoteAccessClient{
		client: NewPluginClient(baseURL),
	}
}

// SetupContainers delegates pod setup to the plugin via POST /v1alpha1/remote-access/register-node.
func (s *PluginRemoteAccessClient) SetupContainers(_ context.Context, _ *corev1.Pod, _ *workspacev1alpha1.Workspace, _ *workspacev1alpha1.WorkspaceAccessStrategy) error {
	return ErrNotImplemented
}

// DeregisterNode delegates cleanup to the plugin via POST /v1alpha1/remote-access/deregister-node.
func (s *PluginRemoteAccessClient) DeregisterNode(_ context.Context, _ *corev1.Pod) error {
	return ErrNotImplemented
}

// Initialize delegates one-time resource creation to the plugin via POST /v1alpha1/remote-access/initialize.
func (s *PluginRemoteAccessClient) Initialize(_ context.Context) error {
	return ErrNotImplemented
}

// CreateSession delegates session creation to the plugin via POST /v1alpha1/remote-access/create-session.
func (s *PluginRemoteAccessClient) CreateSession(_ context.Context, _ string, _ string, _ string, _ *workspacev1alpha1.WorkspaceAccessStrategy) (string, error) {
	return "", ErrNotImplemented
}
