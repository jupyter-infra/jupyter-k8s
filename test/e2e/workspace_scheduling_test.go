//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

var _ = Describe("Workspace Scheduling", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "scheduling"
	)

	AfterEach(func() {
		deleteResourcesForSchedulingTest(workspaceNamespace)
	})

	Context("Node Affinity", func() {
		It("should create workspace with node affinity and apply it to deployment", func() {
			workspaceName := "workspace-with-affinity"
			workspaceFilename := "workspace-with-affinity"

			By("creating workspace with node affinity")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("waiting for workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying Available=True, Progressing=False, Degraded=False, Stopped=False")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeDegraded:    ConditionFalse,
				controller.ConditionTypeAvailable:   ConditionTrue,
				controller.ConditionTypeStopped:     ConditionFalse,
			})

			By("verifying workspace has correct affinity spec")
			jsonPath := "{.spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution" +
				".nodeSelectorTerms[0].matchExpressions[0].key}"
			affinityKey, err := kubectlGet("workspace", workspaceName, workspaceNamespace, jsonPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(affinityKey).To(Equal("kubernetes.io/arch"))
		})
	})

	Context("Tolerations", func() {
		It("should create workspace with tolerations and apply them to deployment", func() {
			workspaceName := "workspace-with-tolerations"
			workspaceFilename := "workspace-with-tolerations"

			By("creating workspace with tolerations")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("waiting for workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying Available=True, Progressing=False, Degraded=False, Stopped=False")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeDegraded:    ConditionFalse,
				controller.ConditionTypeAvailable:   ConditionTrue,
				controller.ConditionTypeStopped:     ConditionFalse,
			})

			By("verifying workspace has correct tolerations spec")
			tolerationKey, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.tolerations[0].key}")
			Expect(err).NotTo(HaveOccurred())
			Expect(tolerationKey).To(Equal("dedicated"))
		})
	})

	Context("Node Selector", func() {
		It("should create workspace with node selector and apply it to deployment", func() {
			workspaceName := "workspace-with-node-selector"
			workspaceFilename := "workspace-with-node-selector"

			By("creating workspace with node selector")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("waiting for workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying Available=True, Progressing=False, Degraded=False, Stopped=False")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeDegraded:    ConditionFalse,
				controller.ConditionTypeAvailable:   ConditionTrue,
				controller.ConditionTypeStopped:     ConditionFalse,
			})

			By("verifying workspace has correct node selector spec")
			nodeSelector, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.nodeSelector}")
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeSelector).To(ContainSubstring("kubernetes.io/arch"))
			Expect(nodeSelector).To(ContainSubstring("amd64"))
		})
	})
})

func deleteResourcesForSchedulingTest(workspaceNamespace string) {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", workspaceNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("waiting an arbitrary fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
