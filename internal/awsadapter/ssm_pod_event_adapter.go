/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package awsadapter

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	pluginapi "github.com/jupyter-infra/jupyter-k8s/api/plugin/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/plugin"
	"github.com/jupyter-infra/jupyter-k8s/internal/pluginadapters"
	corev1 "k8s.io/api/core/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// RegistrationState tracks SSM registration state across container restarts
type RegistrationState struct {
	SidecarRestartCount   int32     `json:"sidecarRestartCount"`
	WorkspaceRestartCount int32     `json:"workspaceRestartCount"`
	SetupInProgress       bool      `json:"setupInProgress,omitempty"`
	SetupStartedAt        time.Time `json:"setupStartedAt,omitempty"`
}

// AwsSsmPodEventAdapter handles SSM-based pod lifecycle events.
// It owns Kubernetes-side orchestration (pod exec, state file I/O, restart detection)
// and delegates pure cloud SDK operations to a plugin sidecar via plugin.RemoteAccessPluginApis.
// All configuration (container names, script paths, etc.) comes from the podEventsContext map.
type AwsSsmPodEventAdapter struct {
	pluginClient plugin.RemoteAccessPluginApis
	podExecUtil  pluginadapters.PodExecInterface
}

// NewAwsSsmPodEventAdapter creates a new AwsSsmPodEventAdapter,
// which implements pluginadapters.PodEventPluginAdapter for SSM-based remote access.
// pluginClient handles cloud SDK operations (register, deregister).
// podExecUtil handles pod exec operations (state files, registration scripts).
func NewAwsSsmPodEventAdapter(pluginClient plugin.RemoteAccessPluginApis, podExecUtil pluginadapters.PodExecInterface) (*AwsSsmPodEventAdapter, error) {
	if podExecUtil == nil {
		return nil, fmt.Errorf("podExecUtil is required")
	}

	return &AwsSsmPodEventAdapter{
		pluginClient: pluginClient,
		podExecUtil:  podExecUtil,
	}, nil
}

// HandlePodRunning initializes SSM agent and remote access server for workspace containers.
// Handles both first-time setup and recovery from container restarts.
//
// Container Restart Detection:
// - Uses persistent state file (path from podEventsContext["registrationStateFile"])
// - Compares stored restart counts with current pod status to detect restarts
// - Selectively re-setups only affected containers (sidecar and/or workspace)
//
// Concurrency Protection:
// - Random delay (0-2s) spreads out concurrent pod events
// - SetupInProgress flag prevents duplicate setup attempts
func (s *AwsSsmPodEventAdapter) HandlePodRunning(ctx context.Context, pod *corev1.Pod, workspaceName, namespace string, podEventsContext map[string]string) error {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "namespace", namespace, "workspace", workspaceName)

	if s.pluginClient == nil {
		return fmt.Errorf("plugin client not available")
	}

	// Propagate a request ID so plugin logs can be correlated with this pod event
	ctx = plugin.ContextWithOriginRequestID(ctx, plugin.GenerateRequestID())

	sidecarContainer := ContextKeySidecarContainerName.ResolveStr(podEventsContext)

	// Early exit if sidecar not running yet
	if !pluginadapters.IsContainerRunning(pod, sidecarContainer) {
		logger.V(1).Info("Sidecar container not running yet, waiting")
		return nil
	}

	// Random delay to spread out concurrent events
	delay := time.Duration(rand.Intn(2000)) * time.Millisecond
	logger.V(1).Info("Adding random delay before setup", "delay", delay)
	time.Sleep(delay)

	// Get current restart counts
	workspaceContainer := ContextKeyWorkspaceContainerName.ResolveStr(podEventsContext)
	currentSidecarRestarts := pluginadapters.GetContainerRestartCount(pod, sidecarContainer)
	currentWorkspaceRestarts := pluginadapters.GetContainerRestartCount(pod, workspaceContainer)
	logger.V(1).Info("Current restart counts",
		"sidecar", currentSidecarRestarts,
		"workspace", currentWorkspaceRestarts)

	stateFile := ContextKeyRegistrationStateFile.ResolveStr(podEventsContext)

	// Read state from shared volume
	state, err := s.readRegistrationState(ctx, pod, sidecarContainer, stateFile)

	// Check if setup is already in progress
	if state != nil && state.SetupInProgress {
		logger.V(1).Info("Setup already in progress by another event, skipping")
		return nil
	}

	// Determine what needs to be setup
	var needSidecarSetup, needWorkspaceSetup, needCleanup bool

	if state == nil {
		if err != nil {
			// File exists but is corrupted - need cleanup
			logger.Info("State file was corrupted, will cleanup before setup")
			needCleanup = true
		} else {
			// File doesn't exist - first time setup, no cleanup needed
			logger.V(1).Info("No state file found, first time setup")
			needCleanup = false
		}
		needSidecarSetup = true
		needWorkspaceSetup = true
	} else {
		// Check for restarts
		sidecarRestarted := currentSidecarRestarts > state.SidecarRestartCount
		workspaceRestarted := currentWorkspaceRestarts > state.WorkspaceRestartCount

		if !sidecarRestarted && !workspaceRestarted {
			logger.V(1).Info("No restarts detected, already setup")
			return nil
		}

		logger.Info("Container restart detected",
			"sidecarRestarted", sidecarRestarted,
			"workspaceRestarted", workspaceRestarted)

		needSidecarSetup = sidecarRestarted
		needWorkspaceSetup = workspaceRestarted
		needCleanup = sidecarRestarted // Only cleanup if sidecar restarted
	}

	// Mark setup as in progress
	inProgressState := &RegistrationState{
		SidecarRestartCount:   currentSidecarRestarts,
		WorkspaceRestartCount: currentWorkspaceRestarts,
		SetupInProgress:       true,
		SetupStartedAt:        time.Now(),
	}
	if err := s.writeRegistrationState(ctx, pod, sidecarContainer, stateFile, inProgressState); err != nil {
		logger.Error(err, "Failed to write in-progress state, continuing anyway")
	} else {
		logger.V(1).Info("Marked setup as in progress")
	}

	// Setup containers as needed
	var setupErr error
	if needSidecarSetup {
		logger.V(1).Info("Setting up sidecar container")
		if err := s.setupSidecarContainer(ctx, pod, workspaceName, namespace, podEventsContext, needCleanup); err != nil {
			setupErr = err
		}
	}

	if setupErr == nil && needWorkspaceSetup {
		logger.V(1).Info("Setting up workspace container")
		if err := s.setupWorkspaceContainer(ctx, pod, podEventsContext); err != nil {
			setupErr = err
		}
	}

	// Mark setup as complete (or failed)
	finalState := &RegistrationState{
		SidecarRestartCount:   currentSidecarRestarts,
		WorkspaceRestartCount: currentWorkspaceRestarts,
		SetupInProgress:       false, // Clear the flag
	}
	if err := s.writeRegistrationState(ctx, pod, sidecarContainer, stateFile, finalState); err != nil {
		logger.Error(err, "Failed to write final state")
	} else {
		logger.V(1).Info("Marked setup as complete")
	}

	if setupErr != nil {
		return setupErr
	}

	logger.Info("Container setup completed")
	return nil
}

