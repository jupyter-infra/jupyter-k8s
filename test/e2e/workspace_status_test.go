//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/


package e2e

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

const (
	runningWorkspace    = "status-test-running"
	stoppedWorkspace    = "status-test-stopped"
	transitionWorkspace = "status-test-transition"
	testTimeout         = 60 * time.Second
	testPolling         = 3 * time.Second
)

var _ = Describe("Workspace Status Transitions", Ordered, func() {
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
		}).WithTimeout(testTimeout).WithPolling(testPolling).Should(Succeed())
	}

	BeforeAll(func() {
		By("applying production template for tests")
		cmd := exec.Command("kubectl", "apply", "-f",
			"config/samples/workspace_v1alpha1_workspacetemplate_production.yaml")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to create production template")
	})

	AfterAll(func() {
		By("cleaning up test workspaces")
		cmd := exec.Command("kubectl", "delete", "workspace",
			runningWorkspace, stoppedWorkspace, transitionWorkspace,
			"--ignore-not-found", "--timeout=60s")
		_, _ = utils.Run(cmd)

		By("cleaning up test templates")
		cmd = exec.Command("kubectl", "delete", "workspacetemplate",
			"production-notebook-template",
			"--ignore-not-found", "--wait=false")
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
			verifyDesiredStatus(runningWorkspace, "Running")

			By("verifying Deployment is created")
			verifyResourceExists("deployment", controller.GenerateDeploymentName(runningWorkspace), true)

			By("verifying Service is created")
			verifyResourceExists("service", controller.GenerateServiceName(runningWorkspace), true)

			By("verifying Available condition is False initially")
			verifyStatusCondition(runningWorkspace, controller.ConditionTypeAvailable, "False")

			By("verifying Progressing condition is True initially")
			verifyStatusCondition(runningWorkspace, controller.ConditionTypeProgressing, "True")
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
			verifyDesiredStatus(stoppedWorkspace, "Stopped")

			By("verifying Available condition is False")
			verifyStatusCondition(stoppedWorkspace, controller.ConditionTypeAvailable, "False")

			By("verifying no resources are created")
			verifyResourceExists("deployment", controller.GenerateDeploymentName(stoppedWorkspace), false)
			verifyResourceExists("service", controller.GenerateServiceName(stoppedWorkspace), false)
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
			verifyResourceExists("deployment", controller.GenerateDeploymentName(transitionWorkspace), true)
			verifyResourceExists("service", controller.GenerateServiceName(transitionWorkspace), true)

			By("verifying initial state: Available=False, Progressing=True")
			verifyStatusCondition(transitionWorkspace, controller.ConditionTypeAvailable, "False")
			verifyStatusCondition(transitionWorkspace, controller.ConditionTypeProgressing, "True")

			By("changing desiredStatus to Stopped")
			cmd = exec.Command("kubectl", "patch", "workspace", transitionWorkspace,
				"--type=merge", "-p", `{"spec":{"desiredStatus":"Stopped"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying desiredStatus changed to Stopped")
			verifyDesiredStatus(transitionWorkspace, "Stopped")

			By("verifying resources are deleted")
			verifyResourceExists("deployment", controller.GenerateDeploymentName(transitionWorkspace), false)
			verifyResourceExists("service", controller.GenerateServiceName(transitionWorkspace), false)
		})

		It("should transition from Stopped to Running and create resources", func() {
			By("changing desiredStatus to Running")
			cmd := exec.Command("kubectl", "patch", "workspace", stoppedWorkspace,
				"--type=merge", "-p", `{"spec":{"desiredStatus":"Running"}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying desiredStatus changed to Running")
			verifyDesiredStatus(stoppedWorkspace, "Running")

			By("verifying resources are created")
			verifyResourceExists("deployment", controller.GenerateDeploymentName(stoppedWorkspace), true)
			verifyResourceExists("service", controller.GenerateServiceName(stoppedWorkspace), true)

			By("verifying transition state: Available=False, Progressing=True")
			verifyStatusCondition(stoppedWorkspace, controller.ConditionTypeAvailable, "False")
			verifyStatusCondition(stoppedWorkspace, controller.ConditionTypeProgressing, "True")
		})
	})
})
