//go:build e2e
// +build e2e

package e2e

import "time"

// Namespace constants
const (
	// controllerNamespace is where the jupyter-k8s controller is deployed
	controllerNamespace = "jupyter-k8s-system"

	// certManagerNamespace is where cert-manager is deployed
	certManagerNamespace = "cert-manager"
)

// Label and selector constants
const (
	// controllerLabel identifies controller manager pods
	controllerLabel = "control-plane=controller-manager"
)

// Resource name constants
const (
	// controllerDeploymentName is the name of the controller manager deployment
	controllerDeploymentName = "jupyter-k8s-controller-manager"

	// metricsServiceName is the name of the controller metrics service
	metricsServiceName = "jupyter-k8s-controller-manager-metrics-service"

	// metricsRoleBindingName is the RBAC binding for metrics access
	metricsRoleBindingName = "jupyter-k8s-metrics-binding"

	// webhookCertificateName is the cert-manager certificate for webhooks
	webhookCertificateName = "serving-cert"

	// mutatingWebhookConfigName is the mutating webhook configuration
	mutatingWebhookConfigName = "jupyter-k8s-mutating-webhook-configuration"

	// validatingWebhookConfigName is the validating webhook configuration
	validatingWebhookConfigName = "jupyter-k8s-validating-webhook-configuration"
)

// Timeout constants for Eventually/Wait operations
const (
	// fastTimeout for quick operations (pod status checks, etc.)
	fastTimeout = 10 * time.Second

	// defaultTimeout for standard operations (resource creation, deletion)
	defaultTimeout = 60 * time.Second

	// longTimeout for slow operations (controller deployment, webhook setup)
	longTimeout = 180 * time.Second

	// webhookReadinessTimeout for webhook certificate and CA bundle readiness
	webhookReadinessTimeout = 3 * time.Minute
)

// Polling interval constants
const (
	// defaultPollingInterval for Eventually operations
	defaultPollingInterval = 2 * time.Second

	// webhookPollingInterval for webhook readiness checks
	webhookPollingInterval = 5 * time.Second
)

// Cluster-scoped RBAC resources to clean up
var clusterRBACResources = []string{
	"clusterrole/jupyter-k8s-manager-role",
	"clusterrole/jupyter-k8s-metrics-reader",
	"clusterrole/jupyter-k8s-proxy-role",
	"clusterrolebinding/jupyter-k8s-manager-rolebinding",
	"clusterrolebinding/jupyter-k8s-proxy-rolebinding",
}