// HandlePodDeleted delegates cleanup to the plugin sidecar
func (s *AwsSsmPodEventAdapter) HandlePodDeleted(ctx context.Context, pod *corev1.Pod, _ map[string]string) error {
	if s.pluginClient == nil {
		return fmt.Errorf("plugin client not available")
	}

	ctx = plugin.ContextWithOriginRequestID(ctx, plugin.GenerateRequestID())
	_, err := s.pluginClient.DeregisterNodeAgent(ctx, &pluginapi.DeregisterNodeAgentRequest{
		PodUID: string(pod.UID),
	})
	return err
}

// readRegistrationState reads state from shared volume
// Returns:
//   - (*RegistrationState, nil) if state file exists and is valid
//   - (nil, nil) if state file doesn't exist (first time setup)
//   - (nil, error) if state file exists but is corrupted
func (s *AwsSsmPodEventAdapter) readRegistrationState(ctx context.Context, pod *corev1.Pod, sidecarContainer, stateFile string) (*RegistrationState, error) {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "namespace", pod.Namespace)

	cmd := []string{"cat", stateFile}
	output, err := s.podExecUtil.ExecInPod(ctx, pod, sidecarContainer, cmd, "")
	if err != nil {
		// File doesn't exist - this is first time setup
		logger.V(1).Info("State file not found, treating as first registration")
		return nil, nil
	}

	var state RegistrationState
	if err := json.Unmarshal([]byte(output), &state); err != nil {
		// File exists but is corrupted - need cleanup
		logger.Error(err, "Failed to parse state file, corrupted state detected")
		return nil, err
	}

	logger.V(1).Info("Read registration state",
		"sidecarRestartCount", state.SidecarRestartCount,
		"workspaceRestartCount", state.WorkspaceRestartCount)
	return &state, nil
}

// writeRegistrationState writes state to shared volume
func (s *AwsSsmPodEventAdapter) writeRegistrationState(ctx context.Context, pod *corev1.Pod, sidecarContainer, stateFile string, state *RegistrationState) error {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "namespace", pod.Namespace)

	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	// Write to temp file then move (atomic operation)
	cmd := []string{"bash", "-c", fmt.Sprintf("echo '%s' > %s.tmp && mv %s.tmp %s",
		string(stateJSON), stateFile, stateFile, stateFile)}

	if _, err := s.podExecUtil.ExecInPod(ctx, pod, sidecarContainer, cmd, ""); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	logger.V(1).Info("Wrote registration state",
		"sidecarRestartCount", state.SidecarRestartCount,
		"workspaceRestartCount", state.WorkspaceRestartCount)
	return nil
}

