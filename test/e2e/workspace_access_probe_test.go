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

var _ = Describe("Workspace Access Startup Probe", Ordered, func() {
	const (
		workspaceNamespace    = "default"
		groupDir              = "access-probe"
		workspaceProbeSuccess = "workspace-probe-success"
		workspaceProbeFailure = "workspace-probe-failure"
	)

	AfterEach(func() {
		By("cleaning up workspaces")
		cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", workspaceNamespace,
			"--ignore-not-found", "--wait=true", "--timeout=120s")
		_, _ = utils.Run(cmd)

		By("cleaning up access strategies")
		cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy", "--all", "-n", SharedNamespace,
			"--ignore-not-found", "--wait=true", "--timeout=30s")
		_, _ = utils.Run(cmd)

		time.Sleep(1 * time.Second)
	})

	Context("Access startup probe succeeds", func() {
		It("should become Available after probe succeeds via ClusterIP service", func() {
			workspaceName := workspaceProbeSuccess

			By("creating an access strategy with probe targeting the ClusterIP service")
			createAccessStrategyForTest("access-strategy-probe-success", groupDir, "")

			By("creating a workspace referencing the access strategy")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying all conditions")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeDegraded:    ConditionFalse,
				controller.ConditionTypeAvailable:   ConditionTrue,
				controller.ConditionTypeStopped:     ConditionFalse,
			})

			// Verify the workspace stays Available and doesn't oscillate.
			// A previous bug caused probe success → status write → watch event →
			// re-reconcile → fresh probe cycle → Progressing, looping forever.
			By("verifying workspace remains Available for 1 minute (no oscillation)")
			VerifyConsistentWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeAvailable:   ConditionTrue,
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeDegraded:    ConditionFalse,
				controller.ConditionTypeStopped:     ConditionFalse,
			}, "1m", "5s")

			By("verifying accessStartupProbeFailures is cleared after success")
			probeFailures, _ := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessStartupProbeFailures}")
			Expect(probeFailures).To(BeEmpty())

			By("verifying accessURL is set")
			accessURL, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessURL}")
			Expect(err).NotTo(HaveOccurred())
			Expect(accessURL).To(Equal(
				fmt.Sprintf("https://example.com/workspaces/default/%s/", workspaceName)))
		})
	})

	Context("Access startup probe failure threshold exceeded", func() {
		It("should become Degraded after probe exceeds failureThreshold", func() {
			workspaceName := workspaceProbeFailure

			By("creating an access strategy with probe targeting unreachable URL (failureThreshold=3)")
			createAccessStrategyForTest("access-strategy-probe-failure", groupDir, "")

			By("creating a workspace referencing the access strategy")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become Degraded after failureThreshold is exceeded")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeDegraded,
				ConditionTrue,
			)

			By("verifying Degraded condition reason is AccessProbeThresholdExceeded")
			reason, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				fmt.Sprintf("{.status.conditions[?(@.type==\"%s\")].reason}",
					controller.ConditionTypeDegraded))
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(Equal(controller.ReasonAccessProbeThresholdExceeded))

			By("verifying workspace remains Degraded for 30 seconds (no oscillation)")
			VerifyConsistentWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeDegraded:    ConditionTrue,
				controller.ConditionTypeAvailable:   ConditionFalse,
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeStopped:     ConditionFalse,
			}, "30s", "5s")

			By("verifying accessStartupProbeFailures is retained at threshold")
			probeFailures, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessStartupProbeFailures}")
			Expect(err).NotTo(HaveOccurred())
			Expect(probeFailures).To(Equal("3"))
		})
	})

	Context("Probe reset on AccessStrategy update", func() {
		It("should self-heal from Degraded when AccessStrategy probe URL is fixed", func() {
			workspaceName := workspaceProbeFailure

			By("creating an access strategy with unreachable probe URL (failureThreshold=3)")
			createAccessStrategyForTest("access-strategy-probe-failure", groupDir, "")

			By("creating a workspace referencing the access strategy")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become Degraded")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeDegraded,
				ConditionTrue,
			)

			By("patching access strategy to point probe at the reachable ClusterIP service")
			patchAccessStrategy("access", "probe", "patch-probe-url-to-reachable", "access-strategy-probe-failure")

			By("waiting for the workspace to become Available after probe reset")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying all conditions after self-heal")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeDegraded:    ConditionFalse,
				controller.ConditionTypeAvailable:   ConditionTrue,
				controller.ConditionTypeStopped:     ConditionFalse,
			})

			By("verifying accessStartupProbeFailures is cleared after success")
			probeFailures, _ := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessStartupProbeFailures}")
			Expect(probeFailures).To(BeEmpty())
		})

		It("should re-probe and become Degraded when a Running workspace's strategy changes to unreachable", func() {
			workspaceName := workspaceProbeSuccess

			By("creating an access strategy with reachable probe")
			createAccessStrategyForTest("access-strategy-probe-success", groupDir, "")

			By("creating a workspace referencing the access strategy")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become Available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("patching access strategy to point probe at an unreachable URL (failureThreshold=3)")
			patchAccessStrategy("access", "probe", "patch-probe-url-to-unreachable", "access-strategy-probe-success")

			By("waiting for the workspace to become Degraded after probe reset")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeDegraded,
				ConditionTrue,
			)

			By("verifying Degraded condition reason")
			reason, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				fmt.Sprintf("{.status.conditions[?(@.type==\"%s\")].reason}",
					controller.ConditionTypeDegraded))
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(Equal(controller.ReasonAccessProbeThresholdExceeded))
		})
	})

	Context("Probe reset on accessStrategyRef switch", func() {
		It("should re-probe when workspace switches to a different AccessStrategy", func() {
			workspaceName := workspaceProbeSuccess

			By("creating both access strategies")
			createAccessStrategyForTest("access-strategy-probe-success", groupDir, "")
			createAccessStrategyForTest("access-strategy-probe-failure", groupDir, "")

			By("creating a workspace referencing the reachable access strategy")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become Available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("patching workspace to switch accessStrategyRef to unreachable strategy")
			cmd := exec.Command("kubectl", "patch", "workspace", workspaceName,
				"-n", workspaceNamespace, "--type=merge", "--patch-file",
				"test/e2e/static/access-probe/patch-workspace-switch-to-failure-strategy.json")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for the workspace to become Degraded after probe reset")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeDegraded,
				ConditionTrue,
			)

			By("verifying Degraded condition reason")
			reason, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				fmt.Sprintf("{.status.conditions[?(@.type==\"%s\")].reason}",
					controller.ConditionTypeDegraded))
			Expect(err).NotTo(HaveOccurred())
			Expect(reason).To(Equal(controller.ReasonAccessProbeThresholdExceeded))
		})
	})
})
