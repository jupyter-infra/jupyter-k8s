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
	LabelWorkspaceTemplate = "workspace.jupyter.org/template-name"
	// LabelWorkspaceTemplateNamespace is the label key for workspace template namespace
	LabelWorkspaceTemplateNamespace = "workspace.jupyter.org/template-namespace"

	// LabelComponent is the label key for component identification
	LabelComponent = "workspace.jupyter.org/component"

	// AppLabelValue is the label value for app label
	AppLabelValue = "jupyter"

	// KubernetesAppNameLabel is the Kubernetes recommended label for application name
	KubernetesAppNameLabel = "app.kubernetes.io/name"
	// KubernetesAppInstanceLabel is the Kubernetes recommended label for application instance
	KubernetesAppInstanceLabel = "app.kubernetes.io/instance"
	// KubernetesAppVersionLabel is the Kubernetes recommended label for application version
	KubernetesAppVersionLabel = "app.kubernetes.io/version"
	// KubernetesAppComponentLabel is the Kubernetes recommended label for application component
	KubernetesAppComponentLabel = "app.kubernetes.io/component"
	// KubernetesAppPartOfLabel is the Kubernetes recommended label for application part-of
	KubernetesAppPartOfLabel = "app.kubernetes.io/part-of"
	// KubernetesAppManagedByLabel is the Kubernetes recommended label for application managed-by
	KubernetesAppManagedByLabel = "app.kubernetes.io/managed-by"

	// KubernetesAppNameValue is the value for the Kubernetes app name label
	KubernetesAppNameValue = "jupyter"
	// KubernetesAppComponentValue is the value for the Kubernetes app component label
	KubernetesAppComponentValue = "workspace"
	// KubernetesAppPartOfValue is the value for the Kubernetes app part-of label
	KubernetesAppPartOfValue = "jupyter-k8s"
	// KubernetesAppManagedByValue is the value for the Kubernetes app managed-by label
	KubernetesAppManagedByValue = "jupyter-k8s"

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

	// DesiredStateRunning indicates the workspace is running
	DesiredStateRunning = "Running"
	// DesiredStateStopped indicates the workspace is stopped
	DesiredStateStopped = "Stopped"

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
	WorkspaceFinalizerName = "workspace.jupyter.org/workspace-protection"

	// ControllerPodNamespaceEnv is the environment variable for the controller pod namespace
	ControllerPodNamespaceEnv = "CONTROLLER_POD_NAMESPACE"

	// ControllerPodServiceAccountEnv is the environment variable for the controller pod service account
	ControllerPodServiceAccountEnv = "CONTROLLER_POD_SERVICE_ACCOUNT"

	// ResourcePrefix is the prefix for workspace resource names
	ResourcePrefix = "workspace"
)

// GenerateDeploymentName creates a consistent deployment name
func GenerateDeploymentName(workspaceName string) string {
	return fmt.Sprintf("%s-%s", ResourcePrefix, workspaceName)
}

// GenerateServiceName creates a consistent service name
func GenerateServiceName(workspaceName string) string {
	return fmt.Sprintf("%s-%s-service", ResourcePrefix, workspaceName)
}

// GeneratePVCName creates a consistent PVC name
func GeneratePVCName(workspaceName string) string {
	return fmt.Sprintf("%s-%s-pvc", ResourcePrefix, workspaceName)
}

// GenerateLabels creates consistent labels for resources with Kubernetes recommended labels
func GenerateLabels(workspaceName string) map[string]string {
	labels := map[string]string{
		// Existing custom labels
		AppLabel:                         AppLabelValue,
		workspaceutil.LabelWorkspaceName: workspaceName,
		LabelComponent:                   "workspace",
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
		KubernetesAppNameLabel:      AppLabelValue,
		KubernetesAppInstanceLabel:  workspaceName,
		KubernetesAppVersionLabel:   workspacev1alpha1.GroupVersion.Version,
		KubernetesAppComponentLabel: KubernetesAppComponentValue,
		KubernetesAppPartOfLabel:    KubernetesAppPartOfValue,
		KubernetesAppManagedByLabel: KubernetesAppManagedByValue,
	}
}
