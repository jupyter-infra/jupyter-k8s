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

type valueTestCaseForTemplateTest struct {
	description string
	jsonPath    string
	expected    string
}

var _ = Describe("Workspace Template", Ordered, func() {
	// Define a test case structure for template feature inheritance tests

	const (
		workspaceNamespace   = "default"
		groupDir             = "template"
		subgroupBase         = "base"
		subgroupValidation   = "validation"
		subgroupMutability   = "mutability"
		subgroupDefaults     = "defaults"
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

	Context("Validation", func() {
		It("should reject workspace with image not in allowlist", func() {
			workspaceName := "rejected-image-workspace"
			workspaceFilename := "rejected-image-workspace"

			By("creating the template")
			createTemplateForTest(baseTemplateName, groupDir, subgroupBase)

			By("attempting to create workspace with invalid image")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupValidation, workspaceName, workspaceNamespace)
		})

		It("should reject workspace exceeding CPU bounds", func() {
			workspaceName := "cpu-exceed-workspace"
			workspaceFilename := "cpu-exceed-workspace"

			By("creating the template")
			createTemplateForTest(baseTemplateName, groupDir, subgroupBase)

			By("attempting to create workspace with out of bounds cpu")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupValidation, workspaceName, workspaceNamespace)
		})

		It("should accept workspace with valid overrides", func() {
			workspaceName := "valid-overrides-workspace"
			workspaceFilename := "valid-overrides-workspace"

			By("creating the template")
			createTemplateForTest(baseTemplateName, groupDir, subgroupBase)

			By("creating workspace with valid resource overrides")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupValidation)

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
			})
		})

		It("should reject custom images when allowCustomImages is false", func() {
			restrictedTemplateFilename := "restricted-images-template"
			workspaceName := "restricted-images-workspace"
			workspaceFilename := "restricted-images-workspace"

			By("creating template with allowCustomImages: false")
			createTemplateForTest(restrictedTemplateFilename, groupDir, subgroupValidation)

			By("attempting to create workspace with custom image")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupValidation, workspaceName, workspaceNamespace)
		})

		It("should allow any image when allowCustomImages is true", func() {
			permissiveTemplateFilename := "allow-custom-images-template"

			workspaceName := "allow-custom-images-workspace"
			workspaceFilename := "allow-custom-images-workspace"

			By("creating template with allowCustomImages: true")
			createTemplateForTest(permissiveTemplateFilename, groupDir, subgroupValidation)

			By("attempting to create workspace with custom image")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupValidation)

			// do NOT check the workspace status here, this image doesn't exist

			By("verifying workspace was created with custom image")
			output, err := kubectlGet("workspace", workspaceName, workspaceNamespace, "{.spec.image}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("custom/authorized:latest"))
		})
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

	Context("Defaults", func() {
		It("should apply template defaults during workspace creation", func() {

			workspaceName := "template-defaults-workspace"
			workspaceFilename := "template-defaults-workspace"

			By("creating the base template")
			createTemplateForTest(baseTemplateFilename, groupDir, subgroupBase)

			By("creating workspace without specifying image, resources, or storage")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupDefaults)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			testTemplateFeaturesInheritance(
				workspaceName,
				workspaceNamespace,
				[]valueTestCaseForTemplateTest{
					{
						description: "verifying workspace inherited image",
						jsonPath:    "{.spec.image}",
						expected:    "jk8s-application-jupyter-uv:latest",
					},
					{
						description: "verifying workspace inherited cpu resource",
						jsonPath:    "{.spec.resources.requests.cpu}",
						expected:    "200m",
					},
					{
						description: "verifying workspace inherited memory resource",
						jsonPath:    "{.spec.resources.requests.memory}",
						expected:    "256Mi",
					},
					{
						description: "verifying workspace inherited storage size",
						jsonPath:    "{.spec.storage.size}",
						expected:    "1Gi",
					},
				},
			)

			By("verifying template tracking label was added")
			output, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.metadata.labels['workspace\\.jupyter\\.org/template-name']}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal(baseTemplateName))
		})

		It("should inherit default access strategy from template", func() {
			accessStrategyName := "inherited-access-strategy"
			accessStrategyFilename := "inherited-access-strategy"

			templateFilename := "access-strategy-template"

			workspaceName := "access-strategy-inheritance-workspace"
			workspaceFilename := "access-strategy-inheritance-workspace"

			By("creating a test access strategy")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, subgroupDefaults)

			By("creating a template referencing the strategy")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating workspace without access strategy but with template")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupDefaults)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			testTemplateFeaturesInheritance(
				workspaceName,
				workspaceNamespace,
				[]valueTestCaseForTemplateTest{
					{
						description: "verifying workspace inherited access type",
						jsonPath:    "{.spec.accessStrategy.name}",
						expected:    accessStrategyName,
					},
					{
						description: "verifying workspace inherited app type",
						jsonPath:    "{.spec.accessStrategy.namespace}",
						expected:    SharedNamespace,
					},
				},
			)
		})

		It("should inherit access type and app type from template", func() {
			templateFilename := "access-and-app-type-template"
			workspaceName := "access-and-app-type-workspace"
			workspaceFilename := "access-and-app-type-workspace"

			By("creating template with default app type")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating workspace without app type")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupDefaults)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			testTemplateFeaturesInheritance(
				workspaceName,
				workspaceNamespace,
				[]valueTestCaseForTemplateTest{
					{
						description: "verifying workspace inherited access type",
						jsonPath:    "{.spec.accessType}",
						expected:    "OwnerOnly",
					},
					{
						description: "verifying workspace inherited app type",
						jsonPath:    "{.spec.appType}",
						expected:    "jupyter-lab",
					},
				},
			)
		})

		It("should inherit lifecycle and idle shutdown from template", func() {
			templateFilename := "lifecycle-template"
			workspaceName := "lifecycle-workspace"
			workspaceFilename := "lifecycle-workspace"

			By("creating template with default lifecycle and idle shutdown")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating workspace without lifecycle or idle shutdown")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupDefaults)

			// This workspace fails to become available (invalid command)
			// It's okay, the test isn't verifying that

			testTemplateFeaturesInheritance(
				workspaceName,
				workspaceNamespace,
				[]valueTestCaseForTemplateTest{
					{
						description: "verifying workspace inherited lifecycle",
						jsonPath:    "{.spec.lifecycle.postStart.exec.command[0]}",
						expected:    "/bin/sh",
					},
					{
						description: "verifying workspace inherited idle shutdown",
						jsonPath:    "{.spec.idleShutdown.enabled}",
						expected:    "true",
					},
					{
						description: "verifying workspace inherited idle timeout in minutes",
						jsonPath:    "{.spec.idleShutdown.idleTimeoutInMinutes}",
						expected:    "30",
					},
				},
			)
		})

		It("should inherit pod security context from template", func() {
			templateFilename := "security-context-template"
			workspaceName := "security-context-workspace"
			workspaceFilename := "security-context-workspace"

			By("creating template with default security context")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating workspace without security context")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupDefaults)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			testTemplateFeaturesInheritance(
				workspaceName,
				workspaceNamespace,
				[]valueTestCaseForTemplateTest{
					{
						description: "verifying workspace inherited pod security context",
						jsonPath:    "{.spec.podSecurityContext.fsGroup}",
						expected:    "1000",
					},
					{
						description: "verifying runAsUser was inherited",
						jsonPath:    "{.spec.podSecurityContext.runAsUser}",
						expected:    "1000",
					},
				},
			)
		})
	})
})

func deleteResourcesForTemplateTest() {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=90s")
	_, _ = utils.Run(cmd)

	By("cleaning up template")
	cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)

	By("cleaning up access strategies")
	cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)

	By("waiting for resources to be fully deleted")
	time.Sleep(2 * time.Second)
}

// Helper function to test template feature inheritance
//
//nolint:unparam
func testTemplateFeaturesInheritance(
	workspaceName string,
	workspaceNamespace string,
	testCases []valueTestCaseForTemplateTest,
) {
	// Test each value case
	for _, tc := range testCases {
		By(tc.description)
		output, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
			tc.jsonPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(output).To(Equal(tc.expected))
	}
}
