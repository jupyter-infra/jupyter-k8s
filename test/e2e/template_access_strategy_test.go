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

// These tests cover the template->AccessStrategy protection finalizer (issue #386). A template that
// references an AccessStrategy via spec.defaultAccessStrategy must add its own protection finalizer to
// that AccessStrategy, so the AccessStrategy cannot be fully removed while still referenced. This mirrors
// the long-standing workspace->AccessStrategy finalizer, but uses a distinct finalizer name so the two
// references are tracked independently.
var _ = Describe("Template Access Strategy Protection", Ordered, func() {
	const (
		groupDir = "access-strategy"

		// Dedicated access strategy for this suite (kept minimal so the tests don't depend on
		// unrelated fixture complexity).
		accessStrategyName     = "template-protection-access-strategy"
		accessStrategyFilename = "template-protection-access-strategy"

		// Second access strategy used as the swap target when a template repoints its reference.
		otherAccessStrategyName     = "template-protection-access-strategy-other"
		otherAccessStrategyFilename = "template-protection-access-strategy-other"

		templateName     = "template-with-access-strategy"
		templateFilename = "template-with-access-strategy"

		template2Name     = "template-with-access-strategy-2"
		template2Filename = "template-with-access-strategy-2"

		// Templates whose referenced access strategy cannot be resolved (the webhook must reject them).
		missingASTemplateName      = "template-with-missing-access-strategy"
		missingASTemplateFilename  = "template-with-missing-access-strategy"
		wrongNamespaceTemplateName = "template-with-wrong-namespace-access-strategy"
		wrongNamespaceTemplateFile = "template-with-wrong-namespace-access-strategy"

		// Workspace that references the access strategy directly (for the mixed workspace+template case).
		workspaceNamespace = "default"
		workspaceName      = "workspace-template-protection"
		workspaceFilename  = "workspace-template-protection"
	)

	AfterEach(func() {
		// AccessStrategies referenced by lingering workspaces/templates can't be cleaned up until those
		// references are gone, so delete dependents first, then the access strategies.
		By("cleaning up workspaces")
		cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", workspaceNamespace,
			"--ignore-not-found", "--wait=true", "--timeout=120s")
		_, _ = utils.Run(cmd)

		By("cleaning up templates")
		cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", SharedNamespace,
			"--ignore-not-found", "--wait=true", "--timeout=60s")
		_, _ = utils.Run(cmd)

		By("cleaning up access strategies")
		cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy", "--all", "-n", SharedNamespace,
			"--ignore-not-found", "--wait=true", "--timeout=60s")
		_, _ = utils.Run(cmd)

		time.Sleep(1 * time.Second)
	})

	Context("Label stamping", func() {
		It("should stamp access strategy labels on a template referencing an access strategy", func() {
			By("creating the access strategy")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating a template referencing the access strategy")
			createTemplateForTest(templateFilename, groupDir, "")

			verifyTemplateAccessStrategyLabels(templateName, accessStrategyName, SharedNamespace)
		})
	})

	// The template mutating webhook enforces that the referenced access strategy exists (mustExist=true),
	// matching the workspace webhook. These cases prove create/update is rejected when the reference cannot
	// be resolved - both when the access strategy is missing entirely and when it exists only in another
	// namespace.
	Context("Webhook rejection when the access strategy cannot be resolved", func() {
		It("should reject creating a template whose access strategy does not exist", func() {
			By("creating a template referencing a non-existent access strategy")
			verifyCreateTemplateRejectedByWebhook(missingASTemplateFilename, groupDir, missingASTemplateName)
		})

		It("should reject creating a template whose access strategy exists only in another namespace", func() {
			By("creating the access strategy in the shared namespace")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating a template that references the same name but in the 'default' namespace")
			verifyCreateTemplateRejectedByWebhook(wrongNamespaceTemplateFile, groupDir, wrongNamespaceTemplateName)
		})

		It("should reject mutating a template to reference a non-existent access strategy", func() {
			By("creating the access strategy")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating a valid template referencing it")
			createTemplateForTest(templateFilename, groupDir, "")
			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)

			By("patching the template to point at a non-existent access strategy (must be rejected)")
			patch := `{"spec":{"defaultAccessStrategy":{"name":"template-protection-access-strategy-missing",` +
				`"namespace":"` + SharedNamespace + `"}}}`
			cmd := exec.Command("kubectl", "patch", "workspacetemplate", templateName,
				"-n", SharedNamespace, "--type=merge", "-p", patch)
			output, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "template webhook should reject repointing to a non-existent access strategy")
			Expect(output).To(ContainSubstring("not found"),
				"rejection should explain that the referenced access strategy was not found")

			By("verifying the template still references the original access strategy")
			verifyTemplateAccessStrategyLabels(templateName, accessStrategyName, SharedNamespace)
		})
	})

	Context("Finalizer protection", func() {
		It("should add the template-protection finalizer to a referenced access strategy", func() {
			By("creating the access strategy")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating a template referencing the access strategy")
			createTemplateForTest(templateFilename, groupDir, "")
			verifyTemplateAccessStrategyLabels(templateName, accessStrategyName, SharedNamespace)

			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)
		})

		It("should hold an in-use access strategy in Terminating until the template is gone", func() {
			By("creating the access strategy")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating a template referencing the access strategy")
			createTemplateForTest(templateFilename, groupDir, "")
			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)

			verifyAccessStrategyHeldByTemplate(accessStrategyName)
		})
	})

	Context("Release after dereference", func() {
		It("should allow deletion after the referencing template is deleted", func() {
			By("creating the access strategy")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating a template referencing the access strategy")
			createTemplateForTest(templateFilename, groupDir, "")
			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)

			By("deleting the access strategy (delete accepted, deletionTimestamp set)")
			deleteAccessStrategyNoWait(accessStrategyName)

			By("verifying the finalizer holds it in Terminating while the template references it")
			verifyAccessStrategyStillHeld(accessStrategyName)

			By("deleting the referencing template")
			deleteTemplate(templateName)

			By("verifying the access strategy is now fully deleted")
			WaitForResourceToNotExist("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
				30*time.Second, 2*time.Second)
		})

		It("should drop the finalizer after the template removes its defaultAccessStrategy", func() {
			By("creating the access strategy")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating a template referencing the access strategy")
			createTemplateForTest(templateFilename, groupDir, "")
			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)

			By("patching the template to remove its defaultAccessStrategy")
			cmd := exec.Command("kubectl", "patch", "workspacetemplate", templateName,
				"-n", SharedNamespace, "--type=merge", "-p", `{"spec":{"defaultAccessStrategy":null}}`)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the template's access strategy labels are cleared")
			verifyTemplateAccessStrategyLabelsCleared(templateName)

			By("verifying the template-protection finalizer is removed from the access strategy")
			verifyTemplateAccessStrategyFinalizerAbsent(accessStrategyName)

			By("deleting the access strategy now that nothing references it")
			cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy", accessStrategyName,
				"-n", SharedNamespace, "--wait=true", "--timeout=30s")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the access strategy is deleted")
			WaitForResourceToNotExist("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
				15*time.Second, 1*time.Second)
		})
	})

	// These cases exercise the independence of multiple template references to a single access strategy:
	// the access strategy must stay held until the LAST referring template dereferences it, whether that
	// happens via template deletion or by repointing the reference elsewhere.
	Context("Multiple referencing templates", func() {
		It("should hold the access strategy until the last of two referencing templates is deleted", func() {
			By("creating the access strategy")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating two templates referencing the access strategy")
			createTemplateForTest(templateFilename, groupDir, "")
			createTemplateForTest(template2Filename, groupDir, "")
			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)

			By("deleting the access strategy (finalizer holds it in Terminating)")
			deleteAccessStrategyNoWait(accessStrategyName)
			verifyAccessStrategyStillHeld(accessStrategyName)

			By("deleting the first template - the second still references the access strategy")
			deleteTemplate(templateName)
			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)
			verifyAccessStrategyStillHeld(accessStrategyName)

			By("deleting the second template - nothing references the access strategy now")
			deleteTemplate(template2Name)

			By("verifying the access strategy is finally released")
			WaitForResourceToNotExist("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
				30*time.Second, 2*time.Second)
		})

		It("should keep the finalizer when one template derefs and a second repoints elsewhere", func() {
			By("creating two access strategies")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")
			createAccessStrategyForTest(otherAccessStrategyFilename, groupDir, "")

			By("creating two templates referencing the first access strategy")
			createTemplateForTest(templateFilename, groupDir, "")
			createTemplateForTest(template2Filename, groupDir, "")
			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)

			By("removing the reference from the first template")
			removeTemplateAccessStrategy(templateName)
			verifyTemplateAccessStrategyLabelsCleared(templateName)

			By("verifying the finalizer remains - the second template still references it")
			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)

			By("repointing the second template to the other access strategy")
			patchTemplateAccessStrategy(template2Name, otherAccessStrategyName)
			verifyTemplateAccessStrategyLabels(template2Name, otherAccessStrategyName, SharedNamespace)

			By("verifying the first access strategy's template finalizer is now removed")
			verifyTemplateAccessStrategyFinalizerAbsent(accessStrategyName)

			By("verifying the other access strategy gained the template finalizer")
			verifyTemplateAccessStrategyFinalizerPresent(otherAccessStrategyName)

			By("deleting the now-unreferenced first access strategy")
			cmd := exec.Command("kubectl", "delete", "workspaceaccessstrategy", accessStrategyName,
				"-n", SharedNamespace, "--wait=true", "--timeout=30s")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			WaitForResourceToNotExist("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
				15*time.Second, 1*time.Second)
		})

		It("should release the access strategy only after both referencing templates are deleted", func() {
			By("creating the access strategy")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating two templates referencing the access strategy")
			createTemplateForTest(templateFilename, groupDir, "")
			createTemplateForTest(template2Filename, groupDir, "")
			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)

			By("deleting the first template - the finalizer must remain (second still references it)")
			deleteTemplate(templateName)
			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)

			By("deleting the second template - the finalizer should now be removed")
			deleteTemplate(template2Name)
			verifyTemplateAccessStrategyFinalizerAbsent(accessStrategyName)

			By("verifying the access strategy is deletable now that nothing references it")
			cmd := exec.Command("kubectl", "delete", "workspaceaccessstrategy", accessStrategyName,
				"-n", SharedNamespace, "--wait=true", "--timeout=30s")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			WaitForResourceToNotExist("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
				15*time.Second, 1*time.Second)
		})
	})

	// This case mixes both referrer kinds on a single access strategy to prove the two protection
	// finalizers are tracked independently: the access strategy stays held until BOTH the workspace
	// (workspace-protection finalizer) and the template (template-protection finalizer) dereference it.
	Context("Mixed workspace and template references", func() {
		It("should hold until both the referencing workspace and template are gone", func() {
			By("creating the access strategy (no protection finalizers yet)")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")
			verifyWorkspaceAccessStrategyFinalizerAbsent(accessStrategyName)
			verifyTemplateAccessStrategyFinalizerAbsent(accessStrategyName)

			By("creating a workspace referencing the access strategy (adds workspace-protection finalizer)")
			createWorkspaceForTest(workspaceFilename, groupDir, "")
			verifyWorkspaceAccessStrategyFinalizerPresent(accessStrategyName)

			By("creating a template referencing the access strategy (adds template-protection finalizer)")
			createTemplateForTest(templateFilename, groupDir, "")
			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)

			By("deleting the access strategy (both finalizers hold it in Terminating)")
			deleteAccessStrategyNoWait(accessStrategyName)
			verifyAccessStrategyStillHeld(accessStrategyName)

			By("deleting the workspace - its finalizer goes, but the template finalizer still holds it")
			deleteWorkspace(workspaceName, workspaceNamespace)
			verifyWorkspaceAccessStrategyFinalizerAbsent(accessStrategyName)
			verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName)
			verifyAccessStrategyStillHeld(accessStrategyName)

			By("deleting the template - the last finalizer goes and the access strategy is released")
			deleteTemplate(templateName)

			By("verifying the access strategy is finally released")
			WaitForResourceToNotExist("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
				30*time.Second, 2*time.Second)
		})
	})
})
