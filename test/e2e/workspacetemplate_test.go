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
			"--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)

		By("cleaning up test templates")
		_, _ = fmt.Fprintf(GinkgoWriter, "Deleting test templates...\n")
		// Templates with lazy finalizers will only delete after workspaces are gone
		// Using --wait=false allows kubectl to return immediately while deletion proceeds
		cmd = exec.Command("kubectl", "delete", "workspacetemplate",
			"production-notebook-template", "restricted-template",
			"mutability-test-template", "restricted-template-mutability",
			"lazy-application-template", "compliance-template",
			"--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		_, _ = fmt.Fprintf(GinkgoWriter, "Undeploying controller manager...\n")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CRDs...\n")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)
	})

	// deleteAllWorkspacesUsingTemplate deletes all workspaces that reference a specific template
	// using label-based lookup for dynamic discovery
	var deleteAllWorkspacesUsingTemplate = func(templateName string) {
		By("deleting all workspaces using template: " + templateName)
		cmd := exec.Command("kubectl", "get", "workspace",
			"-l", "workspace.jupyter.org/template="+templateName,
			"-o", "jsonpath={.items[*].metadata.name}")
		output, err := utils.Run(cmd)
		if err != nil || strings.TrimSpace(output) == "" {
			_, _ = fmt.Fprintf(GinkgoWriter, "No workspaces found using template %s\n", templateName)
			return
		}
		workspaces := strings.Fields(output)
		_, _ = fmt.Fprintf(GinkgoWriter, "Deleting %d workspace(s): %v\n", len(workspaces), workspaces)
		cmd = exec.Command("kubectl", "delete", "workspace")
		cmd.Args = append(cmd.Args, workspaces...)
		cmd.Args = append(cmd.Args, "--ignore-not-found")
		_, _ = utils.Run(cmd)
	}

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

		It("should log template resolution and validation", func() {
			By("getting controller pod name")
			cmd := exec.Command("kubectl", "get",
				"pods", "-l", "control-plane=controller-manager",
				"-o", "go-template={{ range .items }}"+
					"{{ if not .metadata.deletionTimestamp }}"+
					"{{ .metadata.name }}"+
					"{{ \"\\n\" }}{{ end }}{{ end }}",
				"-n", namespace,
			)
			podOutput, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			podNames := utils.GetNonEmptyLines(podOutput)
			Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
			controllerPodName := podNames[0]

			By("checking controller logs for template resolution")
			cmd = exec.Command("kubectl", "logs", controllerPodName, "-n", namespace,
				"--tail=500")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(ContainSubstring("Resolving template"))
			Expect(output).To(ContainSubstring("Template resolved and validated successfully"))
		})
	})

	Context("Template Validation", func() {
		It("should reject workspace with image not in allowlist", func() {
			By("attempting to create workspace with invalid image")
			cmd := exec.Command("sh", "-c", `echo 'apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: test-rejected-workspace
  namespace: default
spec:
  displayName: "Test Rejected Workspace"
  desiredStatus: Running
  templateRef:
    name: "restricted-template"
  image: "tensorflow/tensorflow:latest-gpu-jupyter"
  ownershipType: Public
  resources:
    requests:
      cpu: "4"
      memory: "8Gi"
    limits:
      cpu: "8"
      memory: "16Gi"
  storage:
    size: "50Gi"' | kubectl apply -f -`)
			
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
			workspaceYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: cpu-exceed-test
spec:
  displayName: "CPU Bounds Test"
  templateRef:
    name: "production-notebook-template"
  ownershipType: Public
  resources:
    requests:
      cpu: "10"  # Exceeds template max of 2
`
			cmd := exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
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
			workspaceYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: valid-overrides-test
spec:
  displayName: "Valid Overrides Test"
  templateRef:
    name: "production-notebook-template"
  ownershipType: Public
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "200m"
      memory: "256Mi"
  storage:
    size: 100Mi
`
			cmd := exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
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
			workspaceYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: deletion-protection-test
spec:
  displayName: "Deletion Protection Test"
  ownershipType: Public
  templateRef:
    name: "production-notebook-template"
`
			cmd := exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace has templateRef set")
			cmd = exec.Command("kubectl", "get", "workspace", "deletion-protection-test",
				"-o", "jsonpath={.spec.templateRef.name}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("production-notebook-template"))

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
			restrictedTemplateYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceTemplate
metadata:
  name: restricted-template-mutability
spec:
  displayName: "Restricted Template"
  description: "Restricted template for mutability testing"
  defaultOwnershipType: Public
  allowedImages:
    - "jk8s-application-jupyter-uv:latest"
  defaultImage: "jk8s-application-jupyter-uv:latest"
`
			cmd = exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", restrictedTemplateYaml))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace using production template")
			workspaceYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: templateref-mutability-test
spec:
  displayName: "TemplateRef Mutability Test"
  ownershipType: Public
  templateRef:
    name: production-notebook-template
`
			cmd = exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
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

<<<<<<< HEAD

=======
		It("should allow WorkspaceTemplate spec modification (mutability)", func() {
			By("creating a template for mutability testing")
			templateYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceTemplate
metadata:
  name: mutability-test-template
spec:
  displayName: "Mutability Test Template"
  description: "Original description"
  defaultOwnershipType: Public
  allowedImages:
    - "jk8s-application-jupyter-uv:latest"
  defaultImage: "jk8s-application-jupyter-uv:latest"
`
			cmd := exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", templateYaml))
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
>>>>>>> 4aaaf7b (make templates mutable, remove template enforcement from controller logic, add unit and e2e tests)
	})

	Context("Webhook Validation", func() {
		It("should apply template defaults during workspace creation", func() {
			By("creating workspace without specifying image, resources, or storage")
			workspaceYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: webhook-defaults-test
spec:
  displayName: "Webhook Defaults Test"
  templateRef:
    name: "production-notebook-template"
  ownershipType: Public
`
			cmd := exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
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

		It("should reject workspace creation with multiple violations", func() {
			By("attempting to create workspace with multiple template violations")
			workspaceYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: multi-violation-test
spec:
  displayName: "Multi Violation Test"
  templateRef:
    name: "restricted-template"
  image: "invalid/image:latest"
  resources:
    requests:
      cpu: "10"
      memory: "20Gi"
  storage:
    size: "100Gi"
`
			cmd := exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
			output, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Expected webhook to reject workspace with multiple violations")
			Expect(output).To(ContainSubstring("violations"))

			By("verifying workspace was not created")
			cmd = exec.Command("kubectl", "get", "workspace", "multi-violation-test", "--ignore-not-found")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty())
		})
	})
})
