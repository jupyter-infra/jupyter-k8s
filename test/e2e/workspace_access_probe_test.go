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
		workspaceNamespace = "default"
		groupDir           = "access-probe"
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
			workspaceName := "workspace-probe-success"

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
			Consistently(func() string {
				output, _ := kubectlGet("workspace", workspaceName, workspaceNamespace,
					fmt.Sprintf("{.status.conditions[?(@.type==\"%s\")].status}",
						controller.ConditionTypeAvailable))
				return output
			}, "1m", "5s").Should(Equal(ConditionTrue),
				"workspace should remain Available without oscillating")

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
			workspaceName := "workspace-probe-failure"

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

			By("verifying Degraded=True and Available=False")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeDegraded:    ConditionTrue,
				controller.ConditionTypeAvailable:   ConditionFalse,
				controller.ConditionTypeProgressing: ConditionTrue,
				controller.ConditionTypeStopped:     ConditionFalse,
			})

			By("verifying accessStartupProbeFailures is retained at threshold")
			probeFailures, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessStartupProbeFailures}")
			Expect(err).NotTo(HaveOccurred())
			Expect(probeFailures).To(Equal("3"))
		})
	})
})
