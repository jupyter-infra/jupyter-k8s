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

var _ = Describe("Workspace Template Defaults", Ordered, func() {
	const (
		workspaceNamespace      = "default"
		groupDir                = "template"
		subgroupBase            = "base"
		subgroupDefaults        = "defaults"
		baseTemplateName        = "base-template"
		baseTemplateFilename    = "base-template"
		envTemplateFilename     = "env-template"
		defaultVolumesTemplate  = "default-volumes-template"
		defaultVolumesWorkspace = "default-volumes-workspace"
	)

	AfterEach(func() {
		deleteResourcesForTemplateTest()
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
						expected:    valueTrue,
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
						expected:    valueTrue,
					},
					{
						description: "verifying workspace inherited container security context runAsUser",
						jsonPath:    "{.spec.containerSecurityContext.runAsUser}",
						expected:    "1000",
					},
				},
			)
		})

		It("should inherit readiness probe from template", func() {
			templateFilename := "readiness-probe-template"
			workspaceName := "readiness-probe-workspace"
			workspaceFilename := "readiness-probe-workspace"

			By("creating template with default readiness probe")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating workspace without a readiness probe")
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
						description: "verifying workspace inherited readiness probe port",
						jsonPath:    "{.spec.readinessProbe.tcpSocket.port}",
						expected:    "8888",
					},
					{
						description: "verifying workspace inherited readiness probe failureThreshold",
						jsonPath:    "{.spec.readinessProbe.failureThreshold}",
						expected:    "30",
					},
				},
			)

			By("verifying the readiness probe reached the deployment's primary container")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			probePort, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].readinessProbe.tcpSocket.port}")
			Expect(err).NotTo(HaveOccurred())
			Expect(probePort).To(Equal("8888"))
		})

		It("should let workspace readiness probe take precedence over the template default", func() {
			templateFilename := "readiness-probe-template"
			workspaceName := "readiness-probe-override-workspace"
			workspaceFilename := "readiness-probe-override-workspace"

			By("creating template with a default readiness probe (failureThreshold 30)")
			createTemplateForTest(templateFilename, groupDir, subgroupDefaults)

			By("creating workspace that specifies its own readiness probe (failureThreshold 7)")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupDefaults)

			By("verifying workspace becomes available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying the workspace probe was NOT overwritten by the template default")
			testTemplateFeaturesInheritance(
				workspaceName,
				workspaceNamespace,
				[]valueTestCaseForTemplateTest{
					{
						description: "verifying workspace readiness probe failureThreshold is preserved",
						jsonPath:    "{.spec.readinessProbe.failureThreshold}",
						expected:    "7",
					},
				},
			)

			By("verifying the workspace probe reached the deployment's primary container")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			probeThreshold, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].readinessProbe.failureThreshold}")
			Expect(err).NotTo(HaveOccurred())
			Expect(probeThreshold).To(Equal("7"))
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
						jsonPath:    jsonPathVolumeName,
						expected:    volumeNameTeamData,
					},
					{
						description: "verifying workspace inherited volume PVC name",
						jsonPath:    jsonPathVolumePVCName,
						expected:    pvcNameSharedTeamData,
					},
					{
						description: "verifying workspace inherited volume mount path",
						jsonPath:    "{.spec.volumes[0].mountPath}",
						expected:    "/data",
					},
				},
			)

			By("verifying volume is mounted in the deployment")
			VerifyWorkspaceVolumeMount(workspaceName, workspaceNamespace, volumeNameTeamData, "/data")
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
						jsonPath:    jsonPathVolumeName,
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

			By("verifying both workspaces inherited the same volume from template")
			testTemplateFeaturesInheritance(
				workspace1Name,
				workspaceNamespace,
				[]valueTestCaseForTemplateTest{
					{
						description: "verifying first workspace inherited volume name",
						jsonPath:    jsonPathVolumeName,
						expected:    volumeNameTeamData,
					},
					{
						description: "verifying first workspace inherited volume PVC",
						jsonPath:    jsonPathVolumePVCName,
						expected:    pvcNameSharedTeamData,
					},
				},
			)

			testTemplateFeaturesInheritance(
				workspace2Name,
				workspaceNamespace,
				[]valueTestCaseForTemplateTest{
					{
						description: "verifying second workspace inherited volume name",
						jsonPath:    jsonPathVolumeName,
						expected:    volumeNameTeamData,
					},
					{
						description: "verifying second workspace inherited volume PVC",
						jsonPath:    jsonPathVolumePVCName,
						expected:    pvcNameSharedTeamData,
					},
				},
			)
		})
	})
})
