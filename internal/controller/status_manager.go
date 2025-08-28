package controller

import (
	"context"
	"fmt"

	"github.com/jupyter-k8s/jupyter-k8s/api/v1alpha1"

	appsv1 "k8s.io/api/apps/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// StatusManager handles JupyterServer status updates
type StatusManager struct {
	client client.Client
}

// NewStatusManager creates a new StatusManager
func NewStatusManager(client client.Client) *StatusManager {
	return &StatusManager{
		client: client,
	}
}

// UpdateStatus updates the JupyterServer status
func (sm *StatusManager) UpdateStatus(ctx context.Context, jupyterServer *v1alpha1.JupyterServer, phase, message string) error {
	logger := log.FromContext(ctx)
	
	// Only update if status actually changed
	if jupyterServer.Status.Phase == phase && jupyterServer.Status.Message == message {
		return nil
	}
	
	jupyterServer.Status.Phase = phase
	jupyterServer.Status.Message = message
	
	if err := sm.client.Status().Update(ctx, jupyterServer); err != nil {
		logger.Error(err, "Failed to update JupyterServer status", 
			"phase", phase, 
			"message", message)
		return fmt.Errorf("failed to update status: %w", err)
	}
	
	logger.Info("Updated JupyterServer status", 
		"phase", phase, 
		"message", message)
	return nil
}

// UpdateCreatingStatus sets status to Creating
func (sm *StatusManager) UpdateCreatingStatus(ctx context.Context, jupyterServer *v1alpha1.JupyterServer, deploymentName, serviceName string) error {
	jupyterServer.Status.DeploymentName = deploymentName
	jupyterServer.Status.ServiceName = serviceName
	return sm.UpdateStatus(ctx, jupyterServer, PhaseCreating, MessageCreating)
}

// UpdateRunningStatus sets status to Running
func (sm *StatusManager) UpdateRunningStatus(ctx context.Context, jupyterServer *v1alpha1.JupyterServer) error {
	return sm.UpdateStatus(ctx, jupyterServer, PhaseRunning, MessageRunning)
}

// UpdateStoppedStatus sets status to Stopped
func (sm *StatusManager) UpdateStoppedStatus(ctx context.Context, jupyterServer *v1alpha1.JupyterServer) error {
	return sm.UpdateStatus(ctx, jupyterServer, PhaseStopped, MessageStopped)
}

// ShouldUpdateToRunning checks if the status should be updated to Running based on deployment status
func (sm *StatusManager) ShouldUpdateToRunning(jupyterServer *v1alpha1.JupyterServer, deployment *appsv1.Deployment) bool {
	return deployment.Status.ReadyReplicas > 0 && jupyterServer.Status.Phase != PhaseRunning
}
