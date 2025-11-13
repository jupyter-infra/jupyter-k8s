package controller

import (
	"fmt"
	"time"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-ai-contrib/jupyter-k8s/internal/workspace"
)

const (
	// DefaultCPURequest is the default CPU request for workspace containers
	DefaultCPURequest = "100m"
	// DefaultMemoryRequest is the default memory request for workspace containers
	DefaultMemoryRequest = "128Mi"

	// JupyterPort is the default port for Jupyter server
	JupyterPort = 8888

	// DefaultMountPath is the default mount path for workspace storage
	DefaultMountPath = "/home/jovyan"

	// AppLabel is the label key for application identification
	AppLabel = "app"

	// LabelWorkspaceName is the label key for workspace name
	LabelWorkspaceName = "workspace.jupyter.org/workspace-name"
	// LabelWorkspaceNamespace is the label key for workspace namespace
	LabelWorkspaceNamespace = "workspace.jupyter.org/workspace-namespace"
	// LabelAccessStrategyName is the label key for access strategy name
	LabelAccessStrategyName = "workspace.jupyter.org/access-strategy-name"
	// LabelAccessStrategyNamespace is the label key for access strategy namespace
	LabelAccessStrategyNamespace = "workspace.jupyter.org/access-strategy-namespace"
	// LabelWorkspaceTemplate is the label key for workspace template name
	LabelWorkspaceTemplate = "workspace.jupyter.org/template"
	// LabelWorkspaceTemplateNamespace is the label key for workspace template namespace
	LabelWorkspaceTemplateNamespace = "workspace.jupyter.org/template-namespace"

	// AppLabelValue is the label value for app label
	AppLabelValue = "jupyter"

	// Kubernetes recommended labels
	// See: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
	KubernetesAppNameLabel      = "app.kubernetes.io/name"
	KubernetesAppInstanceLabel  = "app.kubernetes.io/instance"
	KubernetesAppVersionLabel   = "app.kubernetes.io/version"
	KubernetesAppComponentLabel = "app.kubernetes.io/component"
	KubernetesAppPartOfLabel    = "app.kubernetes.io/part-of"
	KubernetesAppManagedByLabel = "app.kubernetes.io/managed-by"

	// Kubernetes recommended label values
	KubernetesAppNameValue      = "jupyter"
	KubernetesAppComponentValue = "workspace"
	KubernetesAppPartOfValue    = "jupyter-k8s"
	KubernetesAppManagedByValue = "jupyter-k8s-operator"

	// Version should be passed from build/schema
	KubernetesAppVersionValue = workspacev1alpha1.GroupVersion.Version

	// AnnotationCreatedBy is the annotation key for tracking resource creator
	AnnotationCreatedBy = "workspace.jupyter.org/created-by"
	// AnnotationLastUpdatedBy is the annotation key for tracking last updater
	AnnotationLastUpdatedBy = "workspace.jupyter.org/last-updated-by"
	// AnnotationServiceAccountUsers is the annotation key for service account users
	AnnotationServiceAccountUsers = "workspace.jupyter.org/service-account-users"
	// AnnotationServiceAccountUserPatterns is the annotation key for service account user patterns
	AnnotationServiceAccountUserPatterns = "workspace.jupyter.org/service-account-user-patterns"
	// AnnotationServiceAccountGroups is the annotation key for service account groups
	AnnotationServiceAccountGroups = "workspace.jupyter.org/service-account-groups"

	// PhaseCreating indicates the workspace is being created
	PhaseCreating = "Creating"
	// PhaseRunning indicates the workspace is running
	PhaseRunning = "Running"
	// PhaseStopped indicates the workspace is stopped
	PhaseStopped = "Stopped"

	// PreemptedReason is the reason for preempted workspaces
	PreemptedReason = "Workspace preempted due to resource contention"

	// PreemptionReasonAnnotation is the annotation key for preemption reason
	PreemptionReasonAnnotation = "workspace.jupyter.org/preemption-reason"

	// KindPod represents the Pod resource kind
	KindPod = "Pod"

	// MessageCreating is the status message for creating workspaces
	MessageCreating = "Jupyter server is starting"
	// MessageRunning is the status message for running workspaces
	MessageRunning = "Jupyter server is running"
	// MessageStopped is the status message for stopped workspaces
	MessageStopped = "Jupyter server stopped successfully"

	// DefaultDesiredStatus is the default desired status for workspaces
	DefaultDesiredStatus = "Running"

	// PollRequeueDelay is the delay for polling reconciliation
	PollRequeueDelay = 200 * time.Millisecond
	// LongRequeueDelay is the delay for long reconciliation cycles
	LongRequeueDelay = 60 * time.Second

	// IdleCheckInterval is the interval for checking workspace idle status
	IdleCheckInterval = 5 * time.Minute

	// WorkspaceFinalizerName is the finalizer name for workspace cleanup protection
	WorkspaceFinalizerName = "workspace.jupyter.org/cleanup-protection"

	// ControllerPodNamespaceEnv is the environment variable for the controller pod namespace
	ControllerPodNamespaceEnv = "CONTROLLER_POD_NAMESPACE"

	// ControllerPodServiceAccountEnv is the environment variable for the controller pod service account
	ControllerPodServiceAccountEnv = "CONTROLLER_POD_SERVICE_ACCOUNT"
)

// GenerateDeploymentName creates a consistent deployment name
func GenerateDeploymentName(workspaceName string) string {
	return fmt.Sprintf("workspace-%s", workspaceName)
}

// GenerateServiceName creates a consistent service name
func GenerateServiceName(workspaceName string) string {
	return fmt.Sprintf("workspace-%s-service", workspaceName)
}

// GeneratePVCName creates a consistent PVC name
func GeneratePVCName(workspaceName string) string {
	return fmt.Sprintf("workspace-%s-pvc", workspaceName)
}

// GenerateLabels creates consistent labels for resources with Kubernetes recommended labels
func GenerateLabels(workspaceName string) map[string]string {
	labels := map[string]string{
		// Existing custom labels
		AppLabel:                         AppLabelValue,
		workspaceutil.LabelWorkspaceName: workspaceName,
	}

	// Add Kubernetes recommended labels
	for k, v := range GenerateKubernetesRecommendedLabels(workspaceName) {
		labels[k] = v
	}

	return labels
}

// GenerateKubernetesRecommendedLabels creates Kubernetes recommended labels
// See: https://kubernetes.io/docs/concepts/overview/working-with-objects/common-labels/
func GenerateKubernetesRecommendedLabels(workspaceName string) map[string]string {
	return map[string]string{
		KubernetesAppNameLabel:      KubernetesAppNameValue,
		KubernetesAppInstanceLabel:  workspaceName,
		KubernetesAppVersionLabel:   KubernetesAppVersionValue,
		KubernetesAppComponentLabel: KubernetesAppComponentValue,
		KubernetesAppPartOfLabel:    KubernetesAppPartOfValue,
		KubernetesAppManagedByLabel: KubernetesAppManagedByValue,
	}
}
