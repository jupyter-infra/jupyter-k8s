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

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// verifyAccessStrategyDeletionProtection verifies that an AccessStrategy is protected from deletion
// while a workspace references it, and gets deleted after the workspace is removed
func verifyAccessStrategyDeletionProtection(
	accessStrategyName string,
	workspaceName string,
	workspaceNamespace string,
) {
	ginkgo.GinkgoHelper()

	ginkgo.By("attempting to delete AccessStrategy with --wait=false")
	cmd := exec.Command("kubectl", "delete", "workspaceaccessstrategy", accessStrategyName,
		"-n", SharedNamespace, "--wait=false")
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("verifying DeletionTimestamp is set on AccessStrategy")
	gomega.Eventually(func() bool {
		deletionTimestamp, err := kubectlGet("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
			"{.metadata.deletionTimestamp}")
		if err != nil {
			return false
		}
		return deletionTimestamp != ""
	}, 10*time.Second, 1*time.Second).Should(gomega.BeTrue(), "DeletionTimestamp should be set")

	ginkgo.By("consistently verifying AccessStrategy still exists for 30s")
	gomega.Consistently(func() bool {
		exists := ResourceExists("workspaceaccessstrategy", accessStrategyName,
			SharedNamespace, "{.metadata.name}")
		return exists
	}, 30*time.Second, 5*time.Second).Should(gomega.BeTrue(),
		"AccessStrategy should not be deleted while workspace references it")

	ginkgo.By("verifying AccessStrategy is NOT deleted")
	exists := ResourceExists("workspaceaccessstrategy", accessStrategyName, SharedNamespace, "{.metadata.name}")
	gomega.Expect(exists).To(gomega.BeTrue(), "AccessStrategy should still exist")

	ginkgo.By(fmt.Sprintf("deleting the workspace %s", workspaceName))
	cmd = exec.Command("kubectl", "delete", "workspace", workspaceName,
		"-n", workspaceNamespace, "--wait=true", "--timeout=120s")
	_, err = utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("waiting for workspace deletion to complete")
	WaitForResourceToNotExist("workspace", workspaceName, workspaceNamespace, 2*time.Minute, 5*time.Second)

	ginkgo.By("verifying AccessStrategy is now deleted")
	WaitForResourceToNotExist("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
		10*time.Second, 1*time.Second)
}

const (
	templateAccessStrategyFinalizer  = "workspace.jupyter.org/accessstrategy-template-protection"
	workspaceAccessStrategyFinalizer = "workspace.jupyter.org/accessstrategy-protection"
)

// verifyTemplateAccessStrategyFinalizerPresent asserts the template-protection finalizer was added to
// the AccessStrategy (the safety net that keeps it from being garbage-collected while a template
// references it). Distinct from the workspace-protection finalizer so the two are tracked independently.
func verifyTemplateAccessStrategyFinalizerPresent(accessStrategyName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("verifying template-protection finalizer is present on %s", accessStrategyName))
	gomega.Eventually(func() (string, error) {
		return kubectlGet("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
			`{.metadata.finalizers[?(@=="`+templateAccessStrategyFinalizer+`")]}`)
	}, 30*time.Second, 2*time.Second).Should(gomega.Equal(templateAccessStrategyFinalizer),
		"a template reference must add the template-protection finalizer")
}

// verifyTemplateAccessStrategyFinalizerAbsent asserts the template-protection finalizer is no longer
// present on the AccessStrategy (e.g. after the template dereferences it).
//
//nolint:unparam // helper kept general; current callers happen to share the fixture name
func verifyTemplateAccessStrategyFinalizerAbsent(accessStrategyName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("verifying template-protection finalizer is removed from %s", accessStrategyName))
	gomega.Eventually(func() (string, error) {
		return kubectlGet("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
			`{.metadata.finalizers[?(@=="`+templateAccessStrategyFinalizer+`")]}`)
	}, 30*time.Second, 2*time.Second).Should(gomega.BeEmpty(),
		"the template-protection finalizer should be removed once no template references the access strategy")
}

