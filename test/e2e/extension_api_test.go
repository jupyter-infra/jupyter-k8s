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
	extensionAPIGroupDir      = "extension-api"
	extensionAPISubgroupDir   = ""
	extensionAPITestNamespace = "default"
)

var _ = Describe("Extension API", Ordered, func() {
	Context("Setup and registration", func() {

		It("should have extension API service registered and available", func() {
			By("verifying APIService v1alpha1.connection.workspace.jupyter.org is available")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "apiservice", "v1alpha1.connection.workspace.jupyter.org",
					"-o", "jsonpath={.status.conditions[?(@.type=='Available')].status}")
				status, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.TrimSpace(status)).To(Equal("True"),
					"APIService v1alpha1.connection.workspace.jupyter.org not available")
			}).WithTimeout(3 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		})

		It("should allow authorized user to create ConnectionAccessReview", func() {
			By("creating ConnectionAccessReview without impersonation (admin user)")
			reviewPath := getFixturePath("access-review-basic")
			cmd := exec.Command("kubectl", "create", "-f", reviewPath, "-o", "yaml")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Admin user should be able to create ConnectionAccessReview")

			By("verifying ConnectionAccessReview response contains status fields")
			// ConnectionAccessReview is a review resource - the response is returned immediately
			// with the status populated (similar to SubjectAccessReview)
			Expect(output).To(ContainSubstring("status:"), "Response should contain status section")
			_, _ = fmt.Fprintf(GinkgoWriter, "ConnectionAccessReview created successfully with status:\n%s\n", output)
		})

		It("should deny unauthorized user from creating ConnectionAccessReview", func() {
			By("attempting to create ConnectionAccessReview as unauthorized user via kubectl impersonation")
			reviewPath := getFixturePath("access-review-basic")
			err := createConnectionAccessReviewAsUser(reviewPath, "no-connection-access-review-user", []string{})
			Expect(err).To(HaveOccurred(), "Unauthorized user should NOT be able to create ConnectionAccessReview")
			Expect(err.Error()).To(ContainSubstring("forbidden"), "Error should indicate RBAC denial")
		})
	})

	Context("ConnectionAccessReview", func() {
		BeforeAll(func() {
			By("creating RBAC role for workspace creation")
			cmd := exec.Command("kubectl", "create", "-f",
				getFixturePath("workspace-creator-role"))
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating RoleBinding for owner user to create workspaces")
			cmd = exec.Command("kubectl", "create", "-f",
				getFixturePath("workspace-creator-binding"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating a public workspace for authorization tests")
			createWorkspaceForTest("workspace-public-access", extensionAPIGroupDir, extensionAPISubgroupDir)

			By("creating a private workspace as owner user")
			privateWorkspacePath := getFixturePath("workspace-owner-only-access")
			cmd = exec.Command("kubectl", "create", "-f", privateWorkspacePath,
				"--as=owner-for-access-test-user")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for workspaces to be available")
			WaitForWorkspaceToReachCondition("workspace-public-access", extensionAPITestNamespace, ConditionTypeAvailable, ConditionTrue)
			WaitForWorkspaceToReachCondition("workspace-owner-only-access", extensionAPITestNamespace, ConditionTypeAvailable, ConditionTrue)

			By("creating RBAC role for workspace connection permission")
			cmd = exec.Command("kubectl", "create", "-f",
				getFixturePath("workspace-connection-role"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating RoleBinding for owner user")
			cmd = exec.Command("kubectl", "create", "-f",
				getFixturePath("workspace-connection-owner-binding"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating RoleBinding for workspace-users group")
			cmd = exec.Command("kubectl", "create", "-f",
				getFixturePath("workspace-connection-group-binding"))
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			deleteResourcesForExtensionAPITest()
		})

		It("should allow access when user has RBAC permission for workspace connection", func() {
			By("creating ConnectionAccessReview for user with RBAC permission")
			reviewPath := getFixturePath("access-review-rbac-pass")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is allowed")
			Expect(allowed).To(BeTrue(), "User with RBAC permission should be allowed")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
			_, _ = fmt.Fprintf(GinkgoWriter, "RBAC pass test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
		})

		It("should allow access with EKS-style Extra fields for user with RBAC permission", func() {
			By("creating ConnectionAccessReview with EKS Extra fields for user with RBAC permission")
			reviewPath := getFixturePath("access-review-rbac-pass-with-extra")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is allowed with Extra fields present")
			Expect(allowed).To(BeTrue(), "User with RBAC permission and EKS Extra fields should be allowed")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
			_, _ = fmt.Fprintf(GinkgoWriter, "RBAC pass with EKS Extra test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
		})

		It("should deny access when user lacks RBAC permission for workspace connection", func() {
			By("creating ConnectionAccessReview for user without RBAC permission")
			reviewPath := getFixturePath("access-review-rbac-fail")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is denied")
			Expect(allowed).To(BeFalse(), "User without RBAC permission should be denied")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
			_, _ = fmt.Fprintf(GinkgoWriter, "RBAC fail test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
		})

		It("should allow access when user's group has RBAC permission for workspace connection", func() {
			By("creating ConnectionAccessReview for user in authorized group")
			reviewPath := getFixturePath("access-review-group")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is allowed via group membership")
			Expect(allowed).To(BeTrue(), "User in authorized group should be allowed")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
			_, _ = fmt.Fprintf(GinkgoWriter, "Group RBAC test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
		})

		It("should allow access to public workspace for any user with RBAC permission", func() {
			By("creating ConnectionAccessReview for public workspace")
			reviewPath := getFixturePath("access-review-public")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is allowed for public workspace")
			_, _ = fmt.Fprintf(GinkgoWriter, "Public workspace test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
			Expect(allowed).To(BeTrue(), "Any user with RBAC permission should have access to a public workspace")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
		})

		It("should allow access to the owner of a private workspace", func() {
			By("creating ConnectionAccessReview for the owner of the private workspace")
			reviewPath := getFixturePath("access-review-private-owner")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is allowed for the owner user")
			_, _ = fmt.Fprintf(GinkgoWriter, "Private workspace owner test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
			Expect(allowed).To(BeTrue(), "Owner user should have access private workspace")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
		})

		It("should deny access to private workspace for another user than the owner", func() {
			By("creating ConnectionAccessReview for non-owner of a private workspace")
			reviewPath := getFixturePath("access-review-private-non-owner")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access is denied for non-owner user")
			Expect(allowed).To(BeFalse(), "Non-owner user should be denied access to private workspace")
			Expect(notFound).To(BeFalse(), "Workspace should be found")
			_, _ = fmt.Fprintf(GinkgoWriter, "Private workspace non-owner test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
		})

		It("should return notFound for non-existent workspace", func() {
			By("creating ConnectionAccessReview for non-existent workspace")
			reviewPath := getFixturePath("access-review-not-found")
			allowed, notFound, reason, err := createConnectionAccessReviewAndGetStatus(reviewPath)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace is not found")
			Expect(notFound).To(BeTrue(), "Non-existent workspace should return notFound")
			Expect(allowed).To(BeFalse(), "Access should be denied for non-existent workspace")
			_, _ = fmt.Fprintf(GinkgoWriter, "Workspace not found test - allowed: %v, notFound: %v, reason: %s\n", allowed, notFound, reason)
		})
	})
})

// deleteResourcesForExtensionAPITest cleans up resources created during extension API tests
func deleteResourcesForExtensionAPITest() {
	GinkgoHelper()

	// ConnectionAccessReview resources are not persisted in etcd, so no cleanup needed

	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", extensionAPITestNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up RBAC resources by label")
	cmd = exec.Command("kubectl", "delete", "rolebinding", "-l", "jk8s/e2e=extension-api-test",
		"-n", extensionAPITestNamespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	cmd = exec.Command("kubectl", "delete", "role", "-l", "jk8s/e2e=extension-api-test",
		"-n", extensionAPITestNamespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	By("waiting an arbitrary fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
