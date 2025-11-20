//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/


package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-ai-contrib/jupyter-k8s/test/utils"
)

var (
	// Optional Environment Variables:
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// These variables are useful if CertManager is already installed, avoiding
	// re-installation and conflicts.
	skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true"
	// isCertManagerAlreadyInstalled will be set true when CertManager CRDs be found on the cluster
	isCertManagerAlreadyInstalled = false

	// projectImage is the name of the image which will be build and loaded
	// with the code source changes to be tested.
	projectImage = "jupyter.org/jupyter-k8s:v0.0.1"
)

// TestE2E runs the end-to-end (e2e) test suite for the project. These tests execute in an isolated,
// temporary environment to validate project changes with the purpose of being used in CI jobs.
// The default setup requires Kind, builds/loads the Manager Docker image locally, and installs
// CertManager.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting jupyter-k8s integration test suite\n")
	RunSpecs(t, "e2e suite")
}

var _ = BeforeSuite(func() {
	defer func() {
		if CurrentSpecReport().Failed() {
			dumpSetupDiagnostics()
		}
	}()

	// Check if image exists to skip unnecessary rebuild
	By("checking if manager image exists")
	// Get container tool from environment (matches Makefile pattern)
	containerTool := os.Getenv("CONTAINER_TOOL")
	if containerTool == "" {
		containerTool = "docker" // Default to docker for CI compatibility
	}
	cmd := exec.Command(containerTool, "image", "inspect", projectImage)
	if _, err := cmd.CombinedOutput(); err != nil {
		By("building the manager(Operator) image")
		cmd = exec.Command("make", "docker-build", fmt.Sprintf("IMG=%s", projectImage))
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to build manager image")
	} else {
		_, _ = fmt.Fprintf(GinkgoWriter, "Manager image already exists, skipping build\n")
	}

	// TODO(user): If you want to change the e2e test vendor from Kind, ensure the image is
	// built and available before running the tests. Also, remove the following block.
	By("loading the manager(Operator) image on Kind")
	err := utils.LoadImageToKindClusterWithName(projectImage)
	Expect(err).NotTo(HaveOccurred(), "Failed to load image to Kind cluster")

	// Verify image was successfully loaded to Kind
	cmd = exec.Command("kubectl", "get", "nodes",
		"-o", "jsonpath={.items[0].status.nodeInfo.containerRuntimeVersion}")
	runtime, _ := utils.Run(cmd)
	_, _ = fmt.Fprintf(GinkgoWriter, "Container runtime: %s\n", strings.TrimSpace(string(runtime)))

	// List images on the node to confirm our image is there
	cmd = exec.Command("docker", "exec", "jupyter-k8s-test-e2e-control-plane",
		"crictl", "images")
	if images, err := utils.Run(cmd); err == nil {
		if strings.Contains(string(images), projectImage) {
			_, _ = fmt.Fprintf(GinkgoWriter, "✓ Image %s confirmed on node\n", projectImage)
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "⚠ WARNING: Image %s NOT found on node\n", projectImage)
		}
	}

	// The tests-e2e are intended to run on a temporary cluster that is created and destroyed for testing.
	// To prevent errors when tests run in environments with CertManager already installed,
	// we check for its presence before execution.
	// Setup CertManager before the suite if not skipped and if not already installed
	if !skipCertManagerInstall {
		By("checking if cert manager is installed already")
		isCertManagerAlreadyInstalled = utils.IsCertManagerCRDsInstalled()
		if !isCertManagerAlreadyInstalled {
			_, _ = fmt.Fprintf(GinkgoWriter, "Installing CertManager...\n")
			err := utils.InstallCertManager()
			Expect(err).NotTo(HaveOccurred(), "Failed to install CertManager")
		} else {
			_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: CertManager is already installed. Skipping installation...\n")
		}

		// Wait for cert-manager webhook to be ready before deploying controller
		By("waiting for cert-manager webhook to be ready")
		cmd = exec.Command("kubectl", "wait", "deployment/cert-manager-webhook",
			"-n", "cert-manager", "--for=condition=Available", "--timeout=3m")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "CertManager webhook failed to become ready")
	}

	By("creating shared test namespace")
	// Delete first to ensure clean state (idempotent)
	cmd = exec.Command("kubectl", "delete", "ns", namespace, "--ignore-not-found", "--wait=true", "--timeout=300s")
	_, _ = utils.Run(cmd)
	// Create namespace for all test suites
	cmd = exec.Command("kubectl", "create", "ns", namespace)
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create shared test namespace")

	// Verify namespace actually exists and is Active
	cmd = exec.Command("kubectl", "get", "ns", namespace,
		"-o", "jsonpath={.status.phase}")
	phase, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to verify namespace exists")
	Expect(strings.TrimSpace(string(phase))).To(Equal("Active"),
		"Namespace not in Active phase")

	By("labeling the namespace to enforce the restricted security policy")
	cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
		"pod-security.kubernetes.io/enforce=restricted")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with pod security policy")

	By("creating jupyter-k8s-shared namespace for templates")
	cmd = exec.Command("kubectl", "delete", "ns", "jupyter-k8s-shared", "--ignore-not-found", "--wait=true", "--timeout=300s")
	_, _ = utils.Run(cmd)
	cmd = exec.Command("kubectl", "create", "ns", "jupyter-k8s-shared")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create jupyter-k8s-shared namespace")

	// Consolidated controller deployment for all E2E tests
	// This eliminates race conditions from multiple test suites deploying separate controllers
	By("installing CRDs")
	cmd = exec.Command("make", "install")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

	By("waiting for CRDs to be established in API server")
	Eventually(func(g Gomega) {
		// Workspace CRD
		cmd := exec.Command("kubectl", "get", "crd",
			"workspaces.workspace.jupyter.org",
			"-o", "jsonpath={.status.conditions[?(@.type=='Established')].status}")
		status, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(string(status))).To(Equal("True"))

		// WorkspaceTemplate CRD
		cmd = exec.Command("kubectl", "get", "crd",
			"workspacetemplates.workspace.jupyter.org",
			"-o", "jsonpath={.status.conditions[?(@.type=='Established')].status}")
		status, err = utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(string(status))).To(Equal("True"))

		// WorkspaceAccessStrategy CRD
		cmd = exec.Command("kubectl", "get", "crd",
			"workspaceaccessstrategies.workspace.jupyter.org",
			"-o", "jsonpath={.status.conditions[?(@.type=='Established')].status}")
		status, err = utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(string(status))).To(Equal("True"))
	}).WithTimeout(30 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

	By("deploying the controller-manager with webhook enabled")
	cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to deploy controller-manager")

	By("creating extension API authentication RoleBinding")
	cmd = exec.Command("kubectl", "create", "rolebinding", "jupyter-k8s-extension-api-auth",
		"-n", "kube-system",
		"--role=extension-apiserver-authentication-reader",
		fmt.Sprintf("--serviceaccount=%s:jupyter-k8s-controller-manager", namespace))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create extension API auth RoleBinding")

	By("waiting for controller deployment to be available")
	cmd = exec.Command("kubectl", "wait", "deployment/jupyter-k8s-controller-manager",
		"-n", namespace, "--for=condition=Available", "--timeout=3m")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Controller deployment failed to become available")

	By("verifying controller pods are running")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "pods",
			"-l", "control-plane=controller-manager",
			"-n", namespace,
			"-o", "jsonpath={.items[*].metadata.name}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		podNames := strings.Fields(strings.TrimSpace(string(output)))
		g.Expect(len(podNames)).To(BeNumerically(">=", 1), "Expected at least 1 controller pod")

		// Verify pod is actually Running, not CrashLoopBackOff
		for _, podName := range podNames {
			cmd = exec.Command("kubectl", "get", "pod", podName,
				"-n", namespace,
				"-o", "jsonpath={.status.phase}")
			phase, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(string(phase))).To(Equal("Running"),
				fmt.Sprintf("Pod %s not in Running state", podName))
		}
	}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

	By("waiting for webhook certificate to be ready")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "certificate", webhookCertificateName,
			"-n", namespace,
			"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
		status, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(string(status))).To(Equal("True"), "Certificate not ready")
	}).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	By("verifying mutating webhook configuration with CA bundle")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "mutatingwebhookconfiguration",
			"jupyter-k8s-mutating-webhook-configuration",
			"-o", "jsonpath={.webhooks[0].clientConfig.caBundle}")
		caBundle, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(string(caBundle))).NotTo(BeEmpty(), "CA bundle not injected")
	}).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	By("verifying validating webhook configuration with CA bundle")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "validatingwebhookconfiguration",
			"jupyter-k8s-validating-webhook-configuration",
			"-o", "jsonpath={.webhooks[0].clientConfig.caBundle}")
		caBundle, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(string(caBundle))).NotTo(BeEmpty(), "CA bundle not injected")
	}).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	By("testing webhook endpoint with dry-run API call")
	cmd = exec.Command("kubectl", "apply", "--dry-run=server",
		"-f", "test/e2e/static/workspace_webhook_readiness.yaml")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Webhook endpoint failed to respond to dry-run request")

	_, _ = fmt.Fprintf(GinkgoWriter, "✓ Controller and webhooks are ready for testing\n")
})

