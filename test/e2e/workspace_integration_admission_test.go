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
// WorkspaceIntegrationTemplate validating webhook and the workspace-side integrationTemplateRef validator
// (namespace scope, template existence in the ref's namespace, parameter completeness). It deliberately
// uses templates whose resourceRef is never fetched (admission does not read the referenced resource), so
// it tests admission alone -- the controller resolve/freeze/replay behavior is covered by
// workspace_integration_test.go.
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

	Context("Shared-namespace ref", func() {
		It("admits a ref that explicitly targets the shared namespace where the template lives", func() {
			By("installing the template ONLY in the shared namespace")
			createIntegrationTemplateForTest("valid-integration-shared")

			By("creating a workspace whose ref names the shared namespace explicitly")
			applyExpectSucceeds("ws-integration-shared-explicit",
				"a ref explicitly targeting the shared namespace should be admitted")

			By("verifying the stored ref namespace is left as the user wrote it (the shared namespace)")
			// The validator reads the template from the ref's namespace as written -- it does not rewrite the
			// stored spec, so the ref namespace stays exactly as submitted.
			Consistently(func(g Gomega) {
				stored, err := kubectlGet("workspace", "ws-integration-shared-explicit", integrationWorkspaceNS,
					"{.spec.integrationTemplateRefs[0].namespace}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(stored).To(Equal(SharedNamespace))
			}).WithTimeout(10 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})
	})

	// Referenced-resource authorization (the SubjectAccessReview confused-deputy guard): a non-exempt user
	// who references a resource they cannot get is rejected; one who can get it is admitted; an exempt
	// (admin) caller bypasses the check entirely. These run as impersonated users (kubectl --as), so unlike
	// the specs above they exercise the check against the REAL cluster authorizer and the operator's real
	// SubjectAccessReview-create grant -- which a unit test (fake authorizer) structurally cannot cover.
	Context("Referenced-resource authorization (SubjectAccessReview)", func() {
		BeforeAll(func() {
			By("granting workspace-create to the denied and allowed users, and service-get to the allowed user only")
			for _, fixture := range []string{
				"sar-workspace-creator-role",
				"sar-service-reader-role",
				"sar-denied-user-binding",
				"sar-allowed-user-workspace-binding",
				"sar-allowed-user-service-binding",
			} {
				cmd := exec.Command("kubectl", "apply", "-f",
					BuildTestResourcePath(fixture, integrationAdmissionGroup, integrationAdmissionSubgroup))
				_, err := utils.Run(cmd)
				Expect(err).NotTo(HaveOccurred(), "failed to apply RBAC fixture %s", fixture)
			}

			By("installing the integration template the workspaces reference")
			createIntegrationTemplateForTest("sar-integration")
		})

		AfterAll(func() {
			By("cleaning up the SAR RBAC roles and bindings")
			cmd := exec.Command("kubectl", "delete", "role,rolebinding",
				"-l", "jk8s/e2e=integration-sar-test", "-n", integrationWorkspaceNS,
				"--ignore-not-found", "--wait=true", "--timeout=60s")
			_, _ = utils.Run(cmd)
		})

		It("rejects a workspace whose submitter cannot get the referenced resource", func() {
			createIntegrationTemplateForTest("sar-integration")
			By("creating the workspace as sar-denied-user (workspace-create but no service-get)")
			path := BuildTestResourcePath("ws-sar-denied", integrationAdmissionGroup, integrationAdmissionSubgroup)
			err := createObjectAsUser(path, "sar-denied-user", nil)
			Expect(err).To(HaveOccurred(),
				"a user who cannot get the referenced Service must be rejected by the resource authorization")
			// Confirm the rejection is the integration authorization verdict, not an unrelated RBAC denial on
			// workspaces (the user is granted workspace-create precisely so this message is attributable).
			Expect(err.Error()).To(ContainSubstring("may not get"),
				"rejection must come from the referenced-resource SubjectAccessReview, not a workspace RBAC denial")

			By("verifying the rejected workspace was not persisted")
			// --ignore-not-found so an absent workspace yields empty output (not a NotFound error), matching
			// the not-persisted check in applyExpectRejected.
			cmd := exec.Command("kubectl", verbGet, "workspace", "ws-sar-denied",
				"-n", integrationWorkspaceNS, "--ignore-not-found")
			output, getErr := utils.Run(cmd)
			Expect(getErr).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty(), "the rejected workspace should not have been persisted")
		})

		It("admits a workspace whose submitter can get the referenced resource", func() {
			createIntegrationTemplateForTest("sar-integration")
			By("creating the workspace as sar-allowed-user (workspace-create AND service-get)")
			path := BuildTestResourcePath("ws-sar-allowed", integrationAdmissionGroup, integrationAdmissionSubgroup)
			err := createObjectAsUser(path, "sar-allowed-user", nil)
			Expect(err).NotTo(HaveOccurred(),
				"a user who can get the referenced Service must pass the resource authorization")

			By("verifying the admitted workspace was persisted")
			Eventually(func(g Gomega) {
				name, getErr := kubectlGet("workspace", "ws-sar-allowed", integrationWorkspaceNS, "{.metadata.name}")
				g.Expect(getErr).NotTo(HaveOccurred())
				g.Expect(name).To(Equal("ws-sar-allowed"))
			}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
		})

		It("admits an exempt (admin) caller without checking referenced-resource access", func() {
			createIntegrationTemplateForTest("sar-integration")
			By("creating the workspace as an admin (system:masters) with no explicit service-get binding")
			// system:masters is the webhook's DefaultAdminGroup, so the integration resource authorization is
			// skipped entirely -- admitted despite no service-get RoleBinding for this identity.
			path := BuildTestResourcePath("ws-sar-exempt-admin", integrationAdmissionGroup, integrationAdmissionSubgroup)
			err := createObjectAsUser(path, "sar-admin-user", []string{systemMastersGroup})
			Expect(err).NotTo(HaveOccurred(),
				"an exempt admin caller must bypass the referenced-resource authorization")

			By("verifying the admitted workspace was persisted")
			Eventually(func(g Gomega) {
				name, getErr := kubectlGet("workspace", "ws-sar-exempt-admin", integrationWorkspaceNS, "{.metadata.name}")
				g.Expect(getErr).NotTo(HaveOccurred())
				g.Expect(name).To(Equal("ws-sar-exempt-admin"))
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
