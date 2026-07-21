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

var _ = Describe("Template Idle Shutdown Enforcement", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "template-enforce-idle-shutdown"

		enforceTemplate       = "enforce-idle-template"
		permissiveTemplate    = "permissive-idle-template"
		allowOverrideTemplate = "allow-override-idle-template"
	)

	AfterEach(func() {
		deleteResourcesForTemplateEnforceIdleTest()
	})

	Context("when the template does not enforce idle shutdown", func() {
		It("should accept a workspace disabling idle shutdown when no override policy is set", func() {
			workspace := "permissive-disable-idle-workspace"

			By("creating a template with defaultIdleShutdown but no idleShutdownOverrides policy")
			createTemplateForTest(permissiveTemplate, groupDir, "")

			By("creating a workspace that disables idle shutdown")
			createWorkspaceForTest(workspace, groupDir, "")

			By("verifying the workspace was accepted with idle shutdown disabled")
			output, err := kubectlGet("workspace", workspace, workspaceNamespace, "{.spec.idleShutdown.enabled}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("false"))
		})

		It("should accept a disabled override when allow is true (bounds skipped when disabled)", func() {
			workspace := "allow-override-disabled-workspace"

			By("creating a template with allow: true and timeout bounds")
			createTemplateForTest(allowOverrideTemplate, groupDir, "")

			By("creating a workspace that disables idle shutdown with an out-of-bounds timeout")
			createWorkspaceForTest(workspace, groupDir, "")

			By("verifying the workspace was accepted unchanged")
			enabled, err := kubectlGet("workspace", workspace, workspaceNamespace, "{.spec.idleShutdown.enabled}")
			Expect(err).NotTo(HaveOccurred())
			Expect(enabled).To(Equal("false"))

			timeout, err := kubectlGet("workspace", workspace, workspaceNamespace,
				"{.spec.idleShutdown.idleTimeoutInMinutes}")
			Expect(err).NotTo(HaveOccurred())
			Expect(timeout).To(Equal("999"))
		})
	})

	Context("when the template allows overrides but declares timeout bounds", func() {
		It("should reject an enabled timeout outside the declared bounds even when allow is true", func() {
			workspace := "allow-override-enabled-out-of-bounds-workspace"

			By("creating a template with allow: true and bounds [15, 60]")
			createTemplateForTest(allowOverrideTemplate, groupDir, "")

			By("attempting to create an enabled workspace with a timeout of 999")
			VerifyCreateWorkspaceRejectedByWebhook(workspace, groupDir, "", workspace, workspaceNamespace)
		})
	})

	Context("when the template enforces idle shutdown (allow: false)", func() {
		It("should accept a workspace that changes only the timeout within bounds", func() {
			workspace := "valid-timeout-within-bounds-workspace"

			By("creating a template with allow: false and bounds [15, 60]")
			createTemplateForTest(enforceTemplate, groupDir, "")

			By("creating a workspace matching the default with an in-bounds timeout of 45")
			createWorkspaceForTest(workspace, groupDir, "")

			By("verifying the workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspace,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying the timeout override was accepted")
			timeout, err := kubectlGet("workspace", workspace, workspaceNamespace,
				"{.spec.idleShutdown.idleTimeoutInMinutes}")
			Expect(err).NotTo(HaveOccurred())
			Expect(timeout).To(Equal("45"))
		})

		It("should reject a workspace that disables idle shutdown", func() {
			workspace := "reject-disable-idle-workspace"

			By("creating a template with allow: false")
			createTemplateForTest(enforceTemplate, groupDir, "")

			By("attempting to create a workspace with idle shutdown disabled")
			VerifyCreateWorkspaceRejectedByWebhook(workspace, groupDir, "", workspace, workspaceNamespace)
		})

		It("should reject a workspace that modifies a non-timeout idle shutdown setting", func() {
			workspace := "reject-modified-detection-workspace"

			By("creating a template with allow: false")
			createTemplateForTest(enforceTemplate, groupDir, "")

			By("attempting to create a workspace that changes detection path/transport")
			VerifyCreateWorkspaceRejectedByWebhook(workspace, groupDir, "", workspace, workspaceNamespace)
		})

		It("should reject a workspace whose timeout is outside the bounds", func() {
			workspace := "reject-timeout-out-of-bounds-workspace"

			By("creating a template with allow: false and bounds [15, 60]")
			createTemplateForTest(enforceTemplate, groupDir, "")

			By("attempting to create a workspace with a timeout of 120")
			VerifyCreateWorkspaceRejectedByWebhook(workspace, groupDir, "", workspace, workspaceNamespace)
		})

		It("should reject an update that pushes the timeout out of bounds", func() {
			workspace := "valid-timeout-within-bounds-workspace"

			By("creating a template with allow: false and bounds [15, 60]")
			createTemplateForTest(enforceTemplate, groupDir, "")

			By("creating a valid workspace with an in-bounds timeout")
			createWorkspaceForTest(workspace, groupDir, "")

			By("verifying the workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspace,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("patching the workspace to a timeout above the maximum")
			patchCmd := `{"spec":{"idleShutdown":{"idleTimeoutInMinutes":120}}}`
			cmd := exec.Command("kubectl", "patch", "workspace", workspace,
				"-n", workspaceNamespace, "--type=merge", "-p", patchCmd)
			_, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "expected webhook to reject out-of-bounds timeout update")
		})
	})

	Context("template-level policy consistency", func() {
		It("should reject a template that locks idle shutdown without a defaultIdleShutdown", func() {
			templateFilename := "invalid-locking-no-default-template"
			templateName := "invalid-locking-no-default-template"

			By("attempting to create a template with allow: false and no defaultIdleShutdown")
			path := BuildTestResourcePath(templateFilename, groupDir, "")
			cmd := exec.Command("kubectl", "apply", "-f", path)
			output, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "template webhook should reject a locking policy with no default")
			Expect(output).To(ContainSubstring("defaultIdleShutdown is not set"),
				"rejection should explain the missing defaultIdleShutdown")

			By("verifying the template was not created")
			cmd = exec.Command("kubectl", verbGet, "workspacetemplate", templateName,
				"-n", SharedNamespace, "--ignore-not-found")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty(), "template should not exist after webhook rejection")
		})
	})
})

func deleteResourcesForTemplateEnforceIdleTest() {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up templates")
	cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)

	By("waiting an arbitrary fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