func dumpSetupDiagnostics() {
	_, _ = fmt.Fprintf(GinkgoWriter, "\n=== SETUP FAILED - Collecting Diagnostics ===\n")

	cmd := exec.Command("kubectl", "get", "pods", "-A")
	if output, _ := utils.Run(cmd); len(output) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nAll pods:\n%s\n", output)
	}

	// Detailed pod status for jupyter-k8s-system
	cmd = exec.Command("kubectl", "describe", "pods",
		"-n", namespace, "-l", "control-plane=controller-manager")
	if podDetails, _ := utils.Run(cmd); len(podDetails) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nController pod details:\n%s\n", podDetails)
	}

	// Check deployment status
	cmd = exec.Command("kubectl", "get", "deployment",
		"-n", namespace, "-o", "wide")
	if deploys, _ := utils.Run(cmd); len(deploys) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nDeployments:\n%s\n", deploys)
	}

	// Check replicasets (shows deployment issues)
	cmd = exec.Command("kubectl", "get", "replicasets",
		"-n", namespace, "-o", "wide")
	if rs, _ := utils.Run(cmd); len(rs) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nReplicaSets:\n%s\n", rs)
	}

	cmd = exec.Command("kubectl", "logs", "-n", namespace,
		"-l", "control-plane=controller-manager", "--tail=100")
	if logs, _ := utils.Run(cmd); len(logs) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nController logs:\n%s\n", logs)
	}

	cmd = exec.Command("kubectl", "get", "events", "-A",
		"--sort-by=.lastTimestamp", "--field-selector", "type=Warning")
	if events, _ := utils.Run(cmd); len(events) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nWarning events:\n%s\n", events)
	}

	cmd = exec.Command("kubectl", "get", "certificate", "-n", namespace)
	if certs, _ := utils.Run(cmd); len(certs) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nCertificates:\n%s\n", certs)
	}

	cmd = exec.Command("kubectl", "get", "mutatingwebhookconfiguration",
		"jupyter-k8s-mutating-webhook-configuration", "-o", "yaml")
	if mwh, _ := utils.Run(cmd); len(mwh) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nMutatingWebhookConfiguration:\n%s\n", mwh)
	}
}

