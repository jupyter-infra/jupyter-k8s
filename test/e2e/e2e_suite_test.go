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

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

var (
	// Optional Environment Variables:
	// - CERT_MANAGER_INSTALL_SKIP=true: Skips CertManager installation during test setup.
	// - E2E_MANAGER_IMAGE: Override manager image (default: jupyter.org/jupyter-k8s:v0.0.1)
	// - E2E_ROTATOR_IMAGE: Override rotator image (default: docker.io/library/rotator:local)
	// - E2E_CHART_SOURCE: Override chart source — local path or oci:// URL (default: dist/chart)
	// - E2E_CHART_VERSION: Chart version for OCI source (required when E2E_CHART_SOURCE is oci://)
	skipCertManagerInstall = os.Getenv("CERT_MANAGER_INSTALL_SKIP") == "true"
	// isCertManagerAlreadyInstalled will be set true when CertManager CRDs be found on the cluster
	isCertManagerAlreadyInstalled = false

	projectImage    = envOrDefault("E2E_MANAGER_IMAGE", "jupyter.org/jupyter-k8s:v0.0.1")
	rotatorImage    = envOrDefault("E2E_ROTATOR_IMAGE", "docker.io/library/rotator:local")
	chartSource     = envOrDefault("E2E_CHART_SOURCE", "dist/chart")
	chartVersion    = os.Getenv("E2E_CHART_VERSION")
	helmReleaseName = "jupyter-k8s"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// splitImageRepo splits "ghcr.io/jupyter-infra/jupyter-k8s-rotator:tag" into
// ("ghcr.io/jupyter-infra", "jupyter-k8s-rotator:tag").
func splitImageRepo(image string) (string, string) {
	// Remove tag for path parsing
	path := image
	if idx := strings.LastIndex(image, ":"); idx > 0 {
		path = image[:idx]
	}
	lastSlash := strings.LastIndex(path, "/")
	if lastSlash < 0 {
		return "", image
	}
	return image[:lastSlash], image[lastSlash+1:]
}

// splitImageNameTag splits "jupyter-k8s-rotator:tag" into ("jupyter-k8s-rotator", "tag").
func splitImageNameTag(nameTag string) (string, string) {
	if idx := strings.LastIndex(nameTag, ":"); idx > 0 {
		return nameTag[:idx], nameTag[idx+1:]
	}
	return nameTag, "latest"
}

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

	// Ensure we're targeting the e2e Kind cluster, not a production context
	kindCluster := os.Getenv("KIND_CLUSTER")
	if kindCluster != "" {
		kindContext := fmt.Sprintf("kind-%s", kindCluster)
		By(fmt.Sprintf("switching kubectl context to %s", kindContext))
		cmd := exec.Command("kubectl", "config", "use-context", kindContext)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(),
			fmt.Sprintf("Kind context %s not found. Is the cluster running? Run: kind create cluster --name %s",
				kindContext, kindCluster))
	}

	// Check if operator is already installed and clean up the cluster if needed
	checkAndCleanCluster()

	var cmd *exec.Cmd
	var err error

	// The tests-e2e are intended to run on a temporary cluster that is created and destroyed for testing.
	// To prevent errors when tests run in environments with CertManager already installed,
	// we check for its presence before execution.
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

		By("waiting for cert-manager webhook to be ready")
		cmd = exec.Command("kubectl", "wait", "deployment/cert-manager-webhook",
			"-n", "cert-manager", "--for=condition=Available", "--timeout=3m")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "CertManager webhook failed to become ready")
	}

	By("installing Traefik CRDs for access strategy tests")
	cmd = exec.Command("kubectl", "get", "crds", "ingressroutes.traefik.io", "--ignore-not-found")
	traeficCRD, _ := utils.Run(cmd)
	if strings.TrimSpace(traeficCRD) == "" {
		By("Creating traefik namespace")
		cmd = exec.Command("kubectl", "create", "namespace", "traefik", "--dry-run=client", "-o", "yaml")
		createNs, _ := utils.Run(cmd)
		cmd = exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | kubectl apply -f -", createNs))
		_, _ = utils.Run(cmd)

		By("adding traefik helm repo")
		cmd = exec.Command("helm", "repo", "add", "traefik", "https://traefik.github.io/charts")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to add traefik helm repo")

		cmd = exec.Command("helm", "repo", "update")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to update helm repos")

		By("installing traefik CRDs")
		cmd = exec.Command("helm", "install", "traefik-crd", "traefik/traefik-crds",
			"--namespace", "traefik",
			"--version", "1.15.0")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install traefik CRDs")

		By("Verifying CRD installation")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "crds", "ingressroutes.traefik.io")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(ContainSubstring("ingressroutes.traefik.io"))
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
	} else {
		By("traefik CRDs already installed")
	}

	// Deploy operator via helm chart — this installs CRDs, RBAC, webhooks, extension API,
	// and the controller in a single step, matching what users actually consume.
	By(fmt.Sprintf("deploying operator via helm chart (source: %s)", chartSource))

	rotatorRepo, rotatorNameTag := splitImageRepo(rotatorImage)
	rotatorName, rotatorTag := splitImageNameTag(rotatorNameTag)

	helmArgs := []string{
		"upgrade", "--install", helmReleaseName, chartSource,
		"--namespace", OperatorNamespace, "--create-namespace",
		"--set", fmt.Sprintf("manager.image.repository=%s", strings.Split(projectImage, ":")[0]),
		"--set", fmt.Sprintf("manager.image.tag=%s", strings.Split(projectImage, ":")[1]),
		"--set", "manager.image.pullPolicy=Never",
		"--set", "application.imagesPullPolicy=Never",
		"--set", "application.imagesRegistry=docker.io/library",
		"--set", "extensionApi.enable=true",
		"--set", "extensionApi.jwtSecret.enable=true",
		"--set", fmt.Sprintf("extensionApi.jwtSecret.rotator.repository=%s", rotatorRepo),
		"--set", fmt.Sprintf("extensionApi.jwtSecret.rotator.imageName=%s", rotatorName),
		"--set", fmt.Sprintf("extensionApi.jwtSecret.rotator.imageTag=%s", rotatorTag),
		"--set", "extensionApi.jwtSecret.rotator.imagePullPolicy=Never",
		"--set", "workspacePodWatching.enable=true",
		"--set", "accessResources.traefik.enable=true",
		"--wait",
		"--timeout", "5m",
	}
	if strings.HasPrefix(chartSource, "oci://") && chartVersion != "" {
		helmArgs = append(helmArgs, "--version", chartVersion)
	}
	cmd = exec.Command("helm", helmArgs...)
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to deploy operator via helm")

	By("labeling the namespace to enforce the restricted security policy")
	cmd = exec.Command("kubectl", "label", "--overwrite", "ns", OperatorNamespace,
		"pod-security.kubernetes.io/enforce=restricted")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with pod security policy")

	By("creating jupyter-k8s-shared namespace for templates")
	cmd = exec.Command("kubectl", "create", "ns", SharedNamespace, "--dry-run=client", "-o", "yaml")
	nsYaml, _ := utils.Run(cmd)
	cmd = exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | kubectl apply -f -", nsYaml))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create jupyter-k8s-shared namespace")

	ensureCleanState()

	By("waiting for controller deployment to be available")
	cmd = exec.Command("kubectl", "wait", "deployment/jupyter-k8s-controller-manager",
		"-n", OperatorNamespace, "--for=condition=Available", "--timeout=3m")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Controller deployment failed to become available")

	By("verifying controller pods are running and ready")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "pods",
			"-l", "control-plane=controller-manager",
			"-n", OperatorNamespace,
			"-o", "jsonpath={.items[*].metadata.name}")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		podNames := strings.Fields(strings.TrimSpace(output))
		g.Expect(podNames).ToNot(BeEmpty(), "Expected at least 1 controller pod")

		// Verify pod is actually Running, not CrashLoopBackOff
		for _, podName := range podNames {
			phase, err := kubectlGet("pod", podName, OperatorNamespace, "{.status.phase}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(phase)).To(Equal("Running"),
				fmt.Sprintf("Pod %s not in Running state", podName))

			// Verify all containers are ready
			readyStates, err := kubectlGet("pod", podName, OperatorNamespace,
				"{.status.containerStatuses[*].ready}")
			g.Expect(err).NotTo(HaveOccurred())
			for _, ready := range strings.Fields(readyStates) {
				g.Expect(strings.TrimSpace(ready)).To(Equal("true"),
					fmt.Sprintf("Container in pod %s not ready", podName))
			}
		}
	}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())

	By("waiting for webhook certificate to be ready")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "certificate", WebhookCertificateName,
			"-n", OperatorNamespace,
			"-o", "jsonpath={.status.conditions[?(@.type=='Ready')].status}")
		status, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(status)).To(Equal("True"), "Certificate not ready")
	}).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	By("verifying mutating webhook configuration with CA bundle")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "mutatingwebhookconfiguration",
			"jupyter-k8s-mutating-webhook-configuration",
			"-o", "jsonpath={.webhooks[0].clientConfig.caBundle}")
		caBundle, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(caBundle)).NotTo(BeEmpty(), "CA bundle not injected")
	}).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	By("verifying validating webhook configuration with CA bundle")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "validatingwebhookconfiguration",
			"jupyter-k8s-validating-webhook-configuration",
			"-o", "jsonpath={.webhooks[0].clientConfig.caBundle}")
		caBundle, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(caBundle)).NotTo(BeEmpty(), "CA bundle not injected")
	}).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	By("testing webhook endpoint with dry-run API call")
	Eventually(func(g Gomega) {
		workspaceFilename := "webhook-readiness-test"
		group := "webhook"
		path := BuildTestResourcePath(workspaceFilename, group, "")
		cmd := exec.Command("kubectl", "apply", "-f", path)
		_, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred(), "Webhook should be ready to accept requests")

		// Clean up the test workspace immediately
		cmd = exec.Command("kubectl", "delete", "workspace", workspaceFilename,
			"-n", "default", "--ignore-not-found")
		_, _ = utils.Run(cmd)
	}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	By("waiting for extension API service to be available")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "apiservice",
			"v1alpha1.connection.workspace.jupyter.org",
			"-o", "jsonpath={.status.conditions[?(@.type=='Available')].status}")
		status, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(status)).To(Equal("True"),
			"Extension APIService not available")
	}).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

	_, _ = fmt.Fprintf(GinkgoWriter, "✓ Controller, webhooks, and extension API are ready for testing\n")
})

