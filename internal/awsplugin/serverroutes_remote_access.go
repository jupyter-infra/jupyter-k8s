/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package awsplugin

import (
	"context"
	"fmt"
	"os"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// PodEventsContext keys expected from the access strategy
const (
	SSMManagedNodeRoleKey = "ssmManagedNodeRole"
)

// AWSRemoteAccessRoutes implements pluginserver.RemoteAccessHandler using AWS SSM.
type AWSRemoteAccessRoutes struct {
	ssmClient   *SSMClient
	initializer *ResourceInitializer
}

// NewAWSRemoteAccessRoutes creates a new remote access handler backed by SSM.
func NewAWSRemoteAccessRoutes(ssmClient *SSMClient) *AWSRemoteAccessRoutes {
	return &AWSRemoteAccessRoutes{
		ssmClient: ssmClient,
		initializer: &ResourceInitializer{
			ssmClient: ssmClient,
		},
	}
}

// Initialize ensures required AWS resources (SSM documents) are created.
func (h *AWSRemoteAccessRoutes) Initialize(ctx context.Context, _ *pluginapi.InitializeRequest) (*pluginapi.InitializeResponse, error) {
	if err := h.initializer.EnsureResourcesInitialized(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize resources: %w", err)
	}
	return &pluginapi.InitializeResponse{}, nil
}

// RegisterNodeAgent creates an SSM activation for a workspace pod so the SSM agent
// sidecar can register as a managed instance.
func (h *AWSRemoteAccessRoutes) RegisterNodeAgent(ctx context.Context, req *pluginapi.RegisterNodeAgentRequest) (*pluginapi.RegisterNodeAgentResponse, error) {
	logger := log.FromContext(ctx).WithName("remote-access-handler")
	pctx := req.PodEventsContext

	// Extract IAM role from podEventsContext
	iamRole := pctx[SSMManagedNodeRoleKey]
	if iamRole == "" {
		return nil, fmt.Errorf("%s is required in podEventsContext", SSMManagedNodeRoleKey)
	}

	// Resolve overridable constants from podEventsContext
	managedByKey := SageMakerManagedByTagKey.ResolveStr(pctx)
	managedByVal := SageMakerManagedByTagValue.ResolveStr(pctx)
	clusterTagKey := SageMakerEKSClusterTagKey.ResolveStr(pctx)
	instancePrefix := SSMInstanceNamePrefix.ResolveStr(pctx)

	// Build tags for the SSM activation
	clusterARN := os.Getenv("CLUSTER_ID")
	tags := map[string]string{
		"workspace-pod-uid":   req.PodUID,
		"workspace-name":      req.WorkspaceName,
		"workspace-namespace": req.Namespace,
		managedByKey:          managedByVal,
		clusterTagKey:         clusterARN,
	}

	instanceName := fmt.Sprintf("%s-%s", instancePrefix, req.PodUID)
	description := fmt.Sprintf("Workspace %s/%s pod %s", req.Namespace, req.WorkspaceName, req.PodUID)

	logger.Info("Creating SSM activation for workspace pod",
		"workspaceName", req.WorkspaceName,
		"namespace", req.Namespace,
		"podUID", req.PodUID,
		"instanceName", instanceName)

	activation, err := h.ssmClient.CreateActivation(ctx, description, instanceName, iamRole, tags)
	if err != nil {
		return nil, fmt.Errorf("failed to create SSM activation: %w", err)
	}

	return &pluginapi.RegisterNodeAgentResponse{
		ActivationID:   activation.ActivationId,
		ActivationCode: activation.ActivationCode,
	}, nil
}

// DeregisterNodeAgent cleans up SSM managed instances and activations for a deleted pod.
func (h *AWSRemoteAccessRoutes) DeregisterNodeAgent(ctx context.Context, req *pluginapi.DeregisterNodeAgentRequest) (*pluginapi.DeregisterNodeAgentResponse, error) {
	logger := log.FromContext(ctx).WithName("remote-access-handler")
	logger.Info("Deregistering node agent", "podUID", req.PodUID)

	// DeregisterNodeAgentRequest has no context map, use defaults
	podUIDTagKey := WorkspacePodUIDTagKey.Default
	instancePrefix := SSMInstanceNamePrefix.Default

	// Clean up managed instances
	if err := h.ssmClient.CleanupManagedInstancesByPodUID(ctx, req.PodUID, podUIDTagKey); err != nil {
		logger.Error(err, "Failed to cleanup managed instances", "podUID", req.PodUID)
		// Continue to activation cleanup even if instance cleanup fails
	}

	// Clean up activations
	if err := h.ssmClient.CleanupActivationsByPodUID(ctx, req.PodUID, instancePrefix); err != nil {
		logger.Error(err, "Failed to cleanup activations", "podUID", req.PodUID)
	}

	return &pluginapi.DeregisterNodeAgentResponse{}, nil
}

// CreateSession creates an SSM session for VSCode remote connection.
func (h *AWSRemoteAccessRoutes) CreateSession(ctx context.Context, req *pluginapi.CreateSessionRequest) (*pluginapi.CreateSessionResponse, error) {
	logger := log.FromContext(ctx).WithName("remote-access-handler")
	cctx := req.ConnectionContext

	// Resolve overridable constants from connectionContext
	podUIDTagKey := WorkspacePodUIDTagKey.ResolveStr(cctx)
	vscodeScheme := VSCodeScheme.ResolveStr(cctx)
	maxSessions := MaxConcurrentSSMSessions.ResolveInt32(cctx)

	// Find the managed instance by pod UID
	instanceID, err := h.ssmClient.FindInstanceByPodUID(ctx, req.PodUID, podUIDTagKey)
	if err != nil {
		logger.Error(err, "Failed to find managed instance", "podUID", req.PodUID)
		return nil, fmt.Errorf("failed to find managed instance for pod %s: %w", req.PodUID, err)
	}

	// Get SSM document name from connection context
	documentName := cctx["ssmDocumentName"]
	if documentName == "" {
		return nil, fmt.Errorf("ssmDocumentName is required in connectionContext")
	}

	// Start SSM session
	port := "22" // default SSH port
	if p := cctx["port"]; p != "" {
		port = p
	}

	sessionInfo, err := h.ssmClient.StartSession(ctx, instanceID, documentName, port, maxSessions)
	if err != nil {
		return nil, fmt.Errorf("failed to start SSM session: %w", err)
	}

	// Build VSCode connection URL
	clusterARN := os.Getenv("CLUSTER_ID")
	connectionURL := fmt.Sprintf("%s?sessionId=%s&sessionToken=%s&streamUrl=%s&workspaceName=%s&namespace=%s&eksClusterArn=%s",
		vscodeScheme,
		sessionInfo.SessionID,
		sessionInfo.TokenValue,
		sessionInfo.StreamURL,
		req.WorkspaceName,
		req.Namespace,
		clusterARN,
	)

	logger.Info("Created SSM session for workspace",
		"workspaceName", req.WorkspaceName,
		"workspaceNamespace", req.Namespace,
		"instanceID", instanceID,
		"sessionID", sessionInfo.SessionID)

	return &pluginapi.CreateSessionResponse{
		ConnectionURL: connectionURL,
	}, nil
}