// verifyWorkspaceAccessStrategyFinalizerPresent asserts the workspace-protection finalizer was added to
// the AccessStrategy. Distinct from the template-protection finalizer so the two are tracked independently.
func verifyWorkspaceAccessStrategyFinalizerPresent(accessStrategyName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("verifying workspace-protection finalizer is present on %s", accessStrategyName))
	gomega.Eventually(func() (string, error) {
		return kubectlGet("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
			`{.metadata.finalizers[?(@=="`+workspaceAccessStrategyFinalizer+`")]}`)
	}, 30*time.Second, 2*time.Second).Should(gomega.Equal(workspaceAccessStrategyFinalizer),
		"a workspace reference must add the workspace-protection finalizer")
}

// verifyWorkspaceAccessStrategyFinalizerAbsent asserts the workspace-protection finalizer is no longer
// present on the AccessStrategy (e.g. after the referencing workspace is deleted).
func verifyWorkspaceAccessStrategyFinalizerAbsent(accessStrategyName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("verifying workspace-protection finalizer is removed from %s", accessStrategyName))
	gomega.Eventually(func() (string, error) {
		return kubectlGet("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
			`{.metadata.finalizers[?(@=="`+workspaceAccessStrategyFinalizer+`")]}`)
	}, 30*time.Second, 2*time.Second).Should(gomega.BeEmpty(),
		"the workspace-protection finalizer should be removed once no workspace references the access strategy")
}

// deleteWorkspace deletes a Workspace and waits for it to be fully removed.
func deleteWorkspace(workspaceName, namespace string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("deleting workspace %s", workspaceName))
	cmd := exec.Command("kubectl", "delete", "workspace", workspaceName,
		"-n", namespace, "--wait=true", "--timeout=120s")
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// deleteAccessStrategyNoWait issues a delete on the AccessStrategy without waiting and asserts the
// delete is accepted, then that a deletionTimestamp is set. Use when a finalizer is expected to hold
// the object in Terminating afterwards.
func deleteAccessStrategyNoWait(accessStrategyName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("deleting AccessStrategy %s with --wait=false", accessStrategyName))
	cmd := exec.Command("kubectl", "delete", "workspaceaccessstrategy", accessStrategyName,
		"-n", SharedNamespace, "--wait=false")
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("verifying deletionTimestamp is set (delete accepted, finalizer pending)")
	gomega.Eventually(func() string {
		ts, _ := kubectlGet("workspaceaccessstrategy", accessStrategyName, SharedNamespace,
			"{.metadata.deletionTimestamp}")
		return ts
	}, 10*time.Second, 1*time.Second).ShouldNot(gomega.BeEmpty(), "deletionTimestamp should be set")
}

// verifyAccessStrategyStillHeld asserts the AccessStrategy is consistently NOT garbage-collected for a
// window, i.e. a finalizer is still holding it in Terminating. Used to prove that removing one of
// several template references does not prematurely release the access strategy.
func verifyAccessStrategyStillHeld(accessStrategyName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("consistently verifying AccessStrategy %s is still held", accessStrategyName))
	gomega.Consistently(func() bool {
		return ResourceExists("workspaceaccessstrategy", accessStrategyName, SharedNamespace, "{.metadata.name}")
	}, 15*time.Second, 5*time.Second).Should(gomega.BeTrue(),
		"finalizer should keep the access strategy until ALL referencing templates are gone")
}

// verifyAccessStrategyHeldByTemplate deletes the AccessStrategy and verifies the template finalizer holds
// it in Terminating (deletionTimestamp set, object not removed) for as long as a template references it.
//
//nolint:unparam // helper kept general; current callers happen to share the fixture name
func verifyAccessStrategyHeldByTemplate(accessStrategyName string) {
	ginkgo.GinkgoHelper()

	deleteAccessStrategyNoWait(accessStrategyName)
	verifyAccessStrategyStillHeld(accessStrategyName)
}

