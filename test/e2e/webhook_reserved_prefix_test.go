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

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Reserved Prefix Validation", Ordered, func() {
	const (
		namespace = "default"
		groupDir  = "reserved-prefix"
		testUser  = "reserved-prefix-user"
	)

	BeforeAll(func() {
		By("creating RBAC role for reserved prefix tests")
		cmd := exec.Command("kubectl", "create", "-f",
			BuildTestResourcePath("role", groupDir, ""))
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("creating RoleBinding for test user")
		cmd = exec.Command("kubectl", "create", "-f",
			BuildTestResourcePath("role-binding", groupDir, ""))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		deleteResourcesForReservedPrefixTest(namespace)
	})

	Context("Create rejection", func() {
		It("should reject workspace with unknown reserved prefix label", func() {
			By("attempting to create workspace with reserved prefix label as non-admin user")
			path := BuildTestResourcePath("workspace-with-reserved-label", groupDir, "")
			err := createObjectAsUser(path, testUser, []string{})
			Expect(err).To(HaveOccurred(), "workspace with reserved prefix label should be rejected")

			By("verifying workspace was not created")
			VerifyResourceDoesNotExist("workspace", "reserved-label-workspace", namespace)
		})

		It("should reject workspace with unknown reserved prefix annotation", func() {
			By("attempting to create workspace with reserved prefix annotation as non-admin user")
			path := BuildTestResourcePath("workspace-with-reserved-annotation", groupDir, "")
			err := createObjectAsUser(path, testUser, []string{})
			Expect(err).To(HaveOccurred(), "workspace with reserved prefix annotation should be rejected")

			By("verifying workspace was not created")
			VerifyResourceDoesNotExist("workspace", "reserved-annotation-workspace", namespace)
		})
	})

	Context("Update validation", func() {
		BeforeAll(func() {
			By("creating a valid workspace as test user")
			validWorkspacePath := BuildTestResourcePath("valid-workspace", groupDir, "")
			err := createObjectAsUser(validWorkspacePath, testUser, []string{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should reject adding reserved prefix label on update", func() {
			By("patching workspace to add reserved prefix label")
			cmd := exec.Command("kubectl", "patch", "workspace", "valid-workspace",
				"-n", namespace,
				"--type=merge",
				"-p", `{"metadata":{"labels":{"workspace.jupyter.org/custom":"bad"}}}`,
				"--as="+testUser,
			)
			_, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "patch with reserved prefix label should be rejected")
		})

		It("should allow adding non-reserved label on update", func() {
			By("patching workspace to add a normal label")
			cmd := exec.Command("kubectl", "patch", "workspace", "valid-workspace",
				"-n", namespace,
				"--type=merge",
				"-p", `{"metadata":{"labels":{"environment":"staging"}}}`,
				"--as="+testUser,
			)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "patch with non-reserved label should succeed")

			By("verifying the label was applied")
			value, err := kubectlGet("workspace", "valid-workspace", namespace,
				"{.metadata.labels.environment}")
			Expect(err).NotTo(HaveOccurred())
			Expect(value).To(Equal("staging"))
		})
	})
})

func deleteResourcesForReservedPrefixTest(namespace string) {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", namespace,
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up RBAC")
	cmd = exec.Command("kubectl", "delete", "rolebinding", "-l", "jk8s/e2e=reserved-prefix-test",
		"-n", namespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)
	cmd = exec.Command("kubectl", "delete", "role", "-l", "jk8s/e2e=reserved-prefix-test",
		"-n", namespace, "--ignore-not-found")
	_, _ = utils.Run(cmd)

	time.Sleep(1 * time.Second)
}
