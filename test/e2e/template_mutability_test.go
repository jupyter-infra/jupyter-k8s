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

var _ = Describe("Workspace Template Mutability", Ordered, func() {
	const (
		workspaceNamespace   = "default"
		groupDir             = "template"
		subgroupBase         = "base"
		subgroupMutability   = "mutability"
		subgroupBounds       = "bounds"
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

	Context("Pinned access and ownership types", func() {
		const (
			pinnedPrivateTemplate = "pinned-private-template"
			pinnedSharedTemplate  = "pinned-shared-template"
			pinnedSwitchWorkspace = "pinned-switch-workspace"
		)

		// createSwitchWorkspace creates the templates and a workspace on the private template
		createSwitchWorkspace := func() {
			createTemplateForTest(pinnedPrivateTemplate, groupDir, subgroupBounds)
			createTemplateForTest(pinnedSharedTemplate, groupDir, subgroupMutability)
			createWorkspaceForTest(pinnedSwitchWorkspace, groupDir, subgroupMutability)
			WaitForWorkspaceToReachCondition(pinnedSwitchWorkspace, workspaceNamespace,
				controller.ConditionTypeAvailable, ConditionTrue)
		}

		expectSharingTypes := func(expected string) {
			testTemplateFeaturesInheritance(pinnedSwitchWorkspace, workspaceNamespace, []valueTestCaseForTemplateTest{
				{description: "verifying the access type", jsonPath: jsonPathAccessType, expected: expected},
				{description: "verifying the ownership type", jsonPath: jsonPathOwnershipType, expected: expected},
			})
		}

		It("should require a templateRef switch to set the new template's pinned values", func() {
			createSwitchWorkspace()
			expectSharingTypes(sharingTypeOwnerOnly)

			By("switching only templateRef to the shared template")
			output, err := patchWorkspaceForTest(pinnedSwitchWorkspace, workspaceNamespace,
				fmt.Sprintf(`{"spec":{"templateRef":{"name":%q}}}`, pinnedSharedTemplate))
			Expect(err).To(HaveOccurred(), "the switch must set the values the shared template pins")
			Expect(output).To(ContainSubstring(`spec.accessType: pinned to Public by template "pinned-shared-template"`))
			Expect(output).To(ContainSubstring(`spec.ownershipType: pinned to Public by template "pinned-shared-template"`))
			expectSharingTypes(sharingTypeOwnerOnly)

			By("switching to the shared template with its pinned values")
			_, err = patchWorkspaceForTest(pinnedSwitchWorkspace, workspaceNamespace, fmt.Sprintf(
				`{"spec":{"templateRef":{"name":%q},"accessType":%q,"ownershipType":%q}}`,
				pinnedSharedTemplate, sharingTypePublic, sharingTypePublic))
			Expect(err).NotTo(HaveOccurred())
			expectSharingTypes(sharingTypePublic)

			By("switching back to the private template with its pinned values")
			_, err = patchWorkspaceForTest(pinnedSwitchWorkspace, workspaceNamespace, fmt.Sprintf(
				`{"spec":{"templateRef":{"name":%q},"accessType":%q,"ownershipType":%q}}`,
				pinnedPrivateTemplate, sharingTypeOwnerOnly, sharingTypeOwnerOnly))
			Expect(err).NotTo(HaveOccurred())
			expectSharingTypes(sharingTypeOwnerOnly)

			WaitForWorkspaceToReachCondition(pinnedSwitchWorkspace, workspaceNamespace,
				controller.ConditionTypeAvailable, ConditionTrue)
		})

		// Removing templateRef does not make the workspace standalone when the namespace has a default
		// template: the mutating webhook assigns the default template to any workspace without a
		// templateRef, on update as on create. Removing it is then a switch to the default template, and
		// the usual switch rule applies: the workspace keeps its accessType and ownershipType, so the
		// update must set the default template's pinned values or the validating webhook rejects it.
		// The shared template is made the default here so that its pins (Public) conflict with the
		// workspace's values (OwnerOnly, from the private template). This also checks that removing
		// templateRef is no way around a template's pins.
		It("should hold a workspace that removes its templateRef to the default template's pins", func() {
			createSwitchWorkspace()

			By("labelling the shared template as the default template")
			cmd := exec.Command("kubectl", "label", "workspacetemplate", pinnedSharedTemplate,
				"-n", SharedNamespace, "workspace.jupyter.org/default-template=true")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("removing only the templateRef: the webhook assigns the shared default template, " +
				"whose pins the workspace's OwnerOnly values break")
			output, err := patchWorkspaceForTest(pinnedSwitchWorkspace, workspaceNamespace, `{"spec":{"templateRef":null}}`)
			Expect(err).To(HaveOccurred(), "the default template's pins apply to the workspace")
			Expect(output).To(ContainSubstring(`spec.accessType: pinned to Public by template "pinned-shared-template"`))

			By("removing the templateRef and setting the default template's pinned values")
			_, err = patchWorkspaceForTest(pinnedSwitchWorkspace, workspaceNamespace, fmt.Sprintf(
				`{"spec":{"templateRef":null,"accessType":%q,"ownershipType":%q}}`, sharingTypePublic, sharingTypePublic))
			Expect(err).NotTo(HaveOccurred())

			output, err = kubectlGet("workspace", pinnedSwitchWorkspace, workspaceNamespace, "{.spec.templateRef.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(pinnedSharedTemplate))
			expectSharingTypes(sharingTypePublic)
		})
	})
})