// setupSidecarContainer handles SSM agent registration via the plugin sidecar
func (s *AwsSsmPodEventAdapter) setupSidecarContainer(ctx context.Context, pod *corev1.Pod, workspaceName, namespace string, podEventsContext map[string]string, cleanupNeeded bool) error {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "namespace", namespace, "workspace", workspaceName)
	noStdin := ""
	sidecarContainer := ContextKeySidecarContainerName.ResolveStr(podEventsContext)

	// Cleanup old resources if needed
	if cleanupNeeded {
		logger.V(1).Info("Cleaning up old SSM resources before registration")
		if _, err := s.pluginClient.DeregisterNodeAgent(ctx, &pluginapi.DeregisterNodeAgentRequest{
			PodUID: string(pod.UID),
		}); err != nil {
			logger.Error(err, "Failed to cleanup old SSM resources, continuing with registration anyway")
		}
	}

	// Register node agent via plugin
	logger.V(1).Info("Registering node agent via plugin")
	resp, err := s.registerNodeAgent(ctx, pod, workspaceName, namespace, podEventsContext)
	if err != nil {
		return fmt.Errorf("failed to register node agent: %w", err)
	}
	logger.Info("Node agent registered", "activationId", resp.ActivationID)

	// Run registration script
	registrationScript := ContextKeyRegistrationScript.ResolveStr(podEventsContext)
	logger.V(1).Info("Running SSM registration script in sidecar")
	// Use stdin to pass only sensitive values securely
	region := ContextKeyRegion.ResolveStr(podEventsContext)
	cmd := []string{"bash", "-c", fmt.Sprintf("read ACTIVATION_ID && read ACTIVATION_CODE && env ACTIVATION_ID=\"$ACTIVATION_ID\" ACTIVATION_CODE=\"$ACTIVATION_CODE\" REGION=%s %s", region, registrationScript)}
	stdinData := fmt.Sprintf("%s\n%s\n", resp.ActivationID, resp.ActivationCode)

	if _, err := s.podExecUtil.ExecInPod(ctx, pod, sidecarContainer, cmd, stdinData); err != nil {
		return fmt.Errorf("failed to execute SSM registration script: %w", err)
	}
	logger.Info("SSM registration completed successfully")

	// TODO: Remove this once all deployments migrate to state-file-based readiness checks
	markerFile := ContextKeyRegistrationMarkerFile.ResolveStr(podEventsContext)
	logger.V(1).Info("Creating marker file for backward compatibility")
	markerCmd := []string{"touch", markerFile}
	if _, err := s.podExecUtil.ExecInPod(ctx, pod, sidecarContainer, markerCmd, noStdin); err != nil {
		return fmt.Errorf("failed to create marker file: %w", err)
	}

	logger.Info("Sidecar container setup completed")
	return nil
}

// setupWorkspaceContainer starts the remote access server
func (s *AwsSsmPodEventAdapter) setupWorkspaceContainer(ctx context.Context, pod *corev1.Pod, podEventsContext map[string]string) error {
	logger := logf.FromContext(ctx).WithValues("pod", pod.Name, "namespace", pod.Namespace)

	workspaceContainer := ContextKeyWorkspaceContainerName.ResolveStr(podEventsContext)

	// Check workspace is running
	if !pluginadapters.IsContainerRunning(pod, workspaceContainer) {
		logger.V(1).Info("Workspace container not running yet, waiting")
		return nil
	}

	remoteAccessScript := ContextKeyRemoteAccessServerScript.ResolveStr(podEventsContext)
	remoteAccessPort := ContextKeyRemoteAccessServerPort.ResolveStr(podEventsContext)

	logger.Info("Starting remote access server in main container")
	serverCmd := []string{remoteAccessScript, "--port", remoteAccessPort}
	if _, err := s.podExecUtil.ExecInPod(ctx, pod, workspaceContainer, serverCmd, ""); err != nil {
		return fmt.Errorf("failed to start remote access server: %w", err)
	}

	logger.Info("Workspace container setup completed")
	return nil
}

// registerNodeAgent delegates activation creation to the plugin sidecar
func (s *AwsSsmPodEventAdapter) registerNodeAgent(ctx context.Context, pod *corev1.Pod, workspaceName, namespace string, podEventsContext map[string]string) (*pluginapi.RegisterNodeAgentResponse, error) {
	req := &pluginapi.RegisterNodeAgentRequest{
		PodUID:           string(pod.UID),
		WorkspaceName:    workspaceName,
		Namespace:        namespace,
		PodEventsContext: podEventsContext,
	}

	return s.pluginClient.RegisterNodeAgent(ctx, req)
}
