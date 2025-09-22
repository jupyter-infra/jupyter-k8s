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
	AppLabel           = "app"
	JupyterServerLabel = "jupyterserver.servers.jupyter.org/name"

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
func GenerateDeploymentName(jupyterServerName string) string {
	return fmt.Sprintf("jupyter-%s", jupyterServerName)
}

// GenerateServiceName creates a consistent service name
func GenerateServiceName(jupyterServerName string) string {
	return fmt.Sprintf("jupyter-%s-service", jupyterServerName)
}

// GeneratePVCName creates a consistent PVC name
func GeneratePVCName(jupyterServerName string) string {
	return fmt.Sprintf("jupyter-%s-pvc", jupyterServerName)
}

// GenerateLabels creates consistent labels for resources
func GenerateLabels(jupyterServerName string) map[string]string {
	return map[string]string{
		AppLabel:           AppLabelValue,
		JupyterServerLabel: jupyterServerName,
	}
}
