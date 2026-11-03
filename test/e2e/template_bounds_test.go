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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

var _ = Describe("Workspace Template Bounds", Ordered, func() {
	const (
		workspaceNamespace      = "default"
		groupDir                = "template"
		subgroupBase            = "base"
		subgroupBounds          = "bounds"
		baseTemplateName        = "base-template"
		defaultLabelsTemplate   = "default-labels-template"
		envRequirementsTemplate = "env-requirements-template"
		validEnvWorkspace       = "valid-env-workspace"
	)

	AfterEach(func() {
		deleteResourcesForTemplateTest()
	})

	Context("Validation", func() {
		It("should reject workspace with image not in allowlist", func() {
			workspaceName := "rejected-image-workspace"
			workspaceFilename := "rejected-image-workspace"

			By("creating the template")
			createTemplateForTest(baseTemplateName, groupDir, subgroupBase)

			By("attempting to create workspace with invalid image")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupBounds, workspaceName, workspaceNamespace)
		})

		It("should reject workspace exceeding resource request bounds", func() {
			workspaceName := "resource-request-exceed-workspace"
			workspaceFilename := "resource-request-exceed-workspace"

			By("creating the template")
			createTemplateForTest(baseTemplateName, groupDir, subgroupBase)

			By("attempting to create workspace with out of bounds resource requests")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupBounds, workspaceName, workspaceNamespace)
		})

		It("should reject workspace exceeding resource limit bounds", func() {
			workspaceName := "resource-limit-exceed-workspace"
			workspaceFilename := "resource-limit-exceed-workspace"

			By("creating the template")
			createTemplateForTest(baseTemplateName, groupDir, subgroupBase)

			By("attempting to create workspace with out of bounds resource limits")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupBounds, workspaceName, workspaceNamespace)
		})

		It("should accept workspace with valid overrides", func() {
			workspaceName := "valid-overrides-workspace"
			workspaceFilename := "valid-overrides-workspace"

			By("creating the template")
			createTemplateForTest(baseTemplateName, groupDir, subgroupBase)

			By("creating workspace with valid resource overrides")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupBounds)

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

		It("should reject custom images when allowCustomImages is false", func() {
			restrictedTemplateFilename := "restricted-images-template"
			workspaceName := "restricted-images-workspace"
			workspaceFilename := "restricted-images-workspace"

			By("creating template with allowCustomImages: false")
			createTemplateForTest(restrictedTemplateFilename, groupDir, subgroupBounds)

			By("attempting to create workspace with custom image")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupBounds, workspaceName, workspaceNamespace)
		})

		It("should allow any image when allowCustomImages is true", func() {
			permissiveTemplateFilename := "allow-custom-images-template"

			workspaceName := "allow-custom-images-workspace"
			workspaceFilename := "allow-custom-images-workspace"

			By("creating template with allowCustomImages: true")
			createTemplateForTest(permissiveTemplateFilename, groupDir, subgroupBounds)

			By("attempting to create workspace with custom image")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupBounds)

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
			createTemplateForTest(templateFilename, groupDir, subgroupBounds)

			By("creating workspace with matching labels")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupBounds)

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
			createTemplateForTest(templateFilename, groupDir, subgroupBounds)

			By("attempting to create workspace with mismatched label value")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupBounds, workspaceName, workspaceNamespace)
		})

		It("should inject baseLabels when workspace has no labels", func() {
			templateFilename := defaultLabelsTemplate
			workspaceName := "no-labels-workspace"
			workspaceFilename := "no-labels-workspace"

			By("creating template with baseLabels")
			createTemplateForTest(templateFilename, groupDir, subgroupBounds)

			By("creating workspace with no labels")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupBounds)

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
			createTemplateForTest(templateFilename, groupDir, subgroupBounds)

			By("attempting to create workspace without required label")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupBounds, workspaceName, workspaceNamespace)
		})

		It("should accept workspace with valid env matching requirements", func() {
			templateFilename := envRequirementsTemplate
			workspaceName := validEnvWorkspace
			workspaceFilename := validEnvWorkspace

			By("creating template with env requirements")
			createTemplateForTest(templateFilename, groupDir, subgroupBounds)

			By("creating workspace with valid env")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupBounds)

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
			createTemplateForTest(templateFilename, groupDir, subgroupBounds)

			By("attempting to create workspace without required env")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupBounds, workspaceName, workspaceNamespace)
		})

		It("should reject workspace with env value not matching regex", func() {
			templateFilename := envRequirementsTemplate
			workspaceName := "invalid-env-regex-workspace"
			workspaceFilename := "invalid-env-regex-workspace"

			By("creating template with env requirements")
			createTemplateForTest(templateFilename, groupDir, subgroupBounds)

			By("attempting to create workspace with invalid env value")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupBounds, workspaceName, workspaceNamespace)
		})

		It("should reject workspace with optional env var failing regex", func() {
			templateFilename := envRequirementsTemplate
			workspaceName := "optional-env-bad-regex-workspace"
			workspaceFilename := "optional-env-bad-regex-workspace"

			By("creating template with env requirements including optional regex")
			createTemplateForTest(templateFilename, groupDir, subgroupBounds)

			By("attempting to create workspace with optional env var that fails regex")
			VerifyCreateWorkspaceRejectedByWebhook(
				workspaceFilename, groupDir, subgroupBounds, workspaceName, workspaceNamespace)
		})

		It("should reject update that violates env requirements", func() {
			templateFilename := envRequirementsTemplate
			workspaceName := validEnvWorkspace
			workspaceFilename := validEnvWorkspace

			By("creating template with env requirements")
			createTemplateForTest(templateFilename, groupDir, subgroupBounds)

			By("creating workspace with valid env")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupBounds)

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
			createTemplateForTest(templateFilename, groupDir, subgroupBounds)

			By("creating workspace with valid env")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupBounds)

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

	Context("Self-consistency", func() {
		It("should reject a template whose defaultImage is not in a non-empty allowedImages", func() {
			templateFilename := "inconsistent-default-image-template"
			templateName := "inconsistent-default-image-template"

			By("attempting to create a template whose default image would be un-creatable")
			path := BuildTestResourcePath(templateFilename, groupDir, subgroupBounds)
			cmd := exec.Command("kubectl", "apply", "-f", path)
			output, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "template webhook should reject a self-inconsistent image policy")
			Expect(output).To(ContainSubstring("defaultImage"),
				"rejection should explain the defaultImage / allowedImages inconsistency")

			By("verifying the template was not created")
			cmd = exec.Command("kubectl", verbGet, "workspacetemplate", templateName,
				"-n", SharedNamespace, "--ignore-not-found")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(BeEmpty(), "template should not exist after webhook rejection")
		})
	})

	Context("Pinned access type, ownership type and security contexts", func() {
		const (
			pinnedPrivateTemplate   = "pinned-private-template"
			pinnedDefaultWorkspace  = "pinned-default-workspace"
			tightenedTemplate       = "tightened-template"
			tightenedWorkspace      = "tightened-workspace"
			pinnedAccessTypeMessage = `spec.accessType: pinned to OwnerOnly by template "pinned-private-template"`
		)

		It("should reject a create that turns a private template's workspace Public", func() {
			createTemplateForTest(pinnedPrivateTemplate, groupDir, subgroupBounds)

			path := BuildTestResourcePath("pinned-public-workspace", groupDir, subgroupBounds)
			cmd := exec.Command("kubectl", "apply", "-f", path)
			output, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "webhook should reject a workspace breaking the access type pin")
			Expect(output).To(ContainSubstring(pinnedAccessTypeMessage))
		})

		It("should reject a create that requests a privileged container under a locked template", func() {
			createTemplateForTest(pinnedPrivateTemplate, groupDir, subgroupBounds)

			path := BuildTestResourcePath("pinned-privileged-workspace", groupDir, subgroupBounds)
			cmd := exec.Command("kubectl", "apply", "-f", path)
			output, err := utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "webhook should reject a workspace breaking the security context lock")
			Expect(output).To(ContainSubstring("spec.containerSecurityContext: pinned"))
		})

		It("should apply the pinned values and reject an update that breaks them", func() {
			createTemplateForTest(pinnedPrivateTemplate, groupDir, subgroupBounds)

			By("creating a workspace that sets nothing")
			createWorkspaceForTest(pinnedDefaultWorkspace, groupDir, subgroupBounds)

			WaitForWorkspaceToReachCondition(pinnedDefaultWorkspace, workspaceNamespace,
				controller.ConditionTypeAvailable, ConditionTrue)

			testTemplateFeaturesInheritance(pinnedDefaultWorkspace, workspaceNamespace, []valueTestCaseForTemplateTest{
				{description: "verifying the pinned access type",
					jsonPath: jsonPathAccessType, expected: sharingTypeOwnerOnly},
				{description: "verifying the pinned ownership type",
					jsonPath: jsonPathOwnershipType, expected: sharingTypeOwnerOnly},
				{description: "verifying the locked pod security context",
					jsonPath: "{.spec.podSecurityContext.fsGroup}", expected: "1000"},
				{description: "verifying the locked container security context",
					jsonPath: "{.spec.containerSecurityContext.allowPrivilegeEscalation}", expected: "false"},
			})

			By("verifying the Deployment runs with the template's container security context")
			deploymentName := GetWorkspaceDeploymentName(pinnedDefaultWorkspace, workspaceNamespace)
			output, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].securityContext.allowPrivilegeEscalation}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("false"))

			By("turning the workspace Public")
			output, err = patchWorkspaceForTest(pinnedDefaultWorkspace, workspaceNamespace, `{"spec":{"accessType":"Public"}}`)
			Expect(err).To(HaveOccurred(), "webhook should reject an update breaking the access type pin")
			Expect(output).To(ContainSubstring(pinnedAccessTypeMessage))

			By("requesting a privileged container")
			output, err = patchWorkspaceForTest(pinnedDefaultWorkspace, workspaceNamespace,
				`{"spec":{"containerSecurityContext":{"privileged":true}}}`)
			Expect(err).To(HaveOccurred(), "webhook should reject an update breaking the security context lock")
			Expect(output).To(ContainSubstring("spec.containerSecurityContext: pinned"))
		})

		It("should keep a workspace that a template pin leaves non-compliant running until its next update", func() {
			createTemplateForTest(tightenedTemplate, groupDir, subgroupBounds)

			By("creating a Public workspace while the access type is not pinned")
			createWorkspaceForTest(tightenedWorkspace, groupDir, subgroupBounds)
			WaitForWorkspaceToReachCondition(tightenedWorkspace, workspaceNamespace,
				controller.ConditionTypeAvailable, ConditionTrue)

			By("pinning the access type on the template")
			cmd := exec.Command("kubectl", "patch", "workspacetemplate", tightenedTemplate,
				"-n", SharedNamespace, "--type=merge", "-p", `{"spec":{"allowCustomAccessType":false}}`)
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("Template constraints changed"),
				"template webhook should warn that the pin may break workspaces")

			By("verifying the workspace keeps running")
			VerifyConsistentWorkspaceConditions(tightenedWorkspace, workspaceNamespace, map[string]string{
				controller.ConditionTypeAvailable: ConditionTrue,
			}, "10s", "2s")

			By("rejecting a spec update")
			output, err = patchWorkspaceForTest(tightenedWorkspace, workspaceNamespace, `{"spec":{"displayName":"renamed"}}`)
			Expect(err).To(HaveOccurred(), "the next spec update must comply with the pin")
			Expect(output).To(ContainSubstring(`spec.accessType: pinned to OwnerOnly by template "tightened-template"`))

			By("letting the user stop the workspace anyway")
			_, err = patchWorkspaceForTest(tightenedWorkspace, workspaceNamespace,
				fmt.Sprintf(`{"spec":{"desiredStatus":%q}}`, controller.DesiredStateStopped))
			Expect(err).NotTo(HaveOccurred(), "stopping a workspace is always allowed")
		})
	})
})
