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

var _ = Describe("Namespace Template Scope", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "template"
		subgroup           = "scope"
	)

	BeforeAll(func() {
		createNamespaceForTest("namespace-team-b", groupDir, subgroup)
		createTemplateForTest("team-b-template", groupDir, subgroup)
	})

	AfterEach(func() {
		deleteResourcesForScopeTest()
	})

	AfterAll(func() {
		By("cleaning up team-b namespace")
		cmd := exec.Command("kubectl", "delete", "ns", "scope-team-b",
			"--ignore-not-found", "--wait=true", "--timeout=60s")
		_, _ = utils.Run(cmd)
	})

	It("should allow workspace to use a template from its own namespace", func() {
		createTemplateForTest("local-template", groupDir, subgroup)
		createWorkspaceForTest("ws-local-template", groupDir, subgroup)

		WaitForWorkspaceToReachCondition("ws-local-template", workspaceNamespace,
			controller.ConditionTypeAvailable, ConditionTrue)
	})

	It("should allow workspace to use a template from the shared namespace", func() {
		createTemplateForTest("base-template", "template", "base")
		createWorkspaceForTest("ws-shared-template", groupDir, subgroup)

		WaitForWorkspaceToReachCondition("ws-shared-template", workspaceNamespace,
			controller.ConditionTypeAvailable, ConditionTrue)
	})

	It("should reject workspace referencing a template from another team's namespace", func() {
		path := BuildTestResourcePath("ws-cross-ns-rejected", groupDir, subgroup)
		cmd := exec.Command("kubectl", "apply", "-f", path)
		_, err := utils.Run(cmd)
		Expect(err).To(HaveOccurred(), "Expected webhook to reject cross-namespace template reference")

		cmd = exec.Command("kubectl", "get", "workspace", "ws-cross-ns-rejected",
			"-n", workspaceNamespace, "--ignore-not-found")
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(BeEmpty())
	})

	It("should auto-inject local default template over shared default template", func() {
		By("creating default-labeled template in both local and shared namespaces")
		createTemplateForTest("local-default-template", groupDir, subgroup)
		createTemplateForTest("shared-default-template", groupDir, subgroup)

		By("creating workspace without templateRef")
		createWorkspaceForTest("ws-no-templateref", groupDir, subgroup)

		By("verifying local default template was injected")
		Eventually(func(g Gomega) {
			output, err := kubectlGet("workspace", "ws-no-templateref", workspaceNamespace,
				"{.spec.templateRef.name}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("local-default-template"))
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		WaitForWorkspaceToReachCondition("ws-no-templateref", workspaceNamespace,
			controller.ConditionTypeAvailable, ConditionTrue)
	})

	It("should fall back to shared default template when no local default exists", func() {
		By("creating default-labeled template only in shared namespace")
		createTemplateForTest("shared-default-template", groupDir, subgroup)

		By("creating workspace without templateRef")
		createWorkspaceForTest("ws-no-templateref", groupDir, subgroup)

		By("verifying shared default template was injected")
		Eventually(func(g Gomega) {
			output, err := kubectlGet("workspace", "ws-no-templateref", workspaceNamespace,
				"{.spec.templateRef.name}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("shared-default-template"))
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		WaitForWorkspaceToReachCondition("ws-no-templateref", workspaceNamespace,
			controller.ConditionTypeAvailable, ConditionTrue)
	})
})

func deleteResourcesForScopeTest() {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up templates in default namespace")
	cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)

	By("cleaning up templates in shared namespace")
	cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)

	time.Sleep(1 * time.Second)
}
