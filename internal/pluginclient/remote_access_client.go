/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package pluginclient

import (
	"context"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
)

// PluginRemoteAccessClient delegates remote access operations to a plugin sidecar over HTTP.
// It handles only pure cloud SDK operations (register, deregister, create-session).
// The controller owns k8s orchestration (pod exec, state files, restart detection) and
// calls these methods where it previously called the AWS SDK directly.
type PluginRemoteAccessClient struct {
	client *PluginClient
}

// NewPluginRemoteAccessClient creates a new PluginRemoteAccessClient backed by the given shared client.
func NewPluginRemoteAccessClient(client *PluginClient) *PluginRemoteAccessClient {
	return &PluginRemoteAccessClient{client: client}
}

// Initialize delegates one-time resource creation to the plugin via POST /v1alpha1/remote-access/initialize.
func (s *PluginRemoteAccessClient) Initialize(ctx context.Context) error {
	_, _, err := doPost[pluginapi.InitializeResponse](ctx, s.client, pluginapi.RouteRemoteAccessInit.Path, &pluginapi.InitializeRequest{})
	return err
}

// RegisterNodeAgent calls the plugin to create an activation and returns the credentials.
// The controller uses these to exec the registration script in the pod.
func (s *PluginRemoteAccessClient) RegisterNodeAgent(ctx context.Context, req *pluginapi.RegisterNodeAgentRequest) (*pluginapi.RegisterNodeAgentResponse, error) {
	resp, _, err := doPost[pluginapi.RegisterNodeAgentResponse](ctx, s.client, pluginapi.RouteRegisterNodeAgent.Path, req)
	return resp, err
}

// DeregisterNodeAgent delegates cleanup to the plugin via POST /v1alpha1/remote-access/deregister-node-agent.
func (s *PluginRemoteAccessClient) DeregisterNodeAgent(ctx context.Context, pod *corev1.Pod) error {
	req := &pluginapi.DeregisterNodeAgentRequest{
		PodUID: string(pod.UID),
	}
	_, _, err := doPost[pluginapi.DeregisterNodeAgentResponse](ctx, s.client, pluginapi.RouteDeregisterNodeAgent.Path, req)
	return err
}

// CreateSession delegates session creation to the plugin via POST /v1alpha1/remote-access/create-session.
func (s *PluginRemoteAccessClient) CreateSession(ctx context.Context, workspaceName string, namespace string, podUID string, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (string, error) {
	var connectionContext map[string]string
	if accessStrategy != nil {
		connectionContext = accessStrategy.Spec.CreateConnectionContext
	}
	req := &pluginapi.CreateSessionRequest{
		PodUID:            podUID,
		WorkspaceName:     workspaceName,
		Namespace:         namespace,
		ConnectionContext: connectionContext,
	}
	resp, _, err := doPost[pluginapi.CreateSessionResponse](ctx, s.client, pluginapi.RouteCreateSession.Path, req)
	if err != nil {
		return "", err
	}
	return resp.ConnectionURL, nil
}
