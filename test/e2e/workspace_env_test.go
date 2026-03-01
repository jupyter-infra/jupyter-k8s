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

var _ = Describe("Workspace Environment Variables", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "env"
		subgroupBase       = "base"
	)

	AfterEach(func() {
		deleteResourcesForEnvTest()
	})

	Context("Environment Variables in Workspace Spec", func() {
		It("should create workspace with env variables and reach available", func() {
			workspaceName := "workspace-with-env"
			workspaceFilename := "workspace-with-env"

			By("creating workspace with environment variables")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupBase)

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying Available=True, Progressing=False, Degraded=False, Stopped=False")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeAvailable:   ConditionTrue,
				ConditionTypeStopped:     ConditionFalse,
			})

			By("retrieving deployment name from workspace status")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying environment variables are set in the deployment")
			envVars, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[*].name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(envVars).To(ContainSubstring("MY_VAR"))
			Expect(envVars).To(ContainSubstring("ANOTHER_VAR"))

			By("verifying environment variable values")
			myVarValue, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='MY_VAR')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(myVarValue).To(Equal("my-value"))

			anotherVarValue, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='ANOTHER_VAR')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(anotherVarValue).To(Equal("another-value"))
		})

		It("should update workspace with modified env variables and reach available", func() {
			workspaceName := "workspace-env-update"
			workspaceFilename := "workspace-env-update"

			By("creating workspace with initial environment variables")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupBase)

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("updating workspace with modified environment variables")
			patchCmd := `{"spec":{"containerConfig":{"env":[{"name":"MY_VAR","value":"updated-value"},` +
				`{"name":"NEW_VAR","value":"new-value"}]}}}`
			cmd := exec.Command("kubectl", "patch", "workspace", workspaceName,
				"-n", workspaceNamespace, "--type=merge", "-p", patchCmd)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Available condition to become True again")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying Available=True, Progressing=False, Degraded=False, Stopped=False")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeAvailable:   ConditionTrue,
				ConditionTypeStopped:     ConditionFalse,
			})

			By("retrieving deployment name from workspace status")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying environment variables were updated in deployment")
			myVarValue, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='MY_VAR')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(myVarValue).To(Equal("updated-value"))

			newVarValue, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='NEW_VAR')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(newVarValue).To(Equal("new-value"))
		})

		It("should handle workspace env removal and reach available", func() {
			workspaceName := "workspace-env-removal"
			workspaceFilename := "workspace-env-removal"

			By("creating workspace with environment variables")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupBase)

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("removing environment variables from workspace")
			patchCmd := `{"spec":{"containerConfig":{"env":[]}}}`
			cmd := exec.Command("kubectl", "patch", "workspace", workspaceName,
				"-n", workspaceNamespace, "--type=merge", "-p", patchCmd)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for Available condition to become True again")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying Available=True, Progressing=False, Degraded=False, Stopped=False")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeAvailable:   ConditionTrue,
				ConditionTypeStopped:     ConditionFalse,
			})

			By("retrieving deployment name from workspace status")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying environment variables were removed from deployment")
			envVars, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env}")
			Expect(err).NotTo(HaveOccurred())
			// Should be empty array or null
			Expect(envVars).To(Or(Equal("[]"), Equal("null"), BeEmpty()))
		})

		It("should handle env variables with valueFrom sources", func() {
			workspaceName := "workspace-env-valuefrom"
			workspaceFilename := "workspace-env-valuefrom"

			By("creating a ConfigMap for testing")
			cmd := exec.Command("kubectl", "create", "configmap", "test-config",
				"-n", workspaceNamespace,
				"--from-literal=config-key=config-value",
				"--from-literal=another-key=another-value")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("creating workspace with valueFrom environment variables")
			createWorkspaceForTest(workspaceFilename, groupDir, subgroupBase)

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying Available=True, Progressing=False, Degraded=False, Stopped=False")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				ConditionTypeProgressing: ConditionFalse,
				ConditionTypeDegraded:    ConditionFalse,
				ConditionTypeAvailable:   ConditionTrue,
				ConditionTypeStopped:     ConditionFalse,
			})

			By("retrieving deployment name from workspace status")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying valueFrom environment variables are set in deployment")
			// Check ConfigMap reference
			configMapRef, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='CONFIG_VALUE')].valueFrom.configMapKeyRef.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(configMapRef).To(Equal("test-config"))
		})
	})
})

func deleteResourcesForEnvTest() {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up test ConfigMaps")
	cmd = exec.Command("kubectl", "delete", "configmap", "test-config", "-n", "default",
		"--ignore-not-found")
	_, _ = utils.Run(cmd)

	By("waiting an arbitrary fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
