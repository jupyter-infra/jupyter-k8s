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

var _ = Describe("Workspace Template Mutability", Ordered, func() {
	const (
		workspaceNamespace   = "default"
		groupDir             = "template"
		subgroupBase         = "base"
		subgroupMutability   = "mutability"
		baseTemplateName     = "base-template"
		baseTemplateFilename = "base-template"
	)

	AfterEach(func() {
		deleteResourcesForTemplateTest()
	})

	Context("Mutability and Deletion Protection", func() {
		It("should prevent template deletion when workspace is using it", func() {
			workspaceName := "deletion-protection-workspace"
			workspaceFilename := "deletion-protection-workspace"

			By("creating the template")
			createTemplateForTest(baseTemplateName, groupDir, subgroupBase)

			By("creating a workspace using the template")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupMutability)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying workspace has templateRef set")
			output, err := kubectlGet("workspace", workspaceName,
				workspaceNamespace, "{.spec.templateRef.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(baseTemplateName))

			By("attempting to delete the template")
			cmd := exec.Command("kubectl", "delete", "workspacetemplate",
				baseTemplateName, "-n", SharedNamespace, "--wait=false")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying template still exists with deletionTimestamp set")
			Eventually(func(g Gomega) {
				output, err := kubectlGet("workspacetemplate",
					baseTemplateName, SharedNamespace,
					"{.metadata.deletionTimestamp}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).NotTo(BeEmpty(), "expected deletionTimestamp to be set")
			}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

			By("verifying finalizer is blocking deletion")
			output, err = kubectlGet("workspacetemplate",
				baseTemplateName, SharedNamespace,
				"{.metadata.finalizers[0]}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("workspace.jupyter.org/template-protection"))

			By("deleting the workspace")
			cmd = exec.Command("kubectl", "delete", "workspace", workspaceName, "--wait=true")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying template can now be deleted")
			verifyTemplateDeleted := func(g Gomega) {
				_, err := kubectlGet("workspacetemplate",
					baseTemplateName, SharedNamespace, "")
				g.Expect(err).To(HaveOccurred(), "expected template to be deleted")
			}
			Eventually(verifyTemplateDeleted).
				WithPolling(1 * time.Second).
				WithTimeout(30 * time.Second).
				Should(Succeed())
		})

		It("should allow workspace templateRef changes (mutability)", func() {
			templateName := "newref-template"
			templateFilename := "newref-template"

			workspaceName := "newref-workspace"
			workspaceFilename := "newref-workspace"

			By("creating the base template")
			createTemplateForTest(baseTemplateFilename, groupDir, subgroupBase)

			By("creating another template for switching")
			createTemplateForTest(templateFilename, groupDir, subgroupMutability)

			By("creating workspace using base template")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupMutability)

			By("changing templateRef to newref template")
			patchCmd := `{"spec":{"templateRef":{"name":"newref-template", "namespace": "jupyter-k8s-shared"}}}`
			cmd := exec.Command("kubectl", "patch", "workspace", workspaceName,
				"-n", workspaceNamespace, "--type=merge", "-p", patchCmd)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "templateRef should be mutable")

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying Available=True, Progressing=False, Degraded=False, Stopped=False")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeDegraded:    ConditionFalse,
				controller.ConditionTypeAvailable:   ConditionTrue,
				controller.ConditionTypeStopped:     ConditionFalse,
				ConditionTypeDeleting:               ConditionFalse,
			})

			By("verifying templateRef was changed")
			output, getErr := kubectlGet("workspace", workspaceName,
				workspaceNamespace, "{.spec.templateRef.name}")
			Expect(getErr).NotTo(HaveOccurred())
			Expect(output).To(Equal(templateName))
		})

		It("should allow WorkspaceTemplate spec modification (mutability)", func() {
			templateName := "mutable-template"
			templateFilename := "mutable-template"

			By("creating a template for mutability testing")
			createTemplateForTest(templateFilename, groupDir, subgroupMutability)

			By("modifying template spec (change description)")
			patchCmd := `{"spec":{"description":"Modified description"}}`
			cmd := exec.Command("kubectl", "patch", "workspacetemplate",
				templateName, "-n", SharedNamespace, "--type=merge",
				"-p", patchCmd)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "template spec should be mutable")

			By("verifying template spec description was changed")
			output, getErr := kubectlGet("workspacetemplate",
				templateName, SharedNamespace, "{.spec.description}")
			Expect(getErr).NotTo(HaveOccurred())
			Expect(output).To(Equal("Modified description"))
		})
	})
})
