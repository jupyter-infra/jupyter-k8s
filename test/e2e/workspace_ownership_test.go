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

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

const (
	ownershipGroupDir    = "owner"
	ownershipSubgroupDir = ""
	ownershipNamespace   = "default"
	user1                = "user-1"
	user2                = "user-2"
	adminUser            = "admin-user"
)

var _ = Describe("Workspace Ownership", Ordered, func() {
	Context("OwnershipType enforcement", func() {
		BeforeAll(func() {
			By("creating RBAC role for workspace management")
			cmd := exec.Command("kubectl", "create", "-f",
				BuildTestResourcePath("workspace-manager-role", ownershipGroupDir, ownershipSubgroupDir))
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating RoleBinding for user-1")
			cmd = exec.Command("kubectl", "create", "-f",
				BuildTestResourcePath("workspace-manager-user1-binding", ownershipGroupDir, ownershipSubgroupDir))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating RoleBinding for user-2")
			cmd = exec.Command("kubectl", "create", "-f",
				BuildTestResourcePath("workspace-manager-user2-binding", ownershipGroupDir, ownershipSubgroupDir))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating RoleBinding for admin-user (system:masters)")
			cmd = exec.Command("kubectl", "create", "-f",
				BuildTestResourcePath("workspace-manager-admin-binding", ownershipGroupDir, ownershipSubgroupDir))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			deleteResourcesForOwnershipTest()
		})

		Context("Public workspace", func() {
			BeforeAll(func() {
				By("creating a public workspace as user-1")
				publicWorkspacePath := BuildTestResourcePath("public-workspace", ownershipGroupDir, ownershipSubgroupDir)
				err := createObjectAsUser(publicWorkspacePath, user1, []string{})
				Expect(err).NotTo(HaveOccurred())

				By("waiting for public workspace to be available")
				WaitForWorkspaceToReachCondition("public-workspace", ownershipNamespace, ConditionTypeAvailable, ConditionTrue)
			})

			It("should allow user-2 to update a public workspace created by user-1", func() {
				By("updating public workspace as user-2")
				publicWorkspacePath := BuildTestResourcePath("public-workspace-updated", ownershipGroupDir, ownershipSubgroupDir)
				err := updateObjectAsUser(publicWorkspacePath, user2, []string{})
				Expect(err).NotTo(HaveOccurred(), "user-2 should be able to update public workspace")

				By("verifying the update was applied")
				Eventually(func(g Gomega) {
					output, err := kubectlGet("workspace", "public-workspace", ownershipNamespace, "{.spec.displayName}")
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(Equal("Public Workspace Updated"))
				}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
			})

			It("should allow user-2 to delete a public workspace created by user-1", func() {
				By("deleting public workspace as user-2")
				err := deleteWorkspaceAsUser("public-workspace", user2, []string{})
				Expect(err).NotTo(HaveOccurred(), "user-2 should be able to delete public workspace")

				By("verifying workspace was deleted")
				WaitForResourceToNotExist("workspace", "public-workspace", ownershipNamespace, 60*time.Second, 2*time.Second)
			})

			It("should allow user-1 to update and delete a public workspace that user-1 created", func() {
				By("creating a new public workspace as user-1")
				publicWorkspacePath := BuildTestResourcePath("public-workspace", ownershipGroupDir, ownershipSubgroupDir)
				err := createObjectAsUser(publicWorkspacePath, user1, []string{})
				Expect(err).NotTo(HaveOccurred())

				By("waiting for public workspace to be available")
				WaitForWorkspaceToReachCondition("public-workspace", ownershipNamespace, ConditionTypeAvailable, ConditionTrue)

				By("updating public workspace as user-1")
				publicWorkspaceUpdatedPath := BuildTestResourcePath("public-workspace-updated", ownershipGroupDir,
					ownershipSubgroupDir)
				err = updateObjectAsUser(publicWorkspaceUpdatedPath, user1, []string{})
				Expect(err).NotTo(HaveOccurred(), "user-1 should be able to update their own public workspace")

				By("verifying the update was applied")
				Eventually(func(g Gomega) {
					output, err := kubectlGet("workspace", "public-workspace", ownershipNamespace, "{.spec.displayName}")
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(Equal("Public Workspace Updated"))
				}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

				By("deleting public workspace as user-1")
				err = deleteWorkspaceAsUser("public-workspace", user1, []string{})
				Expect(err).NotTo(HaveOccurred(), "user-1 should be able to delete their own public workspace")

				By("verifying workspace was deleted")
				WaitForResourceToNotExist("workspace", "public-workspace", ownershipNamespace, 60*time.Second, 2*time.Second)
			})
		})

		Context("OwnerOnly workspace", func() {
			BeforeAll(func() {
				By("creating an OwnerOnly workspace as user-1")
				ownerOnlyWorkspacePath := BuildTestResourcePath("owner-only-workspace", ownershipGroupDir, ownershipSubgroupDir)
				err := createObjectAsUser(ownerOnlyWorkspacePath, user1, []string{})
				Expect(err).NotTo(HaveOccurred())

				By("waiting for OwnerOnly workspace to be available")
				WaitForWorkspaceToReachCondition("owner-only-workspace", ownershipNamespace, ConditionTypeAvailable, ConditionTrue)
			})

			It("should deny user-2 from updating an OwnerOnly workspace created by user-1", func() {
				By("attempting to update OwnerOnly workspace as user-2")
				ownerOnlyWorkspaceUpdatedPath := BuildTestResourcePath("owner-only-workspace-updated", ownershipGroupDir,
					ownershipSubgroupDir)
				err := updateObjectAsUser(ownerOnlyWorkspaceUpdatedPath, user2, []string{})
				Expect(err).To(HaveOccurred(), "user-2 should NOT be able to update OwnerOnly workspace")
				Expect(err.Error()).To(ContainSubstring("only workspace owner can modify OwnerOnly workspaces"),
					"Error should indicate webhook denial")
			})

			It("should deny user-2 from deleting an OwnerOnly workspace created by user-1", func() {
				By("attempting to delete OwnerOnly workspace as user-2")
				err := deleteWorkspaceAsUser("owner-only-workspace", user2, []string{})
				Expect(err).To(HaveOccurred(), "user-2 should NOT be able to delete OwnerOnly workspace")
				Expect(err.Error()).To(ContainSubstring("only workspace owner can modify OwnerOnly workspaces"),
					"Error should indicate webhook denial")

				By("verifying workspace still exists")
				exists := ResourceExists("workspace", "owner-only-workspace", ownershipNamespace, "{.metadata.name}")
				Expect(exists).To(BeTrue(), "OwnerOnly workspace should still exist after failed delete attempt")
			})

			It("should allow user-1 to update and delete an OwnerOnly workspace that user-1 created", func() {
				By("updating OwnerOnly workspace as user-1")
				ownerOnlyWorkspaceUpdatedPath := BuildTestResourcePath("owner-only-workspace-updated", ownershipGroupDir,
					ownershipSubgroupDir)
				err := updateObjectAsUser(ownerOnlyWorkspaceUpdatedPath, user1, []string{})
				Expect(err).NotTo(HaveOccurred(), "user-1 should be able to update their own OwnerOnly workspace")

				By("verifying the update was applied")
				Eventually(func(g Gomega) {
					output, err := kubectlGet("workspace", "owner-only-workspace", ownershipNamespace, "{.spec.displayName}")
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(Equal("OwnerOnly Workspace Updated"))
				}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

				By("deleting OwnerOnly workspace as user-1")
				err = deleteWorkspaceAsUser("owner-only-workspace", user1, []string{})
				Expect(err).NotTo(HaveOccurred(), "user-1 should be able to delete their own OwnerOnly workspace")

				By("verifying workspace was deleted")
				WaitForResourceToNotExist("workspace", "owner-only-workspace", ownershipNamespace, 60*time.Second, 2*time.Second)
			})

			It("should allow admin-user to update and delete an OwnerOnly workspace created by user-1", func() {
				By("creating a new OwnerOnly workspace as user-1")
				ownerOnlyWorkspacePath := BuildTestResourcePath("owner-only-workspace", ownershipGroupDir, ownershipSubgroupDir)
				err := createObjectAsUser(ownerOnlyWorkspacePath, user1, []string{})
				Expect(err).NotTo(HaveOccurred())

				By("waiting for OwnerOnly workspace to be available")
				WaitForWorkspaceToReachCondition("owner-only-workspace", ownershipNamespace, ConditionTypeAvailable, ConditionTrue)

				By("updating OwnerOnly workspace as admin-user")
				ownerOnlyWorkspaceUpdatedPath := BuildTestResourcePath("owner-only-workspace-updated", ownershipGroupDir,
					ownershipSubgroupDir)
				err = updateObjectAsUser(ownerOnlyWorkspaceUpdatedPath, adminUser, []string{"system:masters"})
				Expect(err).NotTo(HaveOccurred(), "admin-user should be able to update OwnerOnly workspace")

				By("verifying the update was applied")
				Eventually(func(g Gomega) {
					output, err := kubectlGet("workspace", "owner-only-workspace", ownershipNamespace, "{.spec.displayName}")
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(output).To(Equal("OwnerOnly Workspace Updated"))
				}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

				By("deleting OwnerOnly workspace as admin-user")
				err = deleteWorkspaceAsUser("owner-only-workspace", adminUser, []string{"system:masters"})
				Expect(err).NotTo(HaveOccurred(), "admin-user should be able to delete OwnerOnly workspace")

				By("verifying workspace was deleted")
				WaitForResourceToNotExist("workspace", "owner-only-workspace", ownershipNamespace, 60*time.Second, 2*time.Second)
			})
		})
	})
})

// deleteResourcesForOwnershipTest cleans up resources created during ownership tests
func deleteResourcesForOwnershipTest() {
	GinkgoHelper()

	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", ownershipNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up RBAC resources by label")
	cmd = exec.Command("kubectl", "delete", "rolebinding", "-l", "jk8s/e2e=ownership-test",
		"-n", ownershipNamespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	cmd = exec.Command("kubectl", "delete", "role", "-l", "jk8s/e2e=ownership-test",
		"-n", ownershipNamespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	By("waiting an arbitrary fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
