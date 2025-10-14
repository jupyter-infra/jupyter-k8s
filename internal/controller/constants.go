package controller

import (
	"fmt"
	"time"
)

// Constants for resource configuration
const (
	// Default resource allocations
	DefaultCPURequest    = "100m"
	DefaultMemoryRequest = "128Mi"

	// Network configuration
	JupyterPort = 8888

	// Storage configuration
	DefaultMountPath = "/home/jovyan"

	// Label keys
	AppLabel = "app"

	// Access strategy label keys
	LabelWorkspaceName           = "workspaces.jupyter.org/workspaceName"
	LabelWorkspaceNamespace      = "workspaces.jupyter.org/workspaceNamespace"
	LabelAccessStrategyName      = "workspaces.jupyter.org/accessStrategyName"
	LabelAccessStrategyNamespace = "workspaces.jupyter.org/accessStrategyNamespace"

	// Label values
	AppLabelValue = "jupyter"

	// Status phases
	PhaseCreating = "Creating"
	PhaseRunning  = "Running"
	PhaseStopped  = "Stopped"
	// Status messages
	MessageCreating = "Jupyter server is starting"
	MessageRunning  = "Jupyter server is running"
	MessageStopped  = "Jupyter server stopped successfully"

	// Default desired status
	DefaultDesiredStatus = "Running"

	// Reconciliation timing
	PollRequeueDelay = 200 * time.Millisecond
	LongRequeueDelay = 60 * time.Second
)

// GenerateDeploymentName creates a consistent deployment name
func GenerateDeploymentName(workspaceName string) string {
	return fmt.Sprintf("jupyter-%s", workspaceName)
}

// GenerateServiceName creates a consistent service name
func GenerateServiceName(workspaceName string) string {
	return fmt.Sprintf("jupyter-%s-service", workspaceName)
}

// GeneratePVCName creates a consistent PVC name
func GeneratePVCName(workspaceName string) string {
	return fmt.Sprintf("jupyter-%s-pvc", workspaceName)
}

// GenerateLabels creates consistent labels for resources
func GenerateLabels(workspaceName string) map[string]string {
	return map[string]string{
		AppLabel:           AppLabelValue,
		LabelWorkspaceName: workspaceName,
	}
}
