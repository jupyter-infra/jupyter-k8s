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

// Templates referencing an access strategy via spec.defaultAccessStrategy are subject to the same
// namespace-scope rule as workspaces: the reference may only target the template's own namespace or
// the shared namespace. Enforcing it at template admission prevents admins from creating templates
// that would make any referencing workspace fail its own admission webhook. This mirrors the
// workspace-side coverage in workspace_namespace_scope_test.go.
//
// Note: the cross-namespace rejection here is distinct from the "access strategy does not exist"
// rejection covered in template_access_strategy_test.go. To prove the scope rule (not the existence
// check, which the mutating webhook applies first) is what rejects, BeforeAll creates the team-b
// access strategy so the reference resolves and only the scope rule can fail it.
var _ = Describe("Template Namespace Scope", Ordered, func() {
	const (
		groupDir           = "template"
		subgroup           = "scope"
		workspaceNamespace = "default"
	)

	BeforeAll(func() {
		createNamespaceForTest("namespace-team-b", "template", "scope")
		createAccessStrategyForTest("team-b-access-strategy", "access-strategy", "scope")
	})

	AfterAll(func() {
		By("cleaning up team-b namespace")
		cmd := exec.Command("kubectl", "delete", "ns", "scope-team-b",
			"--ignore-not-found", "--wait=true", "--timeout=60s")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		deleteResourcesForScopeTest()
		deleteAccessStrategyResourcesForScopeTest()
	})

	It("should allow a template referencing an access strategy from its own namespace", func() {
		createAccessStrategyForTest("local-access-strategy", "access-strategy", "scope")
		createTemplateForTest("template-local-as", groupDir, subgroup)

		Eventually(func(g Gomega) {
			output, err := kubectlGet("workspacetemplate", "template-local-as", workspaceNamespace,
				"{.metadata.name}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("template-local-as"))
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
	})

	It("should allow a template referencing an access strategy from the shared namespace", func() {
		createAccessStrategyForTest("shared-access-strategy", "access-strategy", "scope")
		createTemplateForTest("template-shared-as", groupDir, subgroup)

		Eventually(func(g Gomega) {
			output, err := kubectlGet("workspacetemplate", "template-shared-as", workspaceNamespace,
				"{.metadata.name}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("template-shared-as"))
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
	})

	It("should reject a template referencing an access strategy from another team's namespace", func() {
		path := BuildTestResourcePath("template-cross-ns-as-rejected", groupDir, subgroup)
		cmd := exec.Command("kubectl", "apply", "-f", path)
		_, err := utils.Run(cmd)
		Expect(err).To(HaveOccurred(), "Expected webhook to reject cross-namespace access strategy reference")

		cmd = exec.Command("kubectl", "get", "workspacetemplate", "template-cross-ns-as-rejected",
			"-n", workspaceNamespace, "--ignore-not-found")
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(BeEmpty())
	})
})