var _ = AfterSuite(func() {
	// Consolidated cleanup for all E2E tests
	// Resources are deleted in reverse order of creation to allow proper finalizer processing
	// Each cleanup step logs errors but continues to ensure comprehensive cleanup

	By("cleaning up all workspaces across all namespaces")
	// Delete workspaces synchronously to ensure controller processes finalizers
	// This allows template finalizers to be removed automatically by the controller
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "--all-namespaces",
		"--ignore-not-found", "--wait=true", "--timeout=180s")
	if output, err := utils.Run(cmd); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Workspace cleanup failed: %v\nOutput: %s\n", err, output)
	}

	By("cleaning up all workspace templates across all namespaces")
	// Templates should have finalizers removed by controller after workspaces are deleted
	// Controller automatically removes finalizers when templates are no longer referenced
	cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "--all-namespaces",
		"--ignore-not-found", "--wait=true", "--timeout=180s")
	if output, err := utils.Run(cmd); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Template cleanup failed: %v\nOutput: %s\n", err, output)
	}

	diagnoseAndCleanupStuckTemplates()

	By("deleting webhook configurations explicitly")
	// Webhook configurations may persist after undeploy if namespace is deleted first
	cmd = exec.Command("kubectl", "delete", "mutatingwebhookconfiguration",
		"jupyter-k8s-mutating-webhook-configuration", "--ignore-not-found")
	_, _ = utils.Run(cmd)
	cmd = exec.Command("kubectl", "delete", "validatingwebhookconfiguration",
		"jupyter-k8s-validating-webhook-configuration", "--ignore-not-found")
	_, _ = utils.Run(cmd)

	By("deleting cert-manager resources")
	// Clean up certificates and issuers created by the controller
	cmd = exec.Command("kubectl", "delete", "certificate", webhookCertificateName,
		"-n", namespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)
	cmd = exec.Command("kubectl", "delete", "issuer", "selfsigned-issuer",
		"-n", namespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	By("undeploying the controller-manager")
	// Controller must be stopped BEFORE deleting CRDs to allow proper finalizer processing
	// Otherwise finalizers can't be processed and resources become stuck
	cmd = exec.Command("make", "undeploy")
	if output, err := utils.Run(cmd); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Undeploy failed: %v\nOutput: %s\n", err, output)
	}

	By("waiting for controller pod to be fully terminated")
	// Ensure controller is completely stopped before deleting CRDs
	// This prevents race conditions with controller finalizer processing
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "pods", "-n", namespace,
			"-l", "control-plane=controller-manager",
			"-o", "jsonpath={.items[*].status.phase}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		// Should be no pods at all (empty output)
		g.Expect(strings.TrimSpace(string(output))).To(BeEmpty(),
			"Controller pod still exists or terminating")
	}).WithTimeout(120 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

	// Defensive check: Force delete if pod is still stuck despite Eventually verification
	// This handles edge cases like finalizers or disrupted API server communication
	cmd = exec.Command("kubectl", "get", "pods", "-n", namespace,
		"-l", "control-plane=controller-manager", "-o", "name")
	if output, _ := utils.Run(cmd); strings.TrimSpace(string(output)) != "" {
		By("Force deleting stuck controller pod")
		podName := strings.TrimSpace(string(output))
		cmd = exec.Command("kubectl", "delete", podName,
			"-n", namespace, "--force", "--grace-period=0")
		_, _ = utils.Run(cmd)
	}

	By("cleaning up cluster-scoped RBAC resources")
	// Cluster-scoped resources survive namespace deletion and must be explicitly removed
	rbacResources := []string{
		"clusterrole/jupyter-k8s-manager-role",
		"clusterrole/jupyter-k8s-metrics-reader",
		"clusterrole/jupyter-k8s-proxy-role",
		"clusterrolebinding/jupyter-k8s-manager-rolebinding",
		"clusterrolebinding/jupyter-k8s-proxy-rolebinding",
	}
	for _, resource := range rbacResources {
		cmd := exec.Command("kubectl", "delete", resource, "--ignore-not-found")
		_, _ = utils.Run(cmd)
	}

	By("uninstalling CRDs")
	// CRDs must be deleted after controller termination to avoid orphaned resources
	cmd = exec.Command("make", "uninstall")
	if output, err := utils.Run(cmd); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: CRD uninstall failed: %v\nOutput: %s\n", err, output)
	}

	By("removing shared test namespace")
	cmd = exec.Command("kubectl", "delete", "ns", namespace, "--wait=true", "--timeout=300s")
	_, _ = utils.Run(cmd)

	// Teardown CertManager after the suite if not skipped and if it was not already installed
	if !skipCertManagerInstall && !isCertManagerAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CertManager...\n")
		utils.UninstallCertManager()
	}
})
