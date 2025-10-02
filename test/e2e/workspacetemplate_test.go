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

			cmd = exec.Command("kubectl", "get",
				"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
				"-n", namespace,
			)
			status, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(status).To(Equal("Running"), "expected controller pod to be Running")
		}
		Eventually(verifyControllerUp, 2*time.Minute).Should(Succeed())

		By("installing WorkspaceTemplate samples")
		_, _ = fmt.Fprintf(GinkgoWriter, "Applying production template...\n")
		cmd = exec.Command("kubectl", "apply", "-f",
			"config/samples/workspaces_v1alpha1_workspacetemplate_production.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create production template")

		_, _ = fmt.Fprintf(GinkgoWriter, "Applying restricted template for validation tests...\n")
		cmd = exec.Command("kubectl", "apply", "-f",
			"examples/test-template-rejection.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test templates")
	})

	AfterAll(func() {
		By("cleaning up test workspaces")
		_, _ = fmt.Fprintf(GinkgoWriter, "Deleting test workspaces...\n")
		cmd := exec.Command("kubectl", "delete", "workspace",
			"workspace-with-template", "test-rejected-workspace",
			"cpu-exceed-test", "valid-overrides-test",
			"--ignore-not-found")
		_, _ = utils.Run(cmd)

		By("cleaning up test templates")
		_, _ = fmt.Fprintf(GinkgoWriter, "Deleting test templates...\n")
		cmd = exec.Command("kubectl", "delete", "workspacetemplate",
			"production-notebook-template", "restricted-template",
			"--ignore-not-found")
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
				"config/samples/workspaces_v1alpha1_workspace_with_template.yaml")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying TemplateValidation condition is True")
			verifyTemplateValidation := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "workspace-with-template",
					"-o", "jsonpath={.status.conditions[?(@.type=='TemplateValidation')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyTemplateValidation).
				WithPolling(1 * time.Second).
				WithTimeout(10 * time.Second).
				Should(Succeed())

			By("verifying Degraded condition is False")
			cmd = exec.Command("kubectl", "get", "workspace", "workspace-with-template",
				"-o", "jsonpath={.status.conditions[?(@.type=='Degraded')].status}")
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
			Expect(output).To(ContainSubstring("Template validation passed"))
		})
	})

	Context("Template Validation", func() {
		It("should reject workspace with image not in allowlist", func() {
			By("verifying test-rejected-workspace was created from test-template-rejection.yaml")
			verifyWorkspaceExists := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "test-rejected-workspace",
					"-o", "jsonpath={.metadata.name}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("test-rejected-workspace"))
			}
			Eventually(verifyWorkspaceExists).
				WithPolling(1 * time.Second).
				WithTimeout(10 * time.Second).
				Should(Succeed())

			By("waiting for TemplateValidation condition to be False")
			verifyTemplateRejected := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "test-rejected-workspace",
					"-o", "jsonpath={.status.conditions[?(@.type=='TemplateValidation')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("False"))
			}
			Eventually(verifyTemplateRejected).
				WithPolling(1 * time.Second).
				WithTimeout(10 * time.Second).
				Should(Succeed())

			By("verifying Degraded condition is True")
			cmd := exec.Command("kubectl", "get", "workspace", "test-rejected-workspace",
				"-o", "jsonpath={.status.conditions[?(@.type=='Degraded')].status}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("True"))

			By("verifying NO pod was created for rejected workspace")
			cmd = exec.Command("kubectl", "get", "pods", "-l",
				"workspace.workspaces.jupyter.org/name=test-rejected-workspace",
				"-o", "jsonpath={.items}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("[]"))
		})

		It("should reject workspace exceeding CPU bounds", func() {
			var output string
			var err error

			By("creating workspace with CPU exceeding template max")
			workspaceYaml := `apiVersion: workspaces.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: cpu-exceed-test
spec:
  displayName: "CPU Bounds Test"
  templateRef: "production-notebook-template"
  templateOverrides:
    resources:
      requests:
        cpu: "10"
`
			cmd := exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace is rejected with PolicyViolation")
			verifyPolicyViolation := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "cpu-exceed-test",
					"-o", "jsonpath={.status.conditions[?(@.type=='Degraded')].reason}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("PolicyViolation"))
			}
			Eventually(verifyPolicyViolation).
				WithPolling(1 * time.Second).
				WithTimeout(10 * time.Second).
				Should(Succeed())

			By("verifying status message explains which bound was exceeded")
			cmd = exec.Command("kubectl", "get", "workspace", "cpu-exceed-test",
				"-o", "jsonpath={.status.conditions[?(@.type=='TemplateValidation')].message}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("cpu"))

			By("verifying NO pod was created")
			cmd = exec.Command("kubectl", "get", "pods", "-l",
				"workspace.workspaces.jupyter.org/name=cpu-exceed-test",
				"-o", "jsonpath={.items}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("[]"))
		})

		It("should accept workspace with valid overrides", func() {
			var output string
			var err error

			By("creating workspace with valid resource overrides")
			workspaceYaml := `apiVersion: workspaces.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: valid-overrides-test
spec:
  displayName: "Valid Overrides Test"
  templateRef: "production-notebook-template"
  storage:
    size: 5Gi
  templateOverrides:
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "500m"
        memory: "256Mi"
`
			cmd := exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying TemplateValidation condition is True")
			verifyTemplateValidation := func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspace", "valid-overrides-test",
					"-o", "jsonpath={.status.conditions[?(@.type=='TemplateValidation')].status}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("True"))
			}
			Eventually(verifyTemplateValidation).
				WithPolling(1 * time.Second).
				WithTimeout(10 * time.Second).
				Should(Succeed())

			By("verifying Degraded condition is False")
			cmd = exec.Command("kubectl", "get", "workspace", "valid-overrides-test",
				"-o", "jsonpath={.status.conditions[?(@.type=='Degraded')].status}")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("False"))
		})
	})
})
