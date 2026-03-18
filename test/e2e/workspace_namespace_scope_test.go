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

var _ = Describe("Namespace Template Scope", Ordered, func() {
	const (
		scopedNs = "scoped-ns"
		groupDir = "template"
		subgroup = "namespace-scope"
	)

	BeforeAll(func() {
		By("creating scoped namespace with Namespaced label")
		createNamespaceForTest(scopedNs, map[string]string{
			"workspace.jupyter.org/template-namespace-scope": "Namespaced",
		})
	})

	AfterAll(func() {
		By("cleaning up workspaces in scoped namespace")
		cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", scopedNs,
			"--ignore-not-found", "--wait=true", "--timeout=120s")
		_, _ = utils.Run(cmd)

		By("cleaning up workspaces in default namespace")
		cmd = exec.Command("kubectl", "delete", "workspace", "--all", "-n", "default",
			"--ignore-not-found", "--wait=true", "--timeout=120s")
		_, _ = utils.Run(cmd)

		By("cleaning up templates in scoped namespace")
		cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", scopedNs,
			"--ignore-not-found", "--wait=true", "--timeout=60s")
		_, _ = utils.Run(cmd)

		By("cleaning up templates in shared namespace")
		cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", SharedNamespace,
			"--ignore-not-found", "--wait=true", "--timeout=60s")
		_, _ = utils.Run(cmd)

		By("deleting scoped namespace")
		cmd = exec.Command("kubectl", "delete", "ns", scopedNs, "--ignore-not-found", "--wait=true", "--timeout=60s")
		_, _ = utils.Run(cmd)
	})

	It("should allow workspace to use template from same namespace "+
		"when namespace has template-namespace-scope label set to Namespaced", func() {
		By("creating local template in scoped namespace")
		createResourceForTest("local-template", groupDir, subgroup)

		By("creating workspace referencing same-namespace template")
		createResourceForTest("ws-same-ns-explicit", groupDir, subgroup)

		By("verifying workspace becomes available")
		WaitForWorkspaceToReachCondition("ws-same-ns-explicit", scopedNs,
			controller.ConditionTypeAvailable, ConditionTrue)
	})

	It("should reject workspace referencing cross-namespace template when namespace has template-namespace-scope label set to Namespaced", func() {
		By("creating base template in shared namespace")
		createTemplateForTest("base-template", "template", "base")

		By("attempting to create workspace with cross-namespace templateRef")
		path := BuildTestResourcePath("ws-cross-ns-rejected", groupDir, subgroup)
		cmd := exec.Command("kubectl", "apply", "-f", path)
		_, err := utils.Run(cmd)
		Expect(err).To(HaveOccurred(), "Expected webhook to reject cross-namespace template reference")

		By("verifying workspace was not created")
		cmd = exec.Command("kubectl", "get", "workspace", "ws-cross-ns-rejected",
			"-n", scopedNs, "--ignore-not-found")
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(BeEmpty())
	})

	It("should auto-inject default template from same namespace when namespace has template-namespace-scope label set to Namespaced", func() {
		By("creating default-labeled template in scoped namespace")
		createResourceForTest("local-default-template", groupDir, subgroup)

		By("creating workspace without templateRef in scoped namespace")
		createResourceForTest("ws-auto-inject-scoped", groupDir, subgroup)

		By("verifying workspace gets the local default template injected")
		Eventually(func(g Gomega) {
			output, err := kubectlGet("workspace", "ws-auto-inject-scoped", scopedNs,
				"{.spec.templateRef.name}")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).To(Equal("local-default-template"))
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		By("verifying workspace becomes available")
		WaitForWorkspaceToReachCondition("ws-auto-inject-scoped", scopedNs,
			controller.ConditionTypeAvailable, ConditionTrue)
	})

	It("should allow cross-namespace template reference when namespace has no template-namespace-scope label", func() {
		By("creating base template in shared namespace")
		createTemplateForTest("base-template", "template", "base")

		By("creating workspace in default (unscoped) namespace with cross-ns templateRef")
		createResourceForTest("ws-cross-ns-unscoped", groupDir, subgroup)

		By("verifying workspace becomes available")
		WaitForWorkspaceToReachCondition("ws-cross-ns-unscoped", "default",
			controller.ConditionTypeAvailable, ConditionTrue)

		By("verifying templateRef points to shared namespace")
		output, err := kubectlGet("workspace", "ws-cross-ns-unscoped", "default",
			"{.spec.templateRef.namespace}")
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(Equal(SharedNamespace))
	})
})

// createResourceForTest applies a YAML resource from the static test directory
//
//nolint:unparam
func createResourceForTest(filename, group, subgroup string) {
	GinkgoHelper()
	path := BuildTestResourcePath(filename, group, subgroup)
	By(fmt.Sprintf("applying resource %s from %s", filename, path))
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}