func dumpSetupDiagnostics() {
	_, _ = fmt.Fprintf(GinkgoWriter, "\n=== SETUP FAILED - Collecting Diagnostics ===\n")

	cmd := exec.Command("kubectl", "get", "pods", "-A")
	if output, _ := utils.Run(cmd); len(output) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nAll pods:\n%s\n", output)
	}

	// Detailed pod status for jupyter-k8s-system
	cmd = exec.Command("kubectl", "describe", "pods",
		"-n", OperatorNamespace, "-l", "control-plane=controller-manager")
	if podDetails, _ := utils.Run(cmd); len(podDetails) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nController pod details:\n%s\n", podDetails)
	}

	// Check deployment status
	cmd = exec.Command("kubectl", "get", "deployment",
		"-n", OperatorNamespace, "-o", "wide")
	if deploys, _ := utils.Run(cmd); len(deploys) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nDeployments:\n%s\n", deploys)
	}

	// Check replicasets (shows deployment issues)
	cmd = exec.Command("kubectl", "get", "replicasets",
		"-n", OperatorNamespace, "-o", "wide")
	if rs, _ := utils.Run(cmd); len(rs) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nReplicaSets:\n%s\n", rs)
	}

	cmd = exec.Command("kubectl", "logs", "-n", OperatorNamespace,
		"-l", "control-plane=controller-manager", "--tail=100")
	if logs, _ := utils.Run(cmd); len(logs) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nController logs:\n%s\n", logs)
	}

	cmd = exec.Command("kubectl", "get", "events", "-A",
		"--sort-by=.lastTimestamp", "--field-selector", "type=Warning")
	if events, _ := utils.Run(cmd); len(events) > 0 {
		_, _ = fmt.Fprintf(GinkgoWriter, "\nWarning events:\n%s\n", events)
	}

	cmd = exec.Command("kubectl", "get", "certificate", "-n", OperatorNamespace)
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
	// Delete custom resources first while the controller is running so it can process finalizers
	By("cleaning up all workspaces across all namespaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "--all-namespaces",
		"--ignore-not-found", "--wait=true", "--timeout=180s")
	if output, err := utils.Run(cmd); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Workspace cleanup failed: %v\nOutput: %s\n", err, output)
	}

	By("cleaning up all workspace templates across all namespaces")
	cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "--all-namespaces",
		"--ignore-not-found", "--wait=true", "--timeout=180s")
	if output, err := utils.Run(cmd); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Template cleanup failed: %v\nOutput: %s\n", err, output)
	}

	By("cleaning up all workspace access strategies across all namespaces")
	cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy", "--all", "--all-namespaces",
		"--ignore-not-found", "--wait=true", "--timeout=180s")
	if output, err := utils.Run(cmd); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: AccessStrategy cleanup failed: %v\nOutput: %s\n", err, output)
	}

	diagnoseAndCleanupStuckTemplates()
	diagnoseAndCleanupStuckAccessStrategies()

	By("uninstalling operator via helm")
	cmd = exec.Command("helm", "uninstall", helmReleaseName,
		"--namespace", OperatorNamespace, "--wait", "--timeout", "3m")
	if output, err := utils.Run(cmd); err != nil {
		_, _ = fmt.Fprintf(GinkgoWriter, "WARNING: Helm uninstall failed: %v\nOutput: %s\n", err, output)
	}

	// CRDs are kept by default (crd.keep=true), delete them explicitly
	By("uninstalling CRDs")
	crds := GetWorkspaceCrds()
	for _, crd := range crds {
		cmd = exec.Command("kubectl", "delete", "crd", crd, "--ignore-not-found")
		_, _ = utils.Run(cmd)
	}

	By("removing operator namespace")
	cmd = exec.Command("kubectl", "delete", "ns", OperatorNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=300s")
	_, _ = utils.Run(cmd)

	By("removing shared namespace")
	cmd = exec.Command("kubectl", "delete", "ns", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=300s")
	_, _ = utils.Run(cmd)

	if !skipCertManagerInstall && !isCertManagerAlreadyInstalled {
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CertManager...\n")
		utils.UninstallCertManager()
	}

	By("cleaning up Traefik resources")
	cmd = exec.Command("helm", "uninstall", "traefik-crd", "--namespace", "traefik", "--ignore-not-found")
	_, _ = utils.Run(cmd)
	cmd = exec.Command("kubectl", "delete", "ns", "traefik", "--ignore-not-found")
	_, _ = utils.Run(cmd)
})
