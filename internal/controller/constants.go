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
	LabelWorkspaceName           = "workspace.jupyter.org/workspaceName"
	LabelWorkspaceNamespace      = "workspace.jupyter.org/workspaceNamespace"
	LabelAccessStrategyName      = "workspace.jupyter.org/accessStrategyName"
	LabelAccessStrategyNamespace = "workspace.jupyter.org/accessStrategyNamespace"

	// Label values
	AppLabelValue = "jupyter"

	// Annotation keys
	AnnotationCreatedBy     = "workspace.jupyter.org/created-by"
	AnnotationLastUpdatedBy = "workspace.jupyter.org/last-updated-by"

	// Status phases
	PhaseCreating = "Creating"
	PhaseRunning  = "Running"
	PhaseStopped  = "Stopped"

	// Preemption annotation value
	PreemptedReason = "Workspace preempted due to resource contention"

	// Annotation keys
	PreemptionReasonAnnotation = "workspace.jupyter.org/preemption-reason"

	// Kubernetes resource kinds
	KindPod = "Pod"
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
