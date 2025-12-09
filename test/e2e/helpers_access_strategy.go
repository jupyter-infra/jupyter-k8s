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

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// verifyAccessStrategyDeletionProtection verifies that an AccessStrategy is protected from deletion
// while a workspace references it, and gets deleted after the workspace is removed
func verifyAccessStrategyDeletionProtection(
	accessStrategyName string,
	workspaceName string,
	workspaceNamespace string,
) {
	ginkgo.GinkgoHelper()

	ginkgo.By("attempting to delete AccessStrategy with --wait=false")
	cmd := exec.Command("kubectl", "delete", "workspaceaccessstrategy", accessStrategyName,
		"-n", SharedNamespace, "--wait=false")
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("verifying DeletionTimestamp is set on AccessStrategy")
	gomega.Eventually(func() bool {
		deletionTimestamp, err := kubectlGet("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
			"{.metadata.deletionTimestamp}")
		if err != nil {
			return false
		}
		return deletionTimestamp != ""
	}, 10*time.Second, 1*time.Second).Should(gomega.BeTrue(), "DeletionTimestamp should be set")

	ginkgo.By("consistently verifying AccessStrategy still exists for 30s")
	gomega.Consistently(func() bool {
		exists := ResourceExists("workspaceaccessstrategy", accessStrategyName,
			SharedNamespace, "{.metadata.name}")
		return exists
	}, 30*time.Second, 5*time.Second).Should(gomega.BeTrue(),
		"AccessStrategy should not be deleted while workspace references it")

	ginkgo.By("verifying AccessStrategy is NOT deleted")
	exists := ResourceExists("workspaceaccessstrategy", accessStrategyName, SharedNamespace, "{.metadata.name}")
	gomega.Expect(exists).To(gomega.BeTrue(), "AccessStrategy should still exist")

	ginkgo.By(fmt.Sprintf("deleting the workspace %s", workspaceName))
	cmd = exec.Command("kubectl", "delete", "workspace", workspaceName,
		"-n", workspaceNamespace, "--wait=true", "--timeout=120s")
	_, err = utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("waiting for workspace deletion to complete")
	WaitForResourceToNotExist("workspace", workspaceName, workspaceNamespace, 2*time.Minute, 5*time.Second)

	ginkgo.By("verifying AccessStrategy is now deleted")
	WaitForResourceToNotExist("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
		10*time.Second, 1*time.Second)
}

// patchAccessStrategy applies a JSON patch file to a WorkspaceAccessStrategy resource
//
//nolint:unparam
func patchAccessStrategy(
	group string,
	subgroup string,
	patchFilename string,
	accessStrategyName string,
) {
	ginkgo.GinkgoHelper()

	patchPath := fmt.Sprintf("test/e2e/static/%s-%s/%s.json", group, subgroup, patchFilename)

	// Execute patch command
	cmd := exec.Command("kubectl", "patch", "workspaceaccessstrategy", accessStrategyName,
		"-n", SharedNamespace, "--type=merge", "--patch-file", patchPath)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}
