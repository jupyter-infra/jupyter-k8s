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

var _ = Describe("Workspace Status Transitions", Ordered, func() {
	var controllerPodName string

	// Helper functions for status verification
	verifyStatusCondition := func(workspaceName, conditionType, expectedStatus string) {
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "workspace", workspaceName,
				"-o", fmt.Sprintf("jsonpath={.status.conditions[?(@.type==\"%s\")].status}", conditionType))
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal(expectedStatus))
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
	}

	verifyDesiredStatus := func(workspaceName, expectedStatus string) {
		cmd := exec.Command("kubectl", "get", "workspace", workspaceName,
			"-o", "jsonpath={.spec.desiredStatus}")
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(Equal(expectedStatus))
	}

	verifyResourceExists := func(resourceType, resourceName string, shouldExist bool) {
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", resourceType, resourceName, "--ignore-not-found")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			if shouldExist {
				g.Expect(output).NotTo(BeEmpty(), fmt.Sprintf("%s %s should exist", resourceType, resourceName))
			} else {
				g.Expect(output).To(BeEmpty(), fmt.Sprintf("%s %s should not exist", resourceType, resourceName))
			}
		}).WithTimeout(60 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
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

		// Set environment variables for e2e testing
		By("setting controller environment variables")
		envVars := []string{
			"CONTROLLER_POD_SERVICE_ACCOUNT=jupyter-k8s-controller-manager",
			"CONTROLLER_POD_NAMESPACE=jupyter-k8s-system",
		}
		for _, envVar := range envVars {
			cmd = exec.Command("kubectl", "set", "env", "deployment/jupyter-k8s-controller-manager", envVar, "-n", namespace)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to set environment variable: %s", envVar))
		}

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
			// Verify webhook endpoint is reachable by attempting a dry-run resource creation
			cmd := exec.Command("kubectl", "apply", "--dry-run=server", "-f", "test/e2e/static/webhook-validation/webhook-readiness-test.yaml")
			_, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred(), "webhook server should respond to admission requests")
		}
		Eventually(verifyWebhookReady, 30*time.Second, 2*time.Second).Should(Succeed())

		By("installing WorkspaceTemplate samples")
		_, _ = fmt.Fprintf(GinkgoWriter, "Creating jupyter-k8s-shared namespace...\n")
		cmd = exec.Command("kubectl", "create", "namespace", "jupyter-k8s-shared")
		_, _ = utils.Run(cmd) // Ignore error if namespace already exists

		By("applying production template for tests")
		_, _ = fmt.Fprintf(GinkgoWriter, "Applying production template...\n")
		cmd = exec.Command("kubectl", "apply", "-f",
			"config/samples/workspace_v1alpha1_workspacetemplate_production.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create production template")
	})

	AfterAll(func() {
		By("cleaning up test workspaces")
		_, _ = fmt.Fprintf(GinkgoWriter, "Deleting test workspaces...\n")
		cmd := exec.Command("kubectl", "delete", "workspace",
			"status-test-running", "status-test-stopped", "status-test-transition",
			"--ignore-not-found", "--timeout=60s")
		_, _ = utils.Run(cmd)

		By("cleaning up test templates")
		_, _ = fmt.Fprintf(GinkgoWriter, "Deleting test templates...\n")
		cmd = exec.Command("kubectl", "delete", "workspacetemplate",
			"production-notebook-template",
			"--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		_, _ = fmt.Fprintf(GinkgoWriter, "Uninstalling CRDs...\n")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		_, _ = fmt.Fprintf(GinkgoWriter, "Undeploying controller manager...\n")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)
	})

	Context("Running State", func() {
		It("should create resources when desiredStatus is Running", func() {
			By("creating workspace with desiredStatus: Running")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/status-transitions/workspace-running.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying desiredStatus is Running")
			verifyDesiredStatus("status-test-running", "Running")

			By("verifying Valid condition becomes True")
			verifyStatusCondition("status-test-running", "Valid", "True")

			By("verifying Deployment is created")
			verifyResourceExists("deployment", "status-test-running", true)

			By("verifying Service is created")
			verifyResourceExists("service", "status-test-running", true)

			By("verifying Available condition becomes True")
			verifyStatusCondition("status-test-running", "Available", "True")

			By("cleaning up workspace")
			cmd = exec.Command("kubectl", "delete", "workspace", "status-test-running", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})
	})

	Context("Stopped State", func() {
		It("should not create resources when desiredStatus is Stopped", func() {
			By("creating workspace with desiredStatus: Stopped")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/status-transitions/workspace-stopped.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying desiredStatus is Stopped")
			verifyDesiredStatus("status-test-stopped", "Stopped")

			By("verifying Valid condition becomes True")
			verifyStatusCondition("status-test-stopped", "Valid", "True")

			By("waiting to ensure no resources are created")
			time.Sleep(10 * time.Second)

			By("verifying Deployment is not created")
			verifyResourceExists("deployment", "status-test-stopped", false)

			By("verifying Service is not created")
			verifyResourceExists("service", "status-test-stopped", false)

			By("verifying Available condition is False")
			verifyStatusCondition("status-test-stopped", "Available", "False")

			By("cleaning up workspace")
			cmd = exec.Command("kubectl", "delete", "workspace", "status-test-stopped", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})
	})

	Context("State Transitions", func() {
		It("should transition from Running to Stopped and delete resources", func() {
			By("creating workspace with desiredStatus: Running")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/status-transitions/workspace-transition.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for resources to be created")
			verifyResourceExists("deployment", "status-test-transition", true)
			verifyResourceExists("service", "status-test-transition", true)
			verifyStatusCondition("status-test-transition", "Available", "True")

			By("changing desiredStatus to Stopped")
			cmd = exec.Command("kubectl", "patch", "workspace", "status-test-transition",
				"--type=merge", "-p", `{"spec":{"desiredStatus":"Stopped"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying desiredStatus changed to Stopped")
			verifyDesiredStatus("status-test-transition", "Stopped")

			By("verifying Progressing condition becomes True during transition")
			verifyStatusCondition("status-test-transition", "Progressing", "True")

			By("verifying resources are deleted")
			verifyResourceExists("deployment", "status-test-transition", false)
			verifyResourceExists("service", "status-test-transition", false)

			By("verifying Available condition becomes False")
			verifyStatusCondition("status-test-transition", "Available", "False")

			By("verifying Progressing condition becomes False after transition")
			verifyStatusCondition("status-test-transition", "Progressing", "False")

			By("cleaning up workspace")
			cmd = exec.Command("kubectl", "delete", "workspace", "status-test-transition", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should transition from Stopped to Running and create resources", func() {
			By("creating workspace with desiredStatus: Stopped")
			cmd := exec.Command("kubectl", "apply", "-f",
				"test/e2e/static/status-transitions/workspace-stopped.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying no resources exist initially")
			verifyResourceExists("deployment", "status-test-stopped", false)
			verifyResourceExists("service", "status-test-stopped", false)
			verifyStatusCondition("status-test-stopped", "Available", "False")

			By("changing desiredStatus to Running")
			cmd = exec.Command("kubectl", "patch", "workspace", "status-test-stopped",
				"--type=merge", "-p", `{"spec":{"desiredStatus":"Running"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying desiredStatus changed to Running")
			verifyDesiredStatus("status-test-stopped", "Running")

			By("verifying Progressing condition becomes True during transition")
			verifyStatusCondition("status-test-stopped", "Progressing", "True")

			By("verifying resources are created")
			verifyResourceExists("deployment", "status-test-stopped", true)
			verifyResourceExists("service", "status-test-stopped", true)

			By("verifying Available condition becomes True")
			verifyStatusCondition("status-test-stopped", "Available", "True")

			By("verifying Progressing condition becomes False after transition")
			verifyStatusCondition("status-test-stopped", "Progressing", "False")

			By("cleaning up workspace")
			cmd = exec.Command("kubectl", "delete", "workspace", "status-test-stopped", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})
	})
})