// verifyCreateTemplateRejectedByWebhook attempts to create a template from the given fixture and asserts
// the webhook rejects it with an error containing expectedSubstring, then that the template was not persisted.
func verifyCreateTemplateRejectedByWebhook(filename, group, templateName, expectedSubstring string) {
	ginkgo.GinkgoHelper()

	path := BuildTestResourcePath(filename, group, "")
	ginkgo.By(fmt.Sprintf("attempting to create template %s from %s", templateName, path))
	cmd := exec.Command("kubectl", "apply", "-f", path)
	output, err := utils.Run(cmd)
	gomega.Expect(err).To(gomega.HaveOccurred(),
		fmt.Sprintf("template webhook should reject %s", templateName))
	gomega.Expect(output).To(gomega.ContainSubstring(expectedSubstring),
		fmt.Sprintf("rejection should contain %q", expectedSubstring))

	ginkgo.By(fmt.Sprintf("verifying template %s was not created", templateName))
	cmd = exec.Command("kubectl", "get", "workspacetemplate", templateName, "-n", SharedNamespace, "--ignore-not-found")
	getOut, getErr := utils.Run(cmd)
	gomega.Expect(getErr).NotTo(gomega.HaveOccurred())
	gomega.Expect(getOut).To(gomega.BeEmpty(), "template should not exist after webhook rejection")
}

// deleteTemplate deletes a WorkspaceTemplate and waits for it to be fully removed.
func deleteTemplate(templateName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("deleting template %s", templateName))
	cmd := exec.Command("kubectl", "delete", "workspacetemplate", templateName,
		"-n", SharedNamespace, "--wait=true", "--timeout=60s")
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// patchTemplateAccessStrategy repoints a template's spec.defaultAccessStrategy to the given access
// strategy in the shared namespace (merge patch).
func patchTemplateAccessStrategy(templateName, accessStrategyName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("repointing template %s to access strategy %s", templateName, accessStrategyName))
	patch := fmt.Sprintf(`{"spec":{"defaultAccessStrategy":{"name":%q,"namespace":%q}}}`,
		accessStrategyName, SharedNamespace)
	cmd := exec.Command("kubectl", "patch", "workspacetemplate", templateName,
		"-n", SharedNamespace, "--type=merge", "-p", patch)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// removeTemplateAccessStrategy clears a template's spec.defaultAccessStrategy (merge patch).
func removeTemplateAccessStrategy(templateName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("removing defaultAccessStrategy from template %s", templateName))
	cmd := exec.Command("kubectl", "patch", "workspacetemplate", templateName,
		"-n", SharedNamespace, "--type=merge", "-p", `{"spec":{"defaultAccessStrategy":null}}`)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// verifyTemplateAccessStrategyLabels asserts the template carries the access strategy lookup labels
// resolving to the given name/namespace. Used to confirm the mutating webhook / controller stamped them.
//
//nolint:unparam // helper kept general; current callers happen to share fixture names
func verifyTemplateAccessStrategyLabels(templateName, expectedASName, expectedASNamespace string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("verifying template %s has access strategy labels", templateName))
	gomega.Eventually(func(g gomega.Gomega) {
		name, err := kubectlGet("workspacetemplate", templateName, SharedNamespace,
			"{.metadata.labels.workspace\\.jupyter\\.org/access-strategy-name}")
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(name).To(gomega.Equal(expectedASName))

		ns, err := kubectlGet("workspacetemplate", templateName, SharedNamespace,
			"{.metadata.labels.workspace\\.jupyter\\.org/access-strategy-namespace}")
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(ns).To(gomega.Equal(expectedASNamespace))
	}, 30*time.Second, 2*time.Second).Should(gomega.Succeed())
}

// verifyTemplateAccessStrategyLabelsCleared asserts the template no longer carries the access
// strategy lookup labels (e.g. after dereferencing via a template update).
func verifyTemplateAccessStrategyLabelsCleared(templateName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("verifying template %s access strategy labels are cleared", templateName))
	gomega.Eventually(func() string {
		name, _ := kubectlGet("workspacetemplate", templateName, SharedNamespace,
			"{.metadata.labels.workspace\\.jupyter\\.org/access-strategy-name}")
		return name
	}, 30*time.Second, 2*time.Second).Should(gomega.BeEmpty(),
		"access strategy labels should be cleared after dereference")
}

// patchAccessStrategy applies a JSON patch file to a WorkspaceAccessStrategy resource
//
//nolint:unparam
func patchAccessStrategy(
	group string,
	subgroup string,
	patchFilename string,
	accessStrategyName string,
) {
	ginkgo.GinkgoHelper()

	patchPath := fmt.Sprintf("test/e2e/static/%s-%s/%s.json", group, subgroup, patchFilename)

	// Execute patch command
	cmd := exec.Command("kubectl", "patch", "workspaceaccessstrategy", accessStrategyName,
		"-n", SharedNamespace, "--type=merge", "--patch-file", patchPath)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}
