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
		teamNamespace = "scope-team-a"
		groupDir      = "template"
		subgroup      = "scope"
	)

	BeforeAll(func() {
		createNamespaceForTest("namespace-team-a", groupDir, subgroup)
	})

	AfterAll(func() {
		By("cleaning up workspaces")
		cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", teamNamespace,
			"--ignore-not-found", "--wait=true", "--timeout=120s")
		_, _ = utils.Run(cmd)

		By("cleaning up templates in team namespace")
		cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", teamNamespace,
			"--ignore-not-found", "--wait=true", "--timeout=60s")
		_, _ = utils.Run(cmd)

		By("cleaning up templates in shared namespace")
		cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", SharedNamespace,
			"--ignore-not-found", "--wait=true", "--timeout=60s")
		_, _ = utils.Run(cmd)

		By("deleting team namespace")
		cmd = exec.Command("kubectl", "delete", "ns", teamNamespace,
			"--ignore-not-found", "--wait=true", "--timeout=60s")
		_, _ = utils.Run(cmd)
	})

	It("should allow workspace to use a template from its own namespace", func() {
		createTemplateForTest("local-template", groupDir, subgroup)
		createWorkspaceForTest("ws-local-template", groupDir, subgroup)

		WaitForWorkspaceToReachCondition("ws-local-template", teamNamespace,
			controller.ConditionTypeAvailable, ConditionTrue)
	})

	It("should allow workspace to use a template from the shared namespace", func() {
		createTemplateForTest("base-template", "template", "base")
		createWorkspaceForTest("ws-shared-template", groupDir, subgroup)

		WaitForWorkspaceToReachCondition("ws-shared-template", teamNamespace,
			controller.ConditionTypeAvailable, ConditionTrue)
	})

	It("should reject workspace referencing a template from another team's namespace", func() {
		path := BuildTestResourcePath("ws-cross-ns-rejected", groupDir, subgroup)
		cmd := exec.Command("kubectl", "apply", "-f", path)
		_, err := utils.Run(cmd)
		Expect(err).To(HaveOccurred(), "Expected webhook to reject cross-namespace template reference")

		cmd = exec.Command("kubectl", "get", "workspace", "ws-cross-ns-rejected",
			"-n", teamNamespace, "--ignore-not-found")
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(BeEmpty())
	})

	It("should auto-inject default template from the workspace's own namespace", func() {
		createTemplateForTest("local-default-template", groupDir, subgroup)
		createWorkspaceForTest("ws-no-templateref", groupDir, subgroup)

		Eventually(func(g Gomega) {
			output, err := kubectlGet("workspace", "ws-no-templateref", teamNamespace,
				"{.spec.templateRef.name}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("local-default-template"))
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		WaitForWorkspaceToReachCondition("ws-no-templateref", teamNamespace,
			controller.ConditionTypeAvailable, ConditionTrue)
	})
})
