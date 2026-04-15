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

type valueTestCaseForTemplateTest struct {
	description string
	jsonPath    string
	expected    string
}

var _ = Describe("Workspace Template", Ordered, func() {
	// Define a test case structure for template feature inheritance tests

	const (
		workspaceNamespace      = "default"
		groupDir                = "template"
		subgroupBase            = "base"
		subgroupValidation      = "validation"
		subgroupMutability      = "mutability"
		subgroupDefaults        = "defaults"
		baseTemplateName        = "base-template"
		baseTemplateFilename    = "base-template"
		defaultLabelsTemplate   = "default-labels-template"
		envRequirementsTemplate = "env-requirements-template"
		validEnvWorkspace       = "valid-env-workspace"
		envTemplateFilename     = "env-template"
		defaultVolumesTemplate  = "default-volumes-template"
		defaultVolumesWorkspace = "default-volumes-workspace"
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

		It("should reject workspace exceeding resource request bounds", func() {
			workspaceName := "resource-request-exceed-workspace"
			workspaceFilename := "resource-request-exceed-workspace"

			By("creating the template")
			createTemplateForTest(baseTemplateName, groupDir, subgroupBase)

			By("attempting to create workspace with out of bounds resource requests")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupValidation, workspaceName, workspaceNamespace)
		})

		It("should reject workspace exceeding resource limit bounds", func() {
			workspaceName := "resource-limit-exceed-workspace"
			workspaceFilename := "resource-limit-exceed-workspace"

			By("creating the template")
			createTemplateForTest(baseTemplateName, groupDir, subgroupBase)

			By("attempting to create workspace with out of bounds resource limits")
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

		It("should accept workspace with matching default labels", func() {
			templateFilename := defaultLabelsTemplate
			workspaceName := "matching-labels-workspace"
			workspaceFilename := "matching-labels-workspace"

			By("creating template with default labels")
			createTemplateForTest(templateFilename, groupDir, subgroupValidation)

			By("creating workspace with matching labels")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupValidation)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying labels are set correctly")
			output, err := kubectlGet("workspace", workspaceName, workspaceNamespace, "{.metadata.labels.env}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("production"))
		})

		It("should reject workspace with mismatched default labels", func() {
			templateFilename := defaultLabelsTemplate
			workspaceName := "mismatched-labels-workspace"
			workspaceFilename := "mismatched-labels-workspace"

			By("creating template with default labels")
			createTemplateForTest(templateFilename, groupDir, subgroupValidation)

			By("attempting to create workspace with mismatched label value")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupValidation, workspaceName, workspaceNamespace)
		})

		It("should inject baseLabels when workspace has no labels", func() {
			templateFilename := defaultLabelsTemplate
			workspaceName := "no-labels-workspace"
			workspaceFilename := "no-labels-workspace"

			By("creating template with baseLabels")
			createTemplateForTest(templateFilename, groupDir, subgroupValidation)

			By("creating workspace with no labels")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupValidation)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying baseLabels were injected")
			output, err := kubectlGet("workspace", workspaceName, workspaceNamespace, "{.metadata.labels.env}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("production"))

			output, err = kubectlGet("workspace", workspaceName, workspaceNamespace, "{.metadata.labels.managed-by}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("platform-team"))

			By("verifying labels propagated to pod")
			podName, err := kubectlGetByLabels("pod",
				fmt.Sprintf("%s=%s", controller.LabelWorkspaceName, workspaceName),
				workspaceNamespace,
				"{.items[0].metadata.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(podName).NotTo(BeEmpty())

			podEnv, err := kubectlGet("pod", podName, workspaceNamespace, "{.metadata.labels.env}")
			Expect(err).NotTo(HaveOccurred())
			Expect(podEnv).To(Equal("production"))

			podManagedBy, err := kubectlGet("pod", podName, workspaceNamespace, "{.metadata.labels.managed-by}")
			Expect(err).NotTo(HaveOccurred())
			Expect(podManagedBy).To(Equal("platform-team"))
		})

		It("should reject workspace missing a required label", func() {
			templateFilename := "required-label-template"
			workspaceName := "missing-required-label-workspace"
			workspaceFilename := "missing-required-label-workspace"

			By("creating template with required label")
			createTemplateForTest(templateFilename, groupDir, subgroupValidation)

			By("attempting to create workspace without required label")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupValidation, workspaceName, workspaceNamespace)
		})

		It("should accept workspace with valid env matching requirements", func() {
			templateFilename := envRequirementsTemplate
			workspaceName := validEnvWorkspace
			workspaceFilename := validEnvWorkspace

			By("creating template with env requirements")
			createTemplateForTest(templateFilename, groupDir, subgroupValidation)

			By("creating workspace with valid env")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupValidation)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying required env var is present")
			regionValue, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.env[?(@.name=='AWS_REGION')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(regionValue).To(Equal("us-west-2"))

			By("verifying template baseEnv was merged in")
			defaultValue, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.env[?(@.name=='DEFAULT_VAR')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(defaultValue).To(Equal("default-value"))
		})

		It("should reject workspace missing required env variable", func() {
			templateFilename := envRequirementsTemplate
			workspaceName := "missing-required-env-workspace"
			workspaceFilename := "missing-required-env-workspace"

			By("creating template with env requirements")
			createTemplateForTest(templateFilename, groupDir, subgroupValidation)

			By("attempting to create workspace without required env")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupValidation, workspaceName, workspaceNamespace)
		})

		It("should reject workspace with env value not matching regex", func() {
			templateFilename := envRequirementsTemplate
			workspaceName := "invalid-env-regex-workspace"
			workspaceFilename := "invalid-env-regex-workspace"

			By("creating template with env requirements")
			createTemplateForTest(templateFilename, groupDir, subgroupValidation)

			By("attempting to create workspace with invalid env value")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupValidation, workspaceName, workspaceNamespace)
		})

		It("should reject workspace with optional env var failing regex", func() {
			templateFilename := envRequirementsTemplate
			workspaceName := "optional-env-bad-regex-workspace"
			workspaceFilename := "optional-env-bad-regex-workspace"

			By("creating template with env requirements including optional regex")
			createTemplateForTest(templateFilename, groupDir, subgroupValidation)

			By("attempting to create workspace with optional env var that fails regex")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupValidation, workspaceName, workspaceNamespace)
		})

		It("should reject update that violates env requirements", func() {
			templateFilename := envRequirementsTemplate
			workspaceName := validEnvWorkspace
			workspaceFilename := validEnvWorkspace

			By("creating template with env requirements")
			createTemplateForTest(templateFilename, groupDir, subgroupValidation)

			By("creating workspace with valid env")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupValidation)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("patching workspace to violate env regex requirement")
			patchCmd := `{"spec":{"env":[{"name":"AWS_REGION","value":"not-valid"}]}}`
			cmd := exec.Command("kubectl", "patch", "workspace", workspaceName,
				"-n", workspaceNamespace, "--type=merge", "-p", patchCmd)
			_, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "Expected webhook to reject update that violates env regex")
		})

		It("should warn when template envRequirements change", func() {
			templateFilename := envRequirementsTemplate
			workspaceName := validEnvWorkspace
			workspaceFilename := validEnvWorkspace

			By("creating template with env requirements")
			createTemplateForTest(templateFilename, groupDir, subgroupValidation)

			By("creating workspace with valid env")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupValidation)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("updating template envRequirements")
			patchCmd := `{"spec":{"envRequirements":[{"name":"NEW_REQUIRED","required":true}]}}`
			cmd := exec.Command("kubectl", "patch", "workspacetemplate", envRequirementsTemplate,
				"-n", SharedNamespace, "--type=merge", "-p", patchCmd)
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			// Template webhook returns a warning when constraints change
			Expect(output).To(ContainSubstring("Warning"), "Expected warning about constraint change")
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

		It("should inherit container security context from template", func() {
			templateFilename := "container-security-context-template"
			workspaceName := "container-security-context-workspace"
			workspaceFilename := "container-security-context-workspace"

			By("creating template with default container security context")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating workspace without container security context")
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
						description: "verifying workspace inherited container security context runAsNonRoot",
						jsonPath:    "{.spec.containerSecurityContext.runAsNonRoot}",
						expected:    "true",
					},
					{
						description: "verifying workspace inherited container security context runAsUser",
						jsonPath:    "{.spec.containerSecurityContext.runAsUser}",
						expected:    "1000",
					},
				},
			)
		})

		It("should inherit container config with env variables from template", func() {
			templateFilename := "env-template"
			workspaceName := "workspace-no-config"
			workspaceFilename := "workspace-no-config"

			By("creating template with baseEnv environment variables")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating workspace without env")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupDefaults)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying workspace inherited template's env variables")
			envVars, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.env[*].name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(envVars).To(ContainSubstring("TEMPLATE_VAR"))
			Expect(envVars).To(ContainSubstring("JUPYTER_ENABLE_LAB"))

			By("verifying environment variable values from template")
			templateVarValue, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.env[?(@.name=='TEMPLATE_VAR')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(templateVarValue).To(Equal("template-value"))

			jupyterLabValue, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.env[?(@.name=='JUPYTER_ENABLE_LAB')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(jupyterLabValue).To(Equal("yes"))
		})

		It("should let workspace env override template baseEnv by name", func() {
			templateFilename := envTemplateFilename
			workspaceName := "workspace-env-override"
			workspaceFilename := "workspace-env-override"

			By("creating template with baseEnv environment variables")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating workspace with its own env")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupDefaults)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying workspace has its own env plus template env merged in")
			envVars, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.env[*].name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(envVars).To(ContainSubstring("WORKSPACE_VAR"))
			// Template vars should be merged in since names don't conflict
			Expect(envVars).To(ContainSubstring("TEMPLATE_VAR"))
			Expect(envVars).To(ContainSubstring("JUPYTER_ENABLE_LAB"))

			By("verifying workspace environment variable value")
			workspaceVarValue, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.env[?(@.name=='WORKSPACE_VAR')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(workspaceVarValue).To(Equal("workspace-value"))
		})

		It("should let workspace env win when name collides with template baseEnv", func() {
			templateFilename := envTemplateFilename
			workspaceName := "env-name-collision-workspace"
			workspaceFilename := "env-name-collision-workspace"

			By("creating template with baseEnv including TEMPLATE_VAR")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating workspace that also sets TEMPLATE_VAR with different value")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupDefaults)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying workspace value wins over template value for same-name env var")
			templateVarValue, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.env[?(@.name=='TEMPLATE_VAR')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(templateVarValue).To(Equal("workspace-wins"))

			By("verifying workspace-only var is present")
			wsOnlyValue, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.env[?(@.name=='WORKSPACE_ONLY')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(wsOnlyValue).To(Equal("only-in-workspace"))

			By("verifying non-conflicting template var was still merged in")
			jupyterLabValue, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.spec.env[?(@.name=='JUPYTER_ENABLE_LAB')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(jupyterLabValue).To(Equal("yes"))
		})

		It("should inherit default volumes from template", func() {
			templateFilename := defaultVolumesTemplate
			workspaceName := defaultVolumesWorkspace
			workspaceFilename := defaultVolumesWorkspace

			By("creating the external PVC that the template references")
			createPvcForTest("shared-team-data-pvc", groupDir, subgroupDefaults)

			By("creating template with defaultVolumes")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating workspace without volumes")
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
						description: "verifying workspace inherited volume name",
						jsonPath:    "{.spec.volumes[0].name}",
						expected:    "team-data",
					},
					{
						description: "verifying workspace inherited volume PVC name",
						jsonPath:    "{.spec.volumes[0].persistentVolumeClaimName}",
						expected:    "shared-team-data",
					},
					{
						description: "verifying workspace inherited volume mount path",
						jsonPath:    "{.spec.volumes[0].mountPath}",
						expected:    "/data",
					},
				},
			)

			By("verifying volume is mounted in the deployment")
			VerifyWorkspaceVolumeMount(workspaceName, workspaceNamespace, "team-data", "/data")
		})

		It("should not override workspace volumes with template default volumes", func() {
			templateFilename := defaultVolumesTemplate
			workspaceName := "default-volumes-override-workspace"
			workspaceFilename := "default-volumes-override-workspace"

			By("creating the external PVC")
			createPvcForTest("shared-team-data-pvc", groupDir, subgroupDefaults)

			By("creating template with defaultVolumes")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating workspace with its own volumes")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupDefaults)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying workspace kept its own volume, not the template default")
			testTemplateFeaturesInheritance(
				workspaceName,
				workspaceNamespace,
				[]valueTestCaseForTemplateTest{
					{
						description: "verifying workspace has its own volume name",
						jsonPath:    "{.spec.volumes[0].name}",
						expected:    "my-data",
					},
					{
						description: "verifying workspace has its own mount path",
						jsonPath:    "{.spec.volumes[0].mountPath}",
						expected:    "/my-custom-path",
					},
				},
			)

			By("verifying only one volume exists (no template default merged in)")
			output, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{range .spec.volumes[*]}{.name}{','}{end}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("my-data,"), "workspace should have only its own volume, not template defaults")
		})

		It("should allow two workspaces to share a default volume from template", func() {
			templateFilename := defaultVolumesTemplate
			workspace1Name := defaultVolumesWorkspace
			workspace1Filename := defaultVolumesWorkspace
			workspace2Name := "default-volumes-workspace-2"
			workspace2Filename := "default-volumes-workspace-2"

			By("creating the shared PVC")
			createPvcForTest("shared-team-data-pvc", groupDir, subgroupDefaults)

			By("creating template with defaultVolumes")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating first workspace without volumes")
			createWorkspaceForTest(workspace1Filename, groupDir, subgroupDefaults)

			By("creating second workspace without volumes")
			createWorkspaceForTest(workspace2Filename, groupDir, subgroupDefaults)

			By("verifying first workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspace1Name,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying second workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspace2Name,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying both workspaces have the volume mounted")
			VerifyWorkspaceVolumeMount(workspace1Name, workspaceNamespace, "team-data", "/data")
			VerifyWorkspaceVolumeMount(workspace2Name, workspaceNamespace, "team-data", "/data")

			By("verifying first workspace can access the shared volume")
			VerifyPodCanAccessExternalVolumes(workspace1Name, workspaceNamespace, "shared-team-data", "/data")

			By("verifying second workspace can access the shared volume")
			VerifyPodCanAccessExternalVolumes(workspace2Name, workspaceNamespace, "shared-team-data", "/data")
		})
	})
})

func deleteResourcesForTemplateTest() {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up templates")
	cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)

	By("cleaning up access strategies")
	cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)

	By("cleaning up standalone PVCs")
	cmd = exec.Command("kubectl", "delete", "pvc", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)

	By("waiting an arbitrary fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
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
