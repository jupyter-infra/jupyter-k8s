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

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
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

			// NOTE: We intentionally do NOT assert Progressing=True immediately after creation.
			// The controller sets .status.conditions asynchronously, so on slow/loaded CI runners
			// the conditions may still be empty when checked, causing flaky failures.
			// See https://github.com/jupyter-infra/jupyter-k8s/issues/350
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
				ConditionTypeDeleting:    ConditionFalse,
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
				ConditionTypeDeleting:    ConditionFalse,
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
				ConditionTypeDeleting:    ConditionFalse,
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
				ConditionTypeDeleting:    ConditionFalse,
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

	Context("DesiredStatus Defaulting", func() {
		const noDesiredStatusWorkspace = "workspace-no-desired-status"

		It("should default desiredStatus to Running when not specified", func() {
			By("creating workspace without desiredStatus field")
			createWorkspaceForTest(noDesiredStatusWorkspace, statusGroupDir, statusSubgroupDir)

			By("verifying spec.desiredStatus was defaulted to Running by the webhook")
			desiredStatus, err := kubectlGet("workspace", noDesiredStatusWorkspace, statusTestNamespace,
				"{.spec.desiredStatus}")
			Expect(err).NotTo(HaveOccurred())
			Expect(desiredStatus).To(Equal("Running"), "desiredStatus should be defaulted to Running by the mutating webhook")
		})
	})

	Context("Start steps", func() {
		It("should report only known steps on Progressing while a workspace starts", func() {
			By("creating workspace with desiredStatus=Running")
			createWorkspaceForTest(runningWorkspace, statusGroupDir, statusSubgroupDir)

			By("recording every Progressing reason on the way to Available")
			reasons, degradedSeen := progressingReasonsUntilAvailable(runningWorkspace, statusTestNamespace)
			for reason := range reasons {
				Expect(reason).To(BeElementOf(
					controller.ReasonResourcesNotReady, controller.ReasonComputeNotReady, controller.ReasonServiceNotReady,
					controller.ReasonAccessNotReady, controller.ReasonWaitingForNode, controller.ReasonPullingImage,
					controller.ReasonStartingContainer), "unexpected Progressing reason during a start")
			}
			Expect(degradedSeen).NotTo(HaveKey(ConditionTrue), "a normal start must never read Degraded")
			Expect(reasons).To(SatisfyAny(HaveKey(controller.ReasonWaitingForNode), HaveKey(controller.ReasonPullingImage),
				HaveKey(controller.ReasonStartingContainer)), "a start must show at least one step")
		})
	})

	Context("Degraded State", func() {
		const (
			unpullableWorkspace = "workspace-unpullable-image"
			unpullableImage     = "jk8s-e2e-missing-image"
			crashLoopWorkspace  = "workspace-crash-loop"
		)
		// The e2e operator runs application images with pull policy Never, so a missing image reads
		// ErrImageNeverPull there and ErrImagePull or ImagePullBackOff on a cluster that pulls.
		imagePullReasons := []string{"ErrImagePull", "ImagePullBackOff", "ErrImageNeverPull"}

		It("should report Degraded as soon as the image cannot be pulled, clear it on stop, and start once fixed", func() {
			By("creating a workspace whose image exists nowhere")
			createWorkspaceForTest(unpullableWorkspace, statusGroupDir, statusSubgroupDir)

			By("waiting for Degraded=True with the kubelet's pull reason naming the image, before any deadline")
			waitForWorkspaceStartFailed(unpullableWorkspace, statusTestNamespace, imagePullReasons,
				ContainSubstring(unpullableImage))
			expectSingleStartFailedEvent(unpullableWorkspace, statusTestNamespace, ContainSubstring(unpullableImage))
			VerifyWorkspaceConditions(unpullableWorkspace, statusTestNamespace, map[string]string{
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionTrue,
				ConditionTypeAvailable:   ConditionFalse,
				ConditionTypeStopped:     ConditionFalse,
				ConditionTypeDeleting:    ConditionFalse,
			})

			By("stopping the workspace")
			UpdateWorkspaceDesiredState(unpullableWorkspace, statusTestNamespace, "Stopped")
			WaitForWorkspaceToReachCondition(unpullableWorkspace, statusTestNamespace, ConditionTypeStopped, ConditionTrue)
			VerifyWorkspaceConditions(unpullableWorkspace, statusTestNamespace, map[string]string{
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeAvailable:   ConditionFalse,
				ConditionTypeStopped:     ConditionTrue,
				ConditionTypeDeleting:    ConditionFalse,
			})

			By("fixing the image and starting the workspace again")
			cmd := exec.Command("kubectl", "patch", "workspace", unpullableWorkspace, "-n", statusTestNamespace,
				"--type=merge", "-p", `{"spec":{"image":"jk8s-application-jupyter-uv:latest","desiredStatus":"Running"}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			WaitForWorkspaceToReachCondition(unpullableWorkspace, statusTestNamespace, ConditionTypeAvailable, ConditionTrue)
			VerifyWorkspaceConditions(unpullableWorkspace, statusTestNamespace, map[string]string{
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeAvailable:   ConditionTrue,
				ConditionTypeStopped:     ConditionFalse,
				ConditionTypeDeleting:    ConditionFalse,
			})
		})

		It("should report Degraded as soon as the container crash-loops, and start once the command is removed", func() {
			By("creating a workspace whose command exits at once")
			createWorkspaceForTest(crashLoopWorkspace, statusGroupDir, statusSubgroupDir)

			By("waiting for Degraded=True with reason CrashLoopBackOff and the kubelet's back-off message")
			waitForWorkspaceStartFailed(crashLoopWorkspace, statusTestNamespace, []string{"CrashLoopBackOff"},
				ContainSubstring("back-off"))
			expectSingleStartFailedEvent(crashLoopWorkspace, statusTestNamespace, ContainSubstring("CrashLoopBackOff"))

			By("stopping the workspace, removing the command and starting it again")
			UpdateWorkspaceDesiredState(crashLoopWorkspace, statusTestNamespace, "Stopped")
			WaitForWorkspaceToReachCondition(crashLoopWorkspace, statusTestNamespace, ConditionTypeStopped, ConditionTrue)
			cmd := exec.Command("kubectl", "patch", "workspace", crashLoopWorkspace, "-n", statusTestNamespace,
				"--type=merge", "-p", `{"spec":{"containerConfig":null,"desiredStatus":"Running"}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			WaitForWorkspaceToReachCondition(crashLoopWorkspace, statusTestNamespace, ConditionTypeAvailable, ConditionTrue)
			VerifyWorkspaceConditions(crashLoopWorkspace, statusTestNamespace, map[string]string{
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeAvailable:   ConditionTrue,
				ConditionTypeStopped:     ConditionFalse,
				ConditionTypeDeleting:    ConditionFalse,
			})
		})
	})

	Context("Deleting State", func() {
		const deletionWorkspace = "workspace-deletion-test"

		It("should set Deleting=True and Available=False when workspace is deleted", func() {
			By("creating workspace with desiredStatus=Running")
			createWorkspaceForTest(deletionWorkspace, statusGroupDir, statusSubgroupDir)

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				deletionWorkspace,
				statusTestNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("deleting the workspace")
			cmd := exec.Command("kubectl", "delete", "workspace", deletionWorkspace,
				"-n", statusTestNamespace, "--wait=false")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Deleting condition to become True")
			WaitForWorkspaceToReachCondition(
				deletionWorkspace,
				statusTestNamespace,
				ConditionTypeDeleting,
				ConditionTrue,
			)

			By("verifying Available=False while Deleting=True")
			VerifyWorkspaceConditions(deletionWorkspace, statusTestNamespace, map[string]string{
				ConditionTypeAvailable:   ConditionFalse,
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeStopped:     ConditionFalse,
				ConditionTypeDeleting:    ConditionTrue,
			})

			By("waiting for workspace to be fully deleted")
			WaitForWorkspaceDeletion(deletionWorkspace, statusTestNamespace)
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
