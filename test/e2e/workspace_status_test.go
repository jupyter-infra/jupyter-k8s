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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

const (
	statusGroupDir      = "status"
	statusSubgroupDir   = ""
	statusTestNamespace = "default"
	runningWorkspace    = "workspace-running"
	stoppedWorkspace    = "workspace-stopped"
	statusTestTimeout   = 60 * time.Second
	statusTestPolling   = 3 * time.Second
)

var _ = Describe("Workspace Status", Ordered, func() {
	AfterEach(func() {
		deleteResourcesForStatusTest()
	})

	Context("Running State", func() {

		It("should reach Available=True condition for desiredStatus=Running", func() {
			By("creating workspace with desiredStatus=Running")
			createWorkspaceForTest(runningWorkspace, statusGroupDir, statusSubgroupDir)

			By("checking initial conditions: Progressing=True, Degraded=False, Available=False, Stopped=False")
			VerifyWorkspaceConditions(runningWorkspace, statusTestNamespace, map[string]string{
				ConditionTypeProgressing: ConditionTrue,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeAvailable:   ConditionFalse,
				ConditionTypeStopped:     ConditionFalse,
			})

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				runningWorkspace,
				statusTestNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("checking final conditions: Progressing=False, Degraded=False, Available=True, Stopped=False")
			VerifyWorkspaceConditions(runningWorkspace, statusTestNamespace, map[string]string{
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeAvailable:   ConditionTrue,
				ConditionTypeStopped:     ConditionFalse,
			})
		})

		It("should create a deployment and .status.deploymentName should track its name", func() {
			By("creating workspace with desiredStatus=Running")
			createWorkspaceForTest(runningWorkspace, statusGroupDir, statusSubgroupDir)

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				runningWorkspace,
				statusTestNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying workspace.status.deploymentName is set")
			statusDeploymentName, err := kubectlGet("workspace", runningWorkspace, statusTestNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(statusDeploymentName).NotTo(BeEmpty(), "workspace.status.deploymentName should be set")

			By("verifying Deployment with that name exists")
			Expect(ResourceExists("deployment", statusDeploymentName, statusTestNamespace, "{.metadata.name}")).
				To(BeTrue(), "Deployment should exist with the name from workspace.status.deploymentName")
		})

		It("should create a service and .status.serviceName should track its name", func() {
			By("creating workspace with desiredStatus=Running")
			createWorkspaceForTest(runningWorkspace, statusGroupDir, statusSubgroupDir)

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				runningWorkspace,
				statusTestNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying workspace.status.serviceName is set")
			statusServiceName, err := kubectlGet("workspace", runningWorkspace, statusTestNamespace,
				"{.status.serviceName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(statusServiceName).NotTo(BeEmpty(), "workspace.status.serviceName should be set")

			By("verifying Service with that name exists")
			Expect(ResourceExists("service", statusServiceName, statusTestNamespace, "{.metadata.name}")).
				To(BeTrue(), "Service should exist with the name from workspace.status.serviceName")
		})

		It("should create exactly one pod with .status.condition[Ready]=True", func() {
			By("creating workspace with desiredStatus=Running")
			createWorkspaceForTest(runningWorkspace, statusGroupDir, statusSubgroupDir)

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				runningWorkspace,
				statusTestNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying exactly 1 pod exists")
			output, err := kubectlGetByLabels("pod",
				fmt.Sprintf("%s=%s", WorkspaceLabelName, runningWorkspace),
				statusTestNamespace, "{.items[*].metadata.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).NotTo(BeEmpty(), "pod should exist")

			podNames := strings.Fields(output)
			Expect(podNames).To(HaveLen(1), "exactly one pod should exist")

			By("verifying pod has Ready=True condition")
			podName := podNames[0]
			readyStatus, err := kubectlGet("pod", podName, statusTestNamespace,
				"{.status.conditions[?(@.type==\"Ready\")].status}")
			Expect(err).NotTo(HaveOccurred())
			Expect(readyStatus).To(Equal(ConditionTrue), "pod should be ready")
		})
	})

	Context("Stopped State", func() {

		It("should reach Stopped=True condition for desiredStatus=Stopped", func() {
			By("creating workspace with desiredStatus=Stopped")
			createWorkspaceForTest(stoppedWorkspace, statusGroupDir, statusSubgroupDir)

			By("waiting for Stopped condition to become True")
			WaitForWorkspaceToReachCondition(
				stoppedWorkspace,
				statusTestNamespace,
				ConditionTypeStopped,
				ConditionTrue,
			)

			By("verifying final conditions: Progressing=False, Degraded=False, Available=False, Stopped=True")
			VerifyWorkspaceConditions(stoppedWorkspace, statusTestNamespace, map[string]string{
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeAvailable:   ConditionFalse,
				ConditionTypeStopped:     ConditionTrue,
			})
		})

		It("should not have .status.deploymentName nor .status.serviceName for desiredStatus=Stopped", func() {
			By("creating workspace with desiredStatus=Stopped")
			createWorkspaceForTest(stoppedWorkspace, statusGroupDir, statusSubgroupDir)

			By("waiting for Stopped condition to become True")
			WaitForWorkspaceToReachCondition(
				stoppedWorkspace,
				statusTestNamespace,
				ConditionTypeStopped,
				ConditionTrue,
			)

			By("verifying workspace.status.deploymentName is empty")
			deploymentName, err := kubectlGet("workspace", stoppedWorkspace, statusTestNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).To(BeEmpty(), "workspace.status.deploymentName should be empty for stopped workspace")

			By("verifying workspace.status.serviceName is empty")
			serviceName, err := kubectlGet("workspace", stoppedWorkspace, statusTestNamespace,
				"{.status.serviceName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceName).To(BeEmpty(), "workspace.status.serviceName should be empty for stopped workspace")
		})

		It("should not create underlying resources when desiredStatus=Stopped", func() {
			By("creating workspace with desiredStatus=Stopped")
			createWorkspaceForTest(stoppedWorkspace, statusGroupDir, statusSubgroupDir)

			By("waiting for Stopped condition to become True")
			WaitForWorkspaceToReachCondition(
				stoppedWorkspace,
				statusTestNamespace,
				ConditionTypeStopped,
				ConditionTrue,
			)

			By("verifying no Deployment exists for this workspace")
			Consistently(func() string {
				output, _ := kubectlGetByLabels("deployment",
					fmt.Sprintf("%s=%s", WorkspaceLabelName, stoppedWorkspace),
					statusTestNamespace, "{.items[*].metadata.name}")
				return output
			}, "5s", "1s").Should(BeEmpty(), "no Deployment should exist for stopped workspace")

			By("verifying no Service exists for this workspace")
			Consistently(func() string {
				output, _ := kubectlGetByLabels("service",
					fmt.Sprintf("%s=%s", WorkspaceLabelName, stoppedWorkspace),
					statusTestNamespace, "{.items[*].metadata.name}")
				return output
			}, "5s", "1s").Should(BeEmpty(), "no Service should exist for stopped workspace")

			By("verifying no Pods exist for this workspace")
			output, err := kubectlGetByLabels("pod",
				fmt.Sprintf("%s=%s", WorkspaceLabelName, stoppedWorkspace),
				statusTestNamespace, "{.items[*].metadata.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty(), "no pod should exist for stopped workspace")
		})
	})

	Context("State Transitions", func() {

		It("should transition from Running to Stopped, update status and delete resources", func() {
			By("creating workspace with desiredStatus: Running")
			createWorkspaceForTest(runningWorkspace, statusGroupDir, statusSubgroupDir)

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				runningWorkspace,
				statusTestNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("retrieving deployment and service names from workspace status")
			deploymentName, err := kubectlGet("workspace", runningWorkspace, statusTestNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty(), "workspace.status.deploymentName should be set")

			serviceName, err := kubectlGet("workspace", runningWorkspace, statusTestNamespace,
				"{.status.serviceName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceName).NotTo(BeEmpty(), "workspace.status.serviceName should be set")

			By("verifying Deployment exists")
			Expect(ResourceExists("deployment", deploymentName, statusTestNamespace, "{.metadata.name}")).
				To(BeTrue(), "Deployment should exist")

			By("verifying Service exists")
			Expect(ResourceExists("service", serviceName, statusTestNamespace, "{.metadata.name}")).
				To(BeTrue(), "Service should exist")

			By("changing desiredStatus to Stopped")
			cmd := exec.Command("kubectl", "patch", "workspace", runningWorkspace,
				"-n", statusTestNamespace,
				"--type=merge", "-p", `{"spec":{"desiredStatus":"Stopped"}}`)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Stopped condition to become True")
			WaitForWorkspaceToReachCondition(
				runningWorkspace,
				statusTestNamespace,
				ConditionTypeStopped,
				ConditionTrue,
			)

			By("verifying Available=False, Progressing=False, Degraded=False, Stopped=True")
			VerifyWorkspaceConditions(runningWorkspace, statusTestNamespace, map[string]string{
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeAvailable:   ConditionFalse,
				ConditionTypeStopped:     ConditionTrue,
			})

			By("verifying .status.deploymentName is removed")
			deploymentNameAfterStop, err := kubectlGet("workspace", runningWorkspace, statusTestNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentNameAfterStop).To(BeEmpty(), "workspace.status.deploymentName should be empty after stopping")

			By("verifying .status.serviceName is removed")
			serviceNameAfterStop, err := kubectlGet("workspace", runningWorkspace, statusTestNamespace,
				"{.status.serviceName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceNameAfterStop).To(BeEmpty(), "workspace.status.serviceName should be empty after stopping")

			By("verifying Deployment is deleted")
			WaitForResourceToNotExist("deployment", deploymentName, statusTestNamespace,
				statusTestTimeout, statusTestPolling)

			By("verifying Service is deleted")
			WaitForResourceToNotExist("service", serviceName, statusTestNamespace,
				statusTestTimeout, statusTestPolling)
		})

		It("should transition from Stopped to Running, create resources and update status", func() {
			By("creating workspace with desiredStatus: Stopped")
			createWorkspaceForTest(stoppedWorkspace, statusGroupDir, statusSubgroupDir)

			By("waiting for Stopped condition to become True")
			WaitForWorkspaceToReachCondition(
				stoppedWorkspace,
				statusTestNamespace,
				ConditionTypeStopped,
				ConditionTrue,
			)

			By("changing desiredStatus to Running")
			cmd := exec.Command("kubectl", "patch", "workspace", stoppedWorkspace,
				"-n", statusTestNamespace,
				"--type=merge", "-p", `{"spec":{"desiredStatus":"Running"}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				stoppedWorkspace,
				statusTestNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying Available=True, Progressing=False, Degraded=False, Stopped=False")
			VerifyWorkspaceConditions(stoppedWorkspace, statusTestNamespace, map[string]string{
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeAvailable:   ConditionTrue,
				ConditionTypeStopped:     ConditionFalse,
			})

			By("retrieving deployment and service names from workspace status")
			deploymentName, err := kubectlGet("workspace", stoppedWorkspace, statusTestNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty(), "workspace.status.deploymentName should be set")

			serviceName, err := kubectlGet("workspace", stoppedWorkspace, statusTestNamespace,
				"{.status.serviceName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceName).NotTo(BeEmpty(), "workspace.status.serviceName should be set")

			By("verifying Deployment exists")
			Expect(ResourceExists("deployment", deploymentName, statusTestNamespace, "{.metadata.name}")).
				To(BeTrue(), "Deployment should exist")

			By("verifying Service exists")
			Expect(ResourceExists("service", serviceName, statusTestNamespace, "{.metadata.name}")).
				To(BeTrue(), "Service should exist")
		})
	})
})

func deleteResourcesForStatusTest() {
	By("cleaning up all workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", statusTestNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("waiting an arbitrary fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
