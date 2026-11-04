//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
)

var _ = Describe("Workspace Template Base", Ordered, func() {
	const (
		workspaceNamespace   = "default"
		groupDir             = "template"
		subgroupBase         = "base"
		baseTemplateName     = "base-template"
		baseTemplateFilename = "base-template"
	)

	AfterEach(func() {
		deleteResourcesForTemplateTest()
	})

	Context("Creation and Usage", func() {
		It("should create WorkspaceTemplate successfully", func() {
			By("creating the template")
			createTemplateForTest(baseTemplateFilename, groupDir, subgroupBase)

			By("verifying production template exists")
			Expect(ResourceExists("workspacetemplate", baseTemplateName, SharedNamespace, "{.metadata.name}")).
				To(BeTrue(), "WorkspaceTemplate should exist")
		})

		It("should create Workspace using template and pass validation", func() {
			workspaceName := "valid-template-ref-workspace"
			workspaceFilename := "valid-template-ref-workspace"

			By("creating the template")
			createTemplateForTest(baseTemplateName, groupDir, subgroupBase)

			By("applying workspace with template reference")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupBase)

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
		})

		It("should reject a Workspace referencing a non-existent template", func() {
			workspaceName := "invalid-template-ref-workspace"
			workspaceFilename := "invalid-template-ref-workspace"

			By("creating the template")
			createTemplateForTest(baseTemplateName, groupDir, subgroupBase)

			By("applying attempting to create the workspace")
			VerifyCreateWorkspaceRejectedByWebhook(workspaceFilename, groupDir, subgroupBase, workspaceName, workspaceNamespace)
		})
	})
})
