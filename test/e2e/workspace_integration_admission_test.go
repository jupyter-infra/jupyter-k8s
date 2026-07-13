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

// This suite exercises the ADMISSION behavior added by the integration webhooks (PR #421): the
// WorkspaceIntegrationTemplate validating webhook, the workspace-side integrationTemplateRef validator,
// and the mutating defaulter that stamps a bare-name ref's namespace. It deliberately uses templates whose
// resourceRef is never fetched (admission does not read the referenced resource), so it tests admission
// alone -- the controller resolve/freeze/replay behavior is covered by workspace_integration_test.go.
// Fixtures for this suite live under test/e2e/static/integration-admission/ with no subgroup. These are
// file-scoped so the apply/create helpers below can reference them directly (they are invariant across
// every spec, so passing them as arguments tripped unparam).
const (
	integrationAdmissionGroup    = "integration-admission"
	integrationAdmissionSubgroup = ""
	integrationWorkspaceNS       = "default"
)

var _ = Describe("Workspace Integration Admission", Ordered, func() {

	AfterEach(func() {
		deleteResourcesForIntegrationAdmissionTest()
	})

	AfterAll(func() {
		By("cleaning up the team-b namespace")
		cmd := exec.Command("kubectl", "delete", "ns", "namespace-team-b",
			"--ignore-not-found", "--wait=true", "--timeout=60s")
		_, _ = utils.Run(cmd)
	})

	Context("WorkspaceIntegrationTemplate validating webhook", func() {
		It("admits a well-formed template", func() {
			applyExpectSucceeds("valid-integration",
				"a well-formed WorkspaceIntegrationTemplate should be admitted")
		})

		It("rejects a template referencing an undeclared resourceRef", func() {
			applyExpectRejected("invalid-integration-undeclared-resource",
				"workspaceintegrationtemplate", "invalid-integration-undeclared-resource",
				"a template with an undeclared {{ resource }} handle should be rejected")
		})

		It("rejects a template referencing an undeclared parameter", func() {
			applyExpectRejected("invalid-integration-undeclared-param",
				"workspaceintegrationtemplate", "invalid-integration-undeclared-param",
				"a template referencing {{ .Parameters.<undeclared> }} should be rejected")
		})
	})

	Context("Workspace integrationTemplateRef validator", func() {
		It("admits a workspace referencing a template in its own namespace", func() {
			createIntegrationTemplateForTest("valid-integration")
			applyExpectSucceeds("ws-integration-own-ns",
				"a workspace referencing an existing own-namespace integration template should be admitted")
		})

		It("rejects a workspace whose ref targets another team's namespace", func() {
			createNamespaceForTest("namespace-team-b", integrationAdmissionGroup, integrationAdmissionSubgroup)
			createIntegrationTemplateForTest("valid-integration")
			applyExpectRejected("ws-integration-cross-ns-rejected",
				"workspace", "ws-integration-cross-ns-rejected",
				"a workspace referencing an integration template in another team's namespace should be rejected")
		})

		It("rejects a workspace referencing a non-existent template", func() {
			applyExpectRejected("ws-integration-missing-template-rejected",
				"workspace", "ws-integration-missing-template-rejected",
				"a workspace referencing a missing integration template should be rejected")
		})

		It("rejects a workspace that omits a declared parameter", func() {
			createIntegrationTemplateForTest("valid-integration")
			applyExpectRejected("ws-integration-missing-param-rejected",
				"workspace", "ws-integration-missing-param-rejected",
				"a workspace omitting a declared parameter should be rejected")
		})
	})

	Context("Mutating defaulter namespace stamping", func() {
		It("stamps the shared namespace onto a bare-name ref that resolves only there", func() {
			By("installing the template ONLY in the shared namespace")
			createIntegrationTemplateForTest("valid-integration-shared")

			By("creating a workspace whose ref omits the namespace")
			applyExpectSucceeds("ws-integration-bare-name",
				"a bare-name ref resolvable in the shared namespace should be admitted")

			By("verifying the mutating defaulter stamped the shared namespace into the stored spec")
			Eventually(func(g Gomega) {
				stamped, err := kubectlGet("workspace", "ws-integration-bare-name", integrationWorkspaceNS,
					"{.spec.integrationTemplateRefs[0].namespace}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(stamped).To(Equal(SharedNamespace))
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})
	})
})

// applyExpectSucceeds applies a static fixture from the integration-admission group and asserts the
// apply is accepted.
func applyExpectSucceeds(filename, because string) {
	GinkgoHelper()
	path := BuildTestResourcePath(filename, integrationAdmissionGroup, integrationAdmissionSubgroup)
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), because)
}

// applyExpectRejected applies a static fixture from the integration-admission group, asserts the apply is
// rejected by the webhook, and confirms the object was not persisted. All rejected fixtures in this suite
// live in the integrationWorkspaceNS ("default") namespace, so the not-persisted check reads from there.
func applyExpectRejected(filename, kind, name, because string) {
	GinkgoHelper()
	path := BuildTestResourcePath(filename, integrationAdmissionGroup, integrationAdmissionSubgroup)
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	Expect(err).To(HaveOccurred(), because)

	// The rejected object must not have been created.
	cmd = exec.Command("kubectl", verbGet, kind, name, "-n", integrationWorkspaceNS, "--ignore-not-found")
	output, getErr := utils.Run(cmd)
	Expect(getErr).NotTo(HaveOccurred())
	Expect(output).To(BeEmpty(), "the rejected %s should not have been persisted", kind)
}

// createIntegrationTemplateForTest applies a WorkspaceIntegrationTemplate fixture from the
// integration-admission group (its own metadata.name and namespace are baked into the file).
func createIntegrationTemplateForTest(filename string) {
	GinkgoHelper()
	path := BuildTestResourcePath(filename, integrationAdmissionGroup, integrationAdmissionSubgroup)
	By("creating integration template from " + path)
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

func deleteResourcesForIntegrationAdmissionTest() {
	By("cleaning up workspaces in default")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up integration templates in default")
	cmd = exec.Command("kubectl", "delete", "workspaceintegrationtemplate", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)

	By("cleaning up integration templates in the shared namespace")
	cmd = exec.Command("kubectl", "delete", "workspaceintegrationtemplate", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)

	time.Sleep(1 * time.Second)
}
