//go:build e2e
// +build e2e

/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/


package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

var _ = Describe("WorkspaceTemplate", Ordered, func() {
	// Helper functions for workspace management
	findWorkspacesUsingTemplate := func(templateName string) ([]string, error) {
		// Use label selector to find workspaces by template
		// Labels persist during deletion, unlike spec.templateRef which gets cleared
		labelSelector := fmt.Sprintf("workspace.jupyter.org/template-name=%s", templateName)
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
		var cmd *exec.Cmd
		var err error

		By("installing WorkspaceTemplate samples")
		_, _ = fmt.Fprintf(GinkgoWriter, "Creating jupyter-k8s-shared namespace...\n")
		cmd = exec.Command("kubectl", "create", "namespace", "jupyter-k8s-shared")
		_, _ = utils.Run(cmd) // Ignore error if namespace already exists

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
			"lazy-application-test", "cel-immutability-test",
			"security-context-test", "owner-workspace", "volume-ownership-allowed-test",
			"--ignore-not-found", "--timeout=60s")
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
			"lazy-application-template",
			"immutability-test-template",
			"--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)
	})

	Context("Template Creation and Usage", func() {
		It("should create WorkspaceTemplate successfully", func() {
			By("verifying production template exists")
			verifyTemplateExists := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspacetemplate",
					"production-notebook-template", "-n", "jupyter-k8s-shared", "-o", "jsonpath={.metadata.name}")
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
				"production-notebook-template", "-n", "jupyter-k8s-shared", "-o", "jsonpath={.spec.displayName}")
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
					"-o", "jsonpath={.metadata.labels.workspace\\.jupyter\\.org/template-name}")
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
					"production-notebook-template", "-n", "jupyter-k8s-shared",
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
				"production-notebook-template", "-n", "jupyter-k8s-shared", "--wait=false")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying template still exists with deletionTimestamp set")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspacetemplate",
					"production-notebook-template", "-n", "jupyter-k8s-shared",
					"-o", "jsonpath={.metadata.deletionTimestamp}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty(), "expected deletionTimestamp to be set")
			}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			By("verifying finalizer is blocking deletion")
			cmd = exec.Command("kubectl", "get", "workspacetemplate",
				"production-notebook-template", "-n", "jupyter-k8s-shared",
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
					"production-notebook-template", "-n", "jupyter-k8s-shared")
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
				"-o", "jsonpath={.metadata.labels['workspace\\.jupyter\\.org/template-name']}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("production-notebook-template"))

			By("cleaning up test workspace")
			cmd = exec.Command("kubectl", "delete", "workspace", "webhook-defaults-test")
			_, _ = utils.Run(cmd)
		})

		It("should inherit default access strategy from template", func() {
			By("creating a test access strategy")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/access-strategy.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating a template with default access strategy")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/access-strategy-template.yaml")
			_, err = utils.Run(cmd)
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
			cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy", "sample-access-strategy", "--ignore-not-found")
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
				"-o", "jsonpath={.spec.idleShutdown.idleTimeoutInMinutes}")
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

		It("should reject workspace referencing PVC owned by another workspace", func() {
			By("creating first workspace to own a PVC")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/owner-workspace.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			ownerPVCName := controller.GeneratePVCName("owner-workspace")
			By(fmt.Sprintf("waiting for first workspace PVC %s to be created with owner reference", ownerPVCName))
			Eventually(func(g Gomega) {
				// Check PVC exists
				cmd := exec.Command("kubectl", "get", "pvc", ownerPVCName, "-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal(ownerPVCName))

				// Check PVC has owner reference to the workspace
				cmd = exec.Command("kubectl", "get", "pvc", ownerPVCName, 
					"-o", "jsonpath={.metadata.ownerReferences[0].name}")
				output, err = utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("owner-workspace"))
			}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("attempting to create second workspace that references the first workspace's PVC")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/volume-ownership-violation-workspace.yaml")
			output, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Expected webhook to reject workspace referencing PVC owned by another workspace")
			Expect(output).To(ContainSubstring("owned by another workspace"))

			By("verifying second workspace was not created")
			cmd = exec.Command("kubectl", "get", "workspace", "volume-ownership-violation-test", "--ignore-not-found")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty())

			By("cleaning up first workspace")
			cmd = exec.Command("kubectl", "delete", "workspace", "owner-workspace", "--wait=true")
			_, _ = utils.Run(cmd)
		})

		It("should allow workspace referencing unowned PVC", func() {
			By("creating standalone PVC not owned by any workspace")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/standalone-pvc.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for standalone PVC to be created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pvc", "standalone-test-pvc", "-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("standalone-test-pvc"))
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

			By("creating workspace that references the standalone PVC")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/volume-ownership-allowed-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Expected webhook to allow workspace referencing unowned PVC")

			By("verifying workspace was created successfully")
			cmd = exec.Command("kubectl", "get", "workspace", "volume-ownership-allowed-test", "-o", "jsonpath={.metadata.name}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("volume-ownership-allowed-test"))

			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "workspace", "volume-ownership-allowed-test", "--wait=true")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "pvc", "standalone-test-pvc", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should inherit pod security context from template", func() {
			By("creating template with pod security context")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/security-context-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace without pod security context")
			cmd = exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/webhook-validation/security-context-workspace.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace inherited pod security context")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "security-context-test",
					"-o", "jsonpath={.spec.podSecurityContext.fsGroup}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("1000"))
			}).WithTimeout(10 * time.Second).Should(Succeed())

			By("verifying runAsUser was inherited")
			cmd = exec.Command("kubectl", "get", "workspace", "security-context-test",
				"-o", "jsonpath={.spec.podSecurityContext.runAsUser}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("1000"))

			By("cleaning up test resources")
			cmd = exec.Command("kubectl", "delete", "workspace", "security-context-test", "--ignore-not-found")
			_, _ = utils.Run(cmd)
			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "security-context-template", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})
	})
})
