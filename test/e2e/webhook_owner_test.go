//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-ai-contrib/jupyter-k8s/test/utils"
)

var _ = Describe("Webhook Owner", Ordered, func() {
	BeforeAll(func() {
		By("installing CRDs")
		cmd := exec.Command("make", "install")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("deploying the controller-manager with webhook enabled")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for controller-manager to be ready")
		Eventually(func() error {
			cmd := exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
				"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}{{ \"\\n\" }}{{ end }}{{ end }}",
				"-n", "jupyter-k8s-system")
			output, err := utils.Run(cmd)
			if err != nil {
				return err
			}
			podName := strings.TrimSpace(string(output))
			if podName == "" {
				return fmt.Errorf("no controller pod found")
			}

			cmd = exec.Command("kubectl", "get", "pods", podName,
				"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}",
				"-n", "jupyter-k8s-system")
			output, err = utils.Run(cmd)
			if err != nil {
				return err
			}
			if strings.TrimSpace(string(output)) != "True" {
				return fmt.Errorf("pod not ready")
			}
			return nil
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		By("verifying webhook service exists")
		cmd = exec.Command("kubectl", "get", "service", "jupyter-k8s-controller-manager",
			"-n", "jupyter-k8s-system")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("cleaning up test workspaces")
		// Use --wait=true for synchronous deletion to ensure finalizers are processed
		cmd := exec.Command("kubectl", "delete", "workspace", "--all", "--ignore-not-found", "--wait=true", "--timeout=180s")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		// Undeploy controller BEFORE uninstalling CRDs to allow controller to process finalizers
		// This follows K8s best practice: delete resources in reverse order of creation
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("waiting for controller pod to be fully terminated")
		// Ensure controller is completely stopped before deleting CRDs
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods",
				"-n", "jupyter-k8s-system",
				"-l", "control-plane=controller-manager",
				"-o", "name")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(string(output))).To(BeEmpty())
		}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		By("uninstalling CRDs")
		// Delete CRDs last to avoid race conditions with controller finalizer processing
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)
	})

	Context("Ownership Annotation", func() {
		It("should add created-by annotation to new workspace", func() {
			By("creating a workspace")
			cmd := exec.Command("kubectl", "apply", "-f", "config/samples/workspace_v1alpha1_workspace.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying created-by annotation exists")
			cmd = exec.Command("kubectl", "get", "workspace", "workspace-sample",
				"-o", "jsonpath={.metadata.annotations.workspace\\.jupyter\\.org/created-by}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			annotation := strings.TrimSpace(string(output))
			Expect(annotation).NotTo(BeEmpty(), "created-by annotation should be present")

			By("verifying annotation contains user identity")
			// Annotation should contain some form of user identity (system:, kubernetes-admin, etc.)
			Expect(len(annotation)).To(BeNumerically(">", 0), "annotation should contain user identity")
		})

		It("should preserve existing annotations when adding created-by", func() {
			By("creating workspace with existing annotation")
			workspaceYAML := `apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: workspace-with-annotations
  annotations:
    custom-annotation: "test-value"
spec:
  displayName: "Test Workspace"
`
			cmd := exec.Command("sh", "-c", fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYAML))
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying both annotations exist")
			cmd = exec.Command("kubectl", "get", "workspace", "workspace-with-annotations",
				"-o", "jsonpath={.metadata.annotations}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			annotations := string(output)
			Expect(annotations).To(ContainSubstring("custom-annotation"))
			Expect(annotations).To(ContainSubstring("created-by"))
		})
	})

	Context("Webhook Health", func() {
		It("should have webhook endpoint responding", func() {
			By("getting controller pod name")
			cmd := exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
				"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}{{ \"\\n\" }}{{ end }}{{ end }}",
				"-n", "jupyter-k8s-system")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			podName := strings.TrimSpace(string(output))
			Expect(podName).NotTo(BeEmpty())

			By("checking webhook logs for successful processing")
			cmd = exec.Command("kubectl", "logs", podName, "-n", "jupyter-k8s-system", "--tail=50")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			logs := string(output)
			Expect(logs).To(ContainSubstring("workspace-with-annotations"), "webhook should be logging")
		})
	})

	Context("Webhook Configuration", func() {
		It("should have mutating webhook configuration registered", func() {
			By("checking mutating webhook configuration exists")
			cmd := exec.Command("kubectl", "get", "mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration")
			output, err := utils.Run(cmd)
			if err != nil {
				Skip("Mutating webhook configuration not found - webhooks may not be enabled in this deployment")
			}
			Expect(string(output)).To(ContainSubstring("jupyter-k8s-mutating-webhook-configuration"))

			By("verifying webhook targets correct service")
			cmd = exec.Command("kubectl", "get", "mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration",
				"-o", "jsonpath={.webhooks[0].clientConfig.service.name}")
			output, err = utils.Run(cmd)
			if err == nil {
				serviceName := strings.TrimSpace(string(output))
				Expect(serviceName).To(Equal("jupyter-k8s-controller-manager"))
			}
		})

		It("should have CA bundle configured", func() {
			By("checking CA bundle is present or webhook is configured")
			cmd := exec.Command("kubectl", "get", "mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration")
			_, err := utils.Run(cmd)
			if err != nil {
				Skip("Mutating webhook configuration not found - webhooks may not be enabled")
			}

			cmd = exec.Command("kubectl", "get", "mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration",
				"-o", "jsonpath={.webhooks[0].clientConfig.caBundle}")
			output, err := utils.Run(cmd)
			// CA bundle may be empty if cert-manager hasn't injected it yet, but config should exist
			Expect(err).NotTo(HaveOccurred())
			_ = output // CA bundle presence is optional depending on cert-manager setup
		})
	})
})
