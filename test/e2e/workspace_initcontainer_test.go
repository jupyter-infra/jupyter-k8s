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

var _ = Describe("Workspace Init Containers", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "initcontainer"
		subgroupBase       = "base"
	)

	AfterEach(func() {
		deleteResourcesForInitContainerTest()
	})

	Context("Init Containers in Workspace Spec", func() {
		It("should create workspace with init containers and reach available", func() {
			workspaceName := "workspace-with-init"

			By("creating workspace with init containers")
			createWorkspaceForTest("workspace-with-init", groupDir, subgroupBase)

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("retrieving deployment name from workspace status")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying init containers are set in the deployment")
			initContainerName, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.initContainers[0].name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(initContainerName).To(Equal("setup-dirs"))

			initContainerImage, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.initContainers[0].image}")
			Expect(err).NotTo(HaveOccurred())
			Expect(initContainerImage).To(Equal("busybox:latest"))
		})
	})

	Context("Init Containers from Template Defaults", func() {
		It("should inherit default init containers from template", func() {
			workspaceName := "workspace-template-init"

			By("creating template with default init containers")
			createWorkspaceForTest("init-container-template", groupDir, subgroupBase)

			By("creating workspace referencing the template")
			createWorkspaceForTest("workspace-template-init", groupDir, subgroupBase)

			By("waiting for Available condition to become True")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				ConditionTypeAvailable,
				ConditionTrue,
			)

			By("retrieving deployment name from workspace status")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying template default init containers are applied")
			initContainerName, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.initContainers[0].name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(initContainerName).To(Equal("template-init"))
		})
	})

	Context("AllowInitContainers Validation", func() {
		It("should reject workspace with init containers when template disallows them", func() {
			By("creating template that disallows init containers")
			createWorkspaceForTest("no-init-container-template", groupDir, subgroupBase)

			By("attempting to create workspace with init containers")
			VerifyCreateWorkspaceRejectedByWebhook(
				"workspace-init-rejected", groupDir, subgroupBase,
				"workspace-init-rejected", workspaceNamespace,
			)
		})
	})
})

func deleteResourcesForInitContainerTest() {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up templates")
	cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("waiting for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
