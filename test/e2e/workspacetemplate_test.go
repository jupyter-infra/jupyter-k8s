//go:build e2e
// +build e2e

/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

var _ = Describe("WorkspaceTemplate", Ordered, func() {
	var controllerPodName string

	// Helper functions for workspace management
	findWorkspacesUsingTemplate := func(templateName string) ([]string, error) {
		// Use label selector to find workspaces by template
		// Labels persist during deletion, unlike spec.templateRef which gets cleared
		labelSelector := fmt.Sprintf("workspace.jupyter.org/template=%s", templateName)
		cmd := exec.Command("kubectl", "get", "workspace", "-l", labelSelector, "-o", "jsonpath={.items[*].metadata.name}")
		output, err := utils.Run(cmd)
		if err != nil {
			return nil, fmt.Errorf("failed to list workspaces: %w", err)
		}

		if output == "" {
			return []string{}, nil
		}

		workspaces := strings.Fields(output)
		return workspaces, nil
	}

	waitForWorkspaceDeletion := func(templateName string) {
		By("waiting for workspaces to be fully deleted")
		Eventually(func(g Gomega) {
			workspaces, err := findWorkspacesUsingTemplate(templateName)
			if err != nil {
				// If we can't list workspaces, fail the test
				g.Expect(err).NotTo(HaveOccurred(), "failed to list workspaces")
				return
			}

			// Wait until NO workspaces are found (fully deleted, not just being deleted)
			g.Expect(workspaces).To(BeEmpty(), fmt.Sprintf("expected all workspaces using template %s to be fully deleted, but found: %v", templateName, workspaces))
		}).WithTimeout(180 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
	}

	// deleteAllWorkspacesUsingTemplate deletes all workspaces that reference a specific template
	deleteAllWorkspacesUsingTemplate := func(templateName string) {
		By("deleting all workspaces using template: " + templateName)

		workspacesToDelete, err := findWorkspacesUsingTemplate(templateName)
		if err != nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "Failed to find workspaces using template %s: %v\n", templateName, err)
			return
		}

		if len(workspacesToDelete) == 0 {
			_, _ = fmt.Fprintf(GinkgoWriter, "No workspaces found using template %s\n", templateName)
			return
		}

		_, _ = fmt.Fprintf(GinkgoWriter, "Deleting %d workspace(s): %v\n", len(workspacesToDelete), workspacesToDelete)
		cmd := exec.Command("kubectl", "delete", "workspace")
		cmd.Args = append(cmd.Args, workspacesToDelete...)
		cmd.Args = append(cmd.Args, "--ignore-not-found", "--timeout=60s")
		_, _ = utils.Run(cmd)

		waitForWorkspaceDeletion(templateName)
	}

	BeforeAll(func() {
		By("installing CRDs")
		_, _ = fmt.Fprintf(GinkgoWriter, "Installing CRDs...\n")
		cmd := exec.Command("make", "install")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		_, _ = fmt.Fprintf(GinkgoWriter, "Deploying controller manager...\n")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy controller")

		By("waiting for controller-manager to be ready")
		_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for controller manager pod...\n")
		verifyControllerUp := func(g Gomega) {
			cmd := exec.Command("kubectl", "get",
				"pods", "-l", "control-plane=controller-manager",
				"-o", "go-template={{ range .items }}"+
					"{{ if not .metadata.deletionTimestamp }}"+
					"{{ .metadata.name }}"+
					"{{ \"\\n\" }}{{ end }}{{ end }}",
				"-n", namespace,
			)
			podOutput, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			podNames := utils.GetNonEmptyLines(podOutput)
			g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
			controllerPodName = podNames[0]

			// Check pod is Ready (not just Running) - ensures webhook server is initialized
			cmd = exec.Command("kubectl", "get",
				"pods", controllerPodName, "-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}",
				"-n", namespace,
			)
			readyStatus, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(readyStatus).To(Equal("True"), "expected controller pod to be Ready (webhook server initialized)")
		}
		Eventually(verifyControllerUp, 2*time.Minute).Should(Succeed())

		By("waiting for webhook server to be ready")
		_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for webhook server to accept requests...\n")
		verifyWebhookReady := func(g Gomega) {
			// Verify mutating webhook configuration has CA bundle injected
			cmd := exec.Command("kubectl", "get", "mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration",
				"-o", "jsonpath={.webhooks[0].clientConfig.caBundle}")
			caBundle, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(caBundle).NotTo(BeEmpty(), "webhook CA bundle should be injected by cert-manager")

			// Verify webhook endpoint is reachable by attempting a dry-run resource creation
			testWorkspaceYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: webhook-readiness-test
spec:
  displayName: "Webhook Readiness Test"`
			cmd = exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply --dry-run=server -f -", testWorkspaceYaml))
			_, err = utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred(), "webhook server should respond to admission requests")
		}
		Eventually(verifyWebhookReady, 30*time.Second, 2*time.Second).Should(Succeed())

		By("installing WorkspaceTemplate samples")
		_, _ = fmt.Fprintf(GinkgoWriter, "Applying production template...\n")
		cmd = exec.Command("kubectl", "apply", "-f",
			"config/samples/workspace_v1alpha1_workspacetemplate_production.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create production template")

		_, _ = fmt.Fprintf(GinkgoWriter, "Applying restricted template for validation tests...\n")
		cmd = exec.Command("kubectl", "apply", "-f",
			"config/samples/test_template_rejection.yaml")
		_, err = utils.Run(cmd)
		// Expect this to fail because test-rejected-workspace violates template constraints
		// The webhook should reject invalid workspaces at admission time
		Expect(err).To(HaveOccurred(), "Expected webhook to reject invalid workspace")
	})

	AfterAll(func() {
		By("cleaning up test workspaces")
		_, _ = fmt.Fprintf(GinkgoWriter, "Deleting test workspaces...\n")
		cmd := exec.Command("kubectl", "delete", "workspace",
			"workspace-with-template", "test-valid-workspace",
			"cpu-exceed-test", "valid-overrides-test",
			"deletion-protection-test", "templateref-mutability-test",
			"lazy-application-test", "compliance-check-test",
			"cel-immutability-test",
			"--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)

		By("waiting for all workspaces to be fully deleted")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "workspace", "-o", "name")
			output, err := utils.Run(cmd)
			if err != nil {
				// Error listing workspaces - fail test to investigate
				g.Expect(err).NotTo(HaveOccurred(), "failed to list workspaces during cleanup")
				return
			}

			output = strings.TrimSpace(output)
			// Wait until NO workspaces exist (fully deleted, not just being deleted)
			// This is critical because webhook needs templates to exist to validate
			// finalizer removal during workspace deletion
			g.Expect(output).To(BeEmpty(), fmt.Sprintf("expected all workspaces to be fully deleted before deleting templates, but found: %s", output))
		}).WithTimeout(180 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		By("cleaning up test templates")
		_, _ = fmt.Fprintf(GinkgoWriter, "Deleting test templates...\n")
		// Templates with lazy finalizers will only delete after workspaces are gone
		cmd = exec.Command("kubectl", "delete", "workspacetemplate",
			"production-notebook-template", "restricted-template",
			"mutability-test-template", "restricted-template-mutability",
			"lazy-application-template", "compliance-template",
			"immutability-test-template",
			"--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CRDs (this deletes all CRs and allows controller to handle finalizers)...\n")
		cmd = exec.Command("make", "uninstall")
		cmd.Args = append(cmd.Args, "--timeout=300s")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		_, _ = fmt.Fprintf(GinkgoWriter, "Undeploying controller manager...\n")
		cmd = exec.Command("make", "undeploy")
		cmd.Args = append(cmd.Args, "--timeout=300s")
		_, _ = utils.Run(cmd)
	})

	Context("Template Creation and Usage", func() {
		It("should create WorkspaceTemplate successfully", func() {
			By("verifying production template exists")
			verifyTemplateExists := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspacetemplate",
					"production-notebook-template", "-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("production-notebook-template"))
			}
			Eventually(verifyTemplateExists).
				WithPolling(1 * time.Second).
				WithTimeout(10 * time.Second).
				Should(Succeed())

			By("verifying template has correct spec fields")
			cmd := exec.Command("kubectl", "get", "workspacetemplate",
				"production-notebook-template", "-o", "jsonpath={.spec.displayName}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Production Jupyter Notebook"))
		})

		It("should create Workspace using template and pass validation", func() {
			var output string
			var err error

			By("applying workspace with template reference")
			cmd := exec.Command("kubectl", "apply", "-f",
				"config/samples/workspace_v1alpha1_workspace_with_template.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Valid condition is True within 10s (before compute is ready)")
			verifyValid := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "workspace-with-template",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyValid).
				WithPolling(1 * time.Second).
				WithTimeout(10 * time.Second). // valid state should be set fast, before compute is ready
				Should(Succeed())

			By("verifying Degraded condition is False")
			cmd = exec.Command("kubectl", "get", "workspace", "workspace-with-template",
				"-o", "jsonpath={.status.conditions[?(@.type==\"Degraded\")].status}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("False"))
		})
	})

	Context("Template Validation", func() {
		It("should reject workspace with image not in allowlist", func() {
			By("attempting to create workspace with invalid image")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/template-validation/rejected-image-workspace.yaml")
			
			_, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Expected webhook to reject workspace with invalid image")
			
			By("verifying workspace was not created")
			cmd = exec.Command("kubectl", "get", "workspace", "test-rejected-workspace", "--ignore-not-found")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty(), "Workspace should not exist after webhook rejection")
		})

		It("should reject workspace exceeding CPU bounds", func() {
			By("creating workspace with CPU exceeding template max")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/template-validation/cpu-exceed-workspace.yaml")
			_, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Expected webhook to reject workspace with CPU exceeding template max")

			By("verifying workspace was not created")
			cmd = exec.Command("kubectl", "get", "workspace", "cpu-exceed-test", "--ignore-not-found")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty(), "Workspace should not exist after webhook rejection")
		})

		It("should accept workspace with valid overrides", func() {
			var output string
			var err error

			By("creating workspace with valid resource overrides")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/template-validation/valid-overrides-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying Valid condition is True")
			verifyValid := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "valid-overrides-test",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyValid).
				WithPolling(1 * time.Second).
				WithTimeout(10 * time.Second). // valid should be set fast on update
				Should(Succeed())

			By("verifying Degraded condition is False")
			cmd = exec.Command("kubectl", "get", "workspace", "valid-overrides-test",
				"-o", "jsonpath={.status.conditions[?(@.type==\"Degraded\")].status}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("False"))
		})
	})

	Context("Template Mutability and Deletion Protection", func() {
		It("should prevent template deletion when workspace is using it", func() {
			var output string
			var err error

			By("creating a workspace using production template")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/template-mutability/deletion-protection-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace has templateRef set")
			cmd = exec.Command("kubectl", "get", "workspace", "deletion-protection-test",
				"-o", "jsonpath={.spec.templateRef.name}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("production-notebook-template"))

			By("waiting for workspace to have template label")
			verifyWorkspaceLabel := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "deletion-protection-test",
					"-o", "jsonpath={.metadata.labels.workspace\\.jupyter\\.org/template}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("production-notebook-template"), "workspace should have template label")
			}
			Eventually(verifyWorkspaceLabel).
				WithPolling(500 * time.Millisecond).
				WithTimeout(10 * time.Second).
				Should(Succeed())

			By("inspecting controller logs before checking finalizer")
			cmd = exec.Command("kubectl", "logs", "-n", "jupyter-k8s-system",
				"-l", "control-plane=controller-manager",
				"--tail=100")
			logsOutput, logsErr := utils.Run(cmd)
			if logsErr == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "=== Controller Logs (last 100 lines) ===\n%s\n", logsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get controller logs: %v\n", logsErr)
			}

			By("waiting for finalizer to be added by controller")
			verifyFinalizerAdded := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspacetemplate",
					"production-notebook-template",
					"-o", "jsonpath={.metadata.finalizers[0]}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("workspace.jupyter.org/template-protection"))
			}
			Eventually(verifyFinalizerAdded).
				WithPolling(500 * time.Millisecond).
				WithTimeout(10 * time.Second).
				Should(Succeed())

			By("attempting to delete the template")
			cmd = exec.Command("kubectl", "delete", "workspacetemplate",
				"production-notebook-template", "--wait=false")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying template still exists with deletionTimestamp set")
			time.Sleep(2 * time.Second)
			cmd = exec.Command("kubectl", "get", "workspacetemplate",
				"production-notebook-template",
				"-o", "jsonpath={.metadata.deletionTimestamp}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "expected deletionTimestamp to be set")

			By("verifying finalizer is blocking deletion")
			cmd = exec.Command("kubectl", "get", "workspacetemplate",
				"production-notebook-template",
				"-o", "jsonpath={.metadata.finalizers[0]}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("workspace.jupyter.org/template-protection"))

			// Delete all workspaces using the template (including deletion-protection-test)
			deleteAllWorkspacesUsingTemplate("production-notebook-template")

			By("inspecting controller logs after workspace deletion")
			cmd = exec.Command("kubectl", "logs", "-n", "jupyter-k8s-system",
				"-l", "control-plane=controller-manager",
				"--tail=200")
			logsOutput, logsErr = utils.Run(cmd)
			if logsErr == nil {
				_, _ = fmt.Fprintf(GinkgoWriter, "=== Controller Logs After Workspace Deletion (last 200 lines) ===\n%s\n", logsOutput)
			} else {
				_, _ = fmt.Fprintf(GinkgoWriter, "Failed to get controller logs: %v\n", logsErr)
			}

			By("verifying template can now be deleted")
			verifyTemplateDeleted := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspacetemplate",
					"production-notebook-template")
				_, err := utils.Run(cmd)
				g.Expect(err).To(HaveOccurred(), "expected template to be deleted")
			}
			Eventually(verifyTemplateDeleted).
				WithPolling(1 * time.Second).
				WithTimeout(30 * time.Second).
				Should(Succeed())
		})

		It("should allow workspace templateRef changes (mutability)", func() {
			var err error

			By("re-creating production template for mutability tests")
			cmd := exec.Command("kubectl", "apply", "-f",
				"config/samples/workspace_v1alpha1_workspacetemplate_production.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating restricted template for switching")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/template-mutability/restricted-template-mutability.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace using production template")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/template-mutability/templateref-mutability-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying initial templateRef")
			cmd = exec.Command("kubectl", "get", "workspace", "templateref-mutability-test",
				"-o", "jsonpath={.spec.templateRef.name}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("production-notebook-template"))

			By("changing templateRef to restricted-template-mutability")
			patchCmd := `{"spec":{"templateRef":{"name":"restricted-template-mutability"}}}`
			cmd = exec.Command("kubectl", "patch", "workspace", "templateref-mutability-test",
				"--type=merge", "-p", patchCmd)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "templateRef should be mutable")

			By("verifying templateRef was changed")
			cmd = exec.Command("kubectl", "get", "workspace", "templateref-mutability-test",
				"-o", "jsonpath={.spec.templateRef.name}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("restricted-template-mutability"))

			By("cleaning up test workspace and template")
			cmd = exec.Command("kubectl", "delete", "workspace", "templateref-mutability-test")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "restricted-template-mutability")
			_, _ = utils.Run(cmd)
		})

		It("should allow WorkspaceTemplate spec modification (mutability)", func() {
			By("creating a template for mutability testing")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/template-mutability/mutability-test-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying template was created")
			cmd = exec.Command("kubectl", "get", "workspacetemplate",
				"mutability-test-template", "-o", "jsonpath={.metadata.name}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("mutability-test-template"))

			By("modifying template spec (change description)")
			patchCmd := `{"spec":{"description":"Modified description - should succeed"}}`
			cmd = exec.Command("kubectl", "patch", "workspacetemplate",
				"mutability-test-template", "--type=merge",
				"-p", patchCmd)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "template spec should be mutable")

			By("verifying template spec description was changed")
			cmd = exec.Command("kubectl", "get", "workspacetemplate",
				"mutability-test-template", "-o", "jsonpath={.spec.description}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Modified description - should succeed"))

			By("cleaning up test template")
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "mutability-test-template")
			_, _ = utils.Run(cmd)
		})
	})

	Context("Webhook Validation", func() {
		It("should apply template defaults during workspace creation", func() {
			By("creating workspace without specifying image, resources, or storage")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/webhook-defaults-workspace.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying template defaults were applied")
			cmd = exec.Command("kubectl", "get", "workspace", "webhook-defaults-test", "-o", "yaml")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("jk8s-application-jupyter-uv:latest"))
			Expect(output).To(ContainSubstring("cpu: 200m"))
			Expect(output).To(ContainSubstring("memory: 256Mi"))
			Expect(output).To(ContainSubstring("size: 1Gi"))

			By("verifying template tracking label was added")
			cmd = exec.Command("kubectl", "get", "workspace", "webhook-defaults-test",
				"-o", "jsonpath={.metadata.labels['workspace\\.jupyter\\.org/template']}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("production-notebook-template"))

			By("cleaning up test workspace")
			cmd = exec.Command("kubectl", "delete", "workspace", "webhook-defaults-test")
			_, _ = utils.Run(cmd)
		})

		It("should inherit default access strategy from template", func() {
			By("creating a template with default access strategy")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/access-strategy-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace without access strategy")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/access-strategy-inheritance-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace inherited access strategy from template")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "access-strategy-inheritance-test",
					"-o", "jsonpath={.spec.accessStrategy.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("test-access-strategy"))
			}).WithTimeout(10 * time.Second).Should(Succeed())

			By("verifying workspace inherited access strategy namespace")
			cmd = exec.Command("kubectl", "get", "workspace", "access-strategy-inheritance-test",
				"-o", "jsonpath={.spec.accessStrategy.namespace}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("default"))

			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "workspace", "access-strategy-inheritance-test", "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "access-strategy-template", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should reject workspace creation with multiple violations", func() {
			By("attempting to create workspace with multiple template violations")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/multi-violation-workspace.yaml")
			output, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Expected webhook to reject workspace with multiple violations")
			Expect(output).To(ContainSubstring("violations"))

			By("verifying workspace was not created")
			cmd = exec.Command("kubectl", "get", "workspace", "multi-violation-test", "--ignore-not-found")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty())
		})

		It("should inherit access type and app type from template", func() {
			By("creating template with default access type and app type")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/access-app-type-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace without access type or app type")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/access-app-type-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace inherited access type")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "access-app-type-test",
					"-o", "jsonpath={.spec.accessType}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("OwnerOnly"))
			}).WithTimeout(10 * time.Second).Should(Succeed())

			By("verifying workspace inherited app type")
			cmd = exec.Command("kubectl", "get", "workspace", "access-app-type-test",
				"-o", "jsonpath={.spec.appType}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("jupyter-lab"))

			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "workspace", "access-app-type-test", "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "access-app-type-template", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should inherit lifecycle and idle shutdown from template", func() {
			By("creating template with default lifecycle and idle shutdown")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/lifecycle-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace without lifecycle or idle shutdown")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/lifecycle-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace inherited lifecycle")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "lifecycle-test",
					"-o", "jsonpath={.spec.lifecycle.postStart.exec.command[0]}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("/bin/sh"))
			}).WithTimeout(10 * time.Second).Should(Succeed())

			By("verifying workspace inherited idle shutdown")
			cmd = exec.Command("kubectl", "get", "workspace", "lifecycle-test",
				"-o", "jsonpath={.spec.idleShutdown.enabled}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("true"))

			cmd = exec.Command("kubectl", "get", "workspace", "lifecycle-test",
				"-o", "jsonpath={.spec.idleShutdown.timeoutMinutes}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("30"))

			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "workspace", "lifecycle-test", "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "lifecycle-template", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should reject custom images when allowCustomImages is false", func() {
			By("creating template with allowCustomImages: false")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/restricted-images-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("attempting to create workspace with custom image")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/restricted-images-workspace.yaml")
			output, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Expected webhook to reject custom image")
			Expect(output).To(ContainSubstring("not allowed"))

			By("cleaning up template")
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "restricted-images-template")
			_, _ = utils.Run(cmd)
		})

		It("should allow any image when allowCustomImages is true", func() {
			By("creating template with allowCustomImages: true")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/custom-images-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace with custom image")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/custom-images-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Expected webhook to allow custom image")

			By("verifying workspace was created with custom image")
			cmd = exec.Command("kubectl", "get", "workspace", "custom-image-allowed-test", "-o", "jsonpath={.spec.image}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("custom/authorized:latest"))

			By("cleaning up workspace and template")
			cmd = exec.Command("kubectl", "delete", "workspace", "custom-image-allowed-test")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "custom-images-template")
			_, _ = utils.Run(cmd)
		})
	})

	Context("Template Compliance Tracking", func() {
		It("should mark workspaces for compliance check when template constraints change", func() {
			By("creating compliance template")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace using compliance template")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace is initially valid")
			verifyInitiallyValid := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyInitiallyValid).
				WithPolling(1 * time.Second).
				WithTimeout(15 * time.Second).
				Should(Succeed())

			By("waiting for template initial reconciliation to complete")
			// Wait for controller to finish initial reconciliation (observedGeneration=1).
			// This ensures the controller has processed the template at least once and is ready
			// to detect subsequent changes. Without this, patching too quickly causes a race where
			// the controller hasn't seen generation=1 yet, so it can't detect the 1→2 transition.
			verifyTemplateReconciled := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspacetemplate", "constraint-violation-template",
					"-o", "jsonpath={.status.observedGeneration}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1"), "Template should complete initial reconciliation before patch")
			}
			Eventually(verifyTemplateReconciled).
				WithPolling(500 * time.Millisecond).
				WithTimeout(15 * time.Second).
				Should(Succeed())

			By("updating template to change AllowedImages (remove scipy-notebook)")
			// This should trigger compliance check since the workspace uses scipy-notebook
			patchCmd := `{"spec":{"allowedImages":["quay.io/jupyter/minimal-notebook:latest"]}}`
			cmd = exec.Command("kubectl", "patch", "workspacetemplate", "constraint-violation-template",
				"--type=merge", "-p", patchCmd)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying compliance label was added to workspace")
			verifyLabelAdded := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
					"-o", "jsonpath={.metadata.labels['workspace\\.jupyter\\.org/compliance-check-needed']}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}
			Eventually(verifyLabelAdded).
				WithPolling(500 * time.Millisecond).
				WithTimeout(30 * time.Second).
				Should(Succeed())

			By("waiting for controller to process compliance check")
			// Controller should detect label, validate workspace, update status, and remove label
			time.Sleep(3 * time.Second)

			By("verifying compliance label was removed after check")
			verifyLabelRemoved := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
					"-o", "jsonpath={.metadata.labels['workspace\\.jupyter\\.org/compliance-check-needed']}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(BeEmpty(), "compliance label should be removed after check")
			}
			Eventually(verifyLabelRemoved).
				WithPolling(500 * time.Millisecond).
				WithTimeout(15 * time.Second).
				Should(Succeed())

			By("verifying workspace status reflects compliance failure")
			cmd = exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
				"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("False"), "workspace should be Invalid after failing compliance check")

			By("verifying ComplianceCheckFailed event was recorded")
			cmd = exec.Command("kubectl", "get", "events",
				"--field-selector", "involvedObject.name=constraint-violation-test",
				"-o", "jsonpath={.items[*].reason}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("ComplianceCheckFailed"))

			By("cleaning up workspace and template")
			cmd = exec.Command("kubectl", "delete", "workspace", "constraint-violation-test")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "constraint-violation-template")
			_, _ = utils.Run(cmd)
		})

		It("should pass compliance check when workspace still complies after constraint changes", func() {
			By("creating compliance template")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace using default image from template")
			workspaceYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: compliance-pass-test
spec:
  displayName: "Compliance Pass Test"
  templateRef:
    name: constraint-violation-template
  image: "quay.io/jupyter/minimal-notebook:latest"
  resources:
    requests:
      cpu: "200m"
      memory: "256Mi"
  storage:
    size: "1Gi"`
			cmd = exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace is initially valid")
			verifyInitiallyValid := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "compliance-pass-test",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyInitiallyValid).
				WithPolling(1 * time.Second).
				WithTimeout(15 * time.Second).
				Should(Succeed())

			By("updating template with tighter CPU constraints (workspace still complies)")
			// Workspace has cpu: 200m, change min from 100m to 150m and max from 2 to 1
			patchCmd := `{"spec":{"resourceBounds":{"cpu":{"min":"150m","max":"1"}}}}`
			cmd = exec.Command("kubectl", "patch", "workspacetemplate", "constraint-violation-template",
				"--type=merge", "-p", patchCmd)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying compliance label was added")
			verifyLabelAdded := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "compliance-pass-test",
					"-o", "jsonpath={.metadata.labels['workspace\\.jupyter\\.org/compliance-check-needed']}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("true"))
			}
			Eventually(verifyLabelAdded).
				WithPolling(500 * time.Millisecond).
				WithTimeout(10 * time.Second).
				Should(Succeed())

			By("waiting for controller to process compliance check")
			time.Sleep(3 * time.Second)

			By("verifying compliance label was removed after check")
			verifyLabelRemoved := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "compliance-pass-test",
					"-o", "jsonpath={.metadata.labels['workspace\\.jupyter\\.org/compliance-check-needed']}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(BeEmpty())
			}
			Eventually(verifyLabelRemoved).
				WithPolling(500 * time.Millisecond).
				WithTimeout(15 * time.Second).
				Should(Succeed())

			By("verifying workspace status remains valid")
			cmd = exec.Command("kubectl", "get", "workspace", "compliance-pass-test",
				"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("True"), "workspace should remain Valid after passing compliance check")

			By("verifying ComplianceCheckPassed event was recorded")
			cmd = exec.Command("kubectl", "get", "events",
				"--field-selector", "involvedObject.name=compliance-pass-test",
				"-o", "jsonpath={.items[*].reason}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("ComplianceCheckPassed"))

			By("cleaning up workspace and template")
			cmd = exec.Command("kubectl", "delete", "workspace", "compliance-pass-test")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "constraint-violation-template")
			_, _ = utils.Run(cmd)
		})

		It("should handle multiple workspaces when marking for compliance", func() {
			By("creating compliance template")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating multiple workspaces using the template")
			workspaceNames := []string{}
			for i := 1; i <= 3; i++ {
				workspaceName := fmt.Sprintf("compliance-multi-test-%d", i)
				workspaceNames = append(workspaceNames, workspaceName)
				workspaceYaml := fmt.Sprintf(`apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: %s
spec:
  displayName: "Compliance Multi Test %d"
  templateRef:
    name: constraint-violation-template
  image: "quay.io/jupyter/minimal-notebook:latest"`, workspaceName, i)
				cmd = exec.Command("sh", "-c",
					fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
				_, err = utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred())
			}

			By("waiting for all workspaces to become valid")
			for _, wsName := range workspaceNames {
				verifyValid := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "workspace", wsName,
						"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(Equal("True"))
				}
				Eventually(verifyValid).
					WithPolling(1 * time.Second).
					WithTimeout(15 * time.Second).
					Should(Succeed())
			}

			By("updating template to trigger compliance check for all workspaces")
			patchCmd := `{"spec":{"description":"Changed to trigger compliance check"}}`
			cmd = exec.Command("kubectl", "patch", "workspacetemplate", "constraint-violation-template",
				"--type=merge", "-p", patchCmd)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying all workspaces received compliance label")
			for _, wsName := range workspaceNames {
				verifyLabelAdded := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "workspace", wsName,
						"-o", "jsonpath={.metadata.labels['workspace\\.jupyter\\.org/compliance-check-needed']}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(Equal("true"))
				}
				Eventually(verifyLabelAdded).
					WithPolling(500 * time.Millisecond).
					WithTimeout(10 * time.Second).
					Should(Succeed())
			}

			By("waiting for controller to process all compliance checks")
			time.Sleep(5 * time.Second)

			By("verifying all compliance labels were removed")
			for _, wsName := range workspaceNames {
				verifyLabelRemoved := func(g Gomega) {
					cmd := exec.Command("kubectl", "get", "workspace", wsName,
						"-o", "jsonpath={.metadata.labels['workspace\\.jupyter\\.org/compliance-check-needed']}")
					output, err := utils.Run(cmd)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(BeEmpty())
				}
				Eventually(verifyLabelRemoved).
					WithPolling(500 * time.Millisecond).
					WithTimeout(20 * time.Second).
					Should(Succeed())
			}

			By("cleaning up all workspaces and template")
			for _, wsName := range workspaceNames {
				cmd = exec.Command("kubectl", "delete", "workspace", wsName)
				_, _ = utils.Run(cmd)
			}
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "constraint-violation-template")
			_, _ = utils.Run(cmd)
		})

		It("should allow stopping workspace without template validation", func() {
			By("creating a template with image constraints")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating a workspace with allowed image")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace is initially valid")
			verifyInitiallyValid := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyInitiallyValid).
				WithPolling(1 * time.Second).
				WithTimeout(15 * time.Second).
				Should(Succeed())

			By("updating template to remove workspace's image from allowed list")
			patchCmd := `{"spec":{"allowedImages":["quay.io/jupyter/minimal-notebook:latest"]}}`
			cmd = exec.Command("kubectl", "patch", "workspacetemplate", "constraint-violation-template",
				"--type=merge", "-p", patchCmd)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for compliance check to fail")
			verifyComplianceFailed := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("False"))
			}
			Eventually(verifyComplianceFailed).
				WithPolling(1 * time.Second).
				WithTimeout(20 * time.Second).
				Should(Succeed())

			By("stopping the workspace despite being non-compliant")
			patchCmd = `{"spec":{"desiredStatus":"Stopped"}}`
			cmd = exec.Command("kubectl", "patch", "workspace", "constraint-violation-test",
				"--type=merge", "-p", patchCmd)
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Should allow stopping workspace without validation. Output: %s", output)

			By("verifying workspace transitions to Stopped status")
			verifyStatusStopped := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
					"-o", "jsonpath={.status.status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Stopped"))
			}
			Eventually(verifyStatusStopped).
				WithPolling(1 * time.Second).
				WithTimeout(15 * time.Second).
				Should(Succeed())

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "workspace", "constraint-violation-test")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "constraint-violation-template")
			_, _ = utils.Run(cmd)
		})

		It("should allow templateRef deletion without validation", func() {
			By("creating a template with image constraints")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating a workspace with template")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace is initially valid")
			verifyInitiallyValid := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyInitiallyValid).
				WithPolling(1 * time.Second).
				WithTimeout(15 * time.Second).
				Should(Succeed())

			By("updating template to make workspace's image non-compliant")
			patchCmd := `{"spec":{"allowedImages":["quay.io/jupyter/minimal-notebook:latest"]}}`
			cmd = exec.Command("kubectl", "patch", "workspacetemplate", "constraint-violation-template",
				"--type=merge", "-p", patchCmd)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for compliance check to fail")
			verifyComplianceFailed := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("False"))
			}
			Eventually(verifyComplianceFailed).
				WithPolling(1 * time.Second).
				WithTimeout(20 * time.Second).
				Should(Succeed())

			By("deleting templateRef to transition workspace to standalone")
			patchCmd = `{"spec":{"templateRef":null}}`
			cmd = exec.Command("kubectl", "patch", "workspace", "constraint-violation-test",
				"--type=merge", "-p", patchCmd)
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Should allow templateRef deletion without validation. Output: %s", output)

			By("verifying templateRef was removed")
			cmd = exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
				"-o", "jsonpath={.spec.templateRef}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty(), "TemplateRef should be empty")

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "workspace", "constraint-violation-test")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "constraint-violation-template")
			_, _ = utils.Run(cmd)
		})

		It("should validate against new template when templateRef changes", func() {
			By("creating two templates with different image constraints")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-template-b.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace using template A with scipy-notebook image")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace is initially valid")
			verifyInitiallyValid := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyInitiallyValid).
				WithPolling(1 * time.Second).
				WithTimeout(15 * time.Second).
				Should(Succeed())

			By("attempting to change templateRef to template B (scipy-notebook not allowed)")
			patchCmd := `{"spec":{"templateRef":{"name":"constraint-violation-template-b"}}}`
			cmd = exec.Command("kubectl", "patch", "workspace", "constraint-violation-test",
				"--type=merge", "-p", patchCmd)
			output, err := utils.Run(cmd)

			// Should fail because workspace uses scipy-notebook which is not in template B's allowedImages
			Expect(err).To(HaveOccurred(), "Should reject templateRef change when spec violates new template")
			Expect(output).To(ContainSubstring("workspace violates template"), "Error should mention template violation")

			By("verifying templateRef is still pointing to original template")
			cmd = exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
				"-o", "jsonpath={.spec.templateRef.name}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("constraint-violation-template"), "TemplateRef should be unchanged")

			By("updating workspace to use image compatible with template B")
			patchCmd = `{"spec":{"image":"quay.io/jupyter/datascience-notebook:latest"}}`
			cmd = exec.Command("kubectl", "patch", "workspace", "constraint-violation-test",
				"--type=merge", "-p", patchCmd)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Should allow image update to compatible image")

			By("now changing templateRef to template B should succeed")
			patchCmd = `{"spec":{"templateRef":{"name":"constraint-violation-template-b"}}}`
			cmd = exec.Command("kubectl", "patch", "workspace", "constraint-violation-test",
				"--type=merge", "-p", patchCmd)
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Should allow templateRef change when spec is compatible. Output: %s", output)

			By("verifying templateRef changed to template B")
			cmd = exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
				"-o", "jsonpath={.spec.templateRef.name}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("constraint-violation-template-b"), "TemplateRef should be updated")

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "workspace", "constraint-violation-test")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "constraint-violation-template")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "constraint-violation-template-b")
			_, _ = utils.Run(cmd)
		})

		It("should validate entire spec when any spec field changes", func() {
			By("creating a template with resource and image constraints")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating a compliant workspace")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/compliance-tracking/constraint-violation-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace is initially valid")
			verifyInitiallyValid := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyInitiallyValid).
				WithPolling(1 * time.Second).
				WithTimeout(15 * time.Second).
				Should(Succeed())

			By("updating template to remove workspace's image from allowed list")
			patchCmd := `{"spec":{"allowedImages":["quay.io/jupyter/minimal-notebook:latest"]}}`
			cmd = exec.Command("kubectl", "patch", "workspacetemplate", "constraint-violation-template",
				"--type=merge", "-p", patchCmd)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for compliance check to fail")
			verifyComplianceFailed := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
					"-o", "jsonpath={.status.conditions[?(@.type==\"Valid\")].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("False"))
			}
			Eventually(verifyComplianceFailed).
				WithPolling(1 * time.Second).
				WithTimeout(20 * time.Second).
				Should(Succeed())

			By("attempting to change displayName (which should trigger full spec validation)")
			patchCmd = `{"spec":{"displayName":"Updated Name"}}`
			cmd = exec.Command("kubectl", "patch", "workspace", "constraint-violation-test",
				"--type=merge", "-p", patchCmd)
			output, err := utils.Run(cmd)

			// Should fail because entire spec (including image) is validated
			Expect(err).To(HaveOccurred(), "Should reject update because full spec validation detects non-compliant image")
			Expect(output).To(ContainSubstring("workspace violates template"), "Error should mention template violation")

			By("verifying workspace is still non-compliant and displayName unchanged")
			cmd = exec.Command("kubectl", "get", "workspace", "constraint-violation-test",
				"-o", "jsonpath={.spec.displayName}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Constraint Violation Test"), "DisplayName should be unchanged")

			By("cleaning up")
			cmd = exec.Command("kubectl", "delete", "workspace", "constraint-violation-test")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "constraint-violation-template")
			_, _ = utils.Run(cmd)
		})
	})
})
