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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

var _ = Describe("Workspace Access Strategy", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "access-strategy"
	)

	// AfterEach - clean up resources after each test
	AfterEach(func() {
		deleteResourcesForAccessStrategyTest(workspaceNamespace)
	})

	// Context for tests with running workspace + access strategy
	Context("Workspace with AccessStrategy and desiredState=running", func() {
		It("should modify deployment with AccessStrategy settings", func() {
			accessStrategyFilename := "access-strategy-with-env"
			workspaceFilename := "workspace-with-env-access-strategy"
			workspaceName := "workspace-with-env-access-strategy"

			By("creating an access strategy with deployment modification")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating a workspace referencing AccessStrategy with desiredState=running")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying conditions Available=True, Progressing=False, Degraded=False, Stopped=False")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeDegraded:    ConditionFalse,
				controller.ConditionTypeAvailable:   ConditionTrue,
				controller.ConditionTypeStopped:     ConditionFalse,
			})

			By("retrieving deployment name from workspace status")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty(), "workspace.status.deploymentName should be set")

			By("verifying the deployment environment variables have been modified")
			envVarValue, deploymentErr := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='JUPYTER_BASE_URL')].value}")
			Expect(deploymentErr).NotTo(HaveOccurred())
			Expect(envVarValue).To(Equal(fmt.Sprintf("/workspaces/%s/%s/", workspaceNamespace, workspaceName)))
		})

		It("should create all access resources defined in AccessStrategy", func() {
			accessStrategyFilename := "access-strategy-with-resources"
			workspaceFilename := "workspace-with-resources-access-strategy"
			workspaceName := "workspace-with-resources-access-strategy"
			networkPolicyName := fmt.Sprintf("workspace-network-policy-%s", workspaceName)
			primaryRouteName := fmt.Sprintf("primary-route-%s", workspaceName)
			authRouteName := fmt.Sprintf("auth-route-%s", workspaceName)

			By("creating an access strategy with access resource templates")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating a Workspace referencing AccessStrategy with desiredState=running")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying conditions Available=True, Progressing=False, Degraded=False, Stopped=False")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeDegraded:    ConditionFalse,
				controller.ConditionTypeAvailable:   ConditionTrue,
				controller.ConditionTypeStopped:     ConditionFalse,
			})

			By("verifying network policy has been created")
			WaitForResourceToExist("networkpolicy", networkPolicyName, workspaceNamespace,
				"{.metadata.name}", 5*time.Minute, 5*time.Second)

			By("verifying primary IngressRoute has been created")
			WaitForResourceToExist("ingressroute.traefik.io", primaryRouteName, workspaceNamespace,
				"{.metadata.name}", 5*time.Minute, 5*time.Second)

			By("verifying auth IngressRoute has been created")
			WaitForResourceToExist("ingressroute.traefik.io", authRouteName, workspaceNamespace,
				"{.metadata.name}", 5*time.Minute, 5*time.Second)

			By("verifying all access resources are referenced in workspace.status.accessResources")
			accessResourceList, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources[*].kind}")
			Expect(err).NotTo(HaveOccurred())
			resources := strings.Fields(accessResourceList)
			Expect(resources).To(HaveLen(3), "Expected 3 access resources (NetworkPolicy and 2 IngressRoutes)")

			By("verifying network policy is referenced in workspace.status.accessResources")
			networkPolicyKind, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				fmt.Sprintf("{.status.accessResources[?(@.name==\"%s\")].kind}", networkPolicyName))
			Expect(err).NotTo(HaveOccurred())
			Expect(networkPolicyKind).To(Equal("NetworkPolicy"))

			By("verifying primary IngressRoute is referenced in workspace.status.accessResources")
			primaryRouteKind, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				fmt.Sprintf("{.status.accessResources[?(@.name==\"%s\")].kind}", primaryRouteName))
			Expect(err).NotTo(HaveOccurred())
			Expect(primaryRouteKind).To(Equal("IngressRoute"))

			By("verifying auth IngressRoute is referenced in workspace.status.accessResources")
			authRouteKind, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				fmt.Sprintf("{.status.accessResources[?(@.name==\"%s\")].kind}", authRouteName))
			Expect(err).NotTo(HaveOccurred())
			Expect(authRouteKind).To(Equal("IngressRoute"))

			By("verifying service name matches in primary IngressRoute matches the workspace service")
			serviceName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.serviceName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceName).NotTo(BeEmpty(), "Service name not found in Workspace status")

			// Verify primary IngressRoute has the correct service name
			primaryRouteServiceName, err := kubectlGet("ingressroute.traefik.io",
				primaryRouteName,
				workspaceNamespace, "{.spec.routes[0].services[0].name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(primaryRouteServiceName).NotTo(BeEmpty(), "Service name not found in primary IngressRoute")
			Expect(primaryRouteServiceName).To(Equal(serviceName),
				"Primary IngressRoute service name should match Workspace service name")
		})

		It("should include access URL in Workspace status", func() {
			accessStrategyFilename := "basic-access-strategy"
			workspaceFilename := "workspace-with-access-strategy"
			workspaceName := "workspace-with-access-strategy"

			By("creating an access strategy with accessURLTemplate")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating a Workspace referencing AccessStrategy with desiredState=running")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying workspace.status contains correctly templated accessURL")
			accessURL, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessURL}")
			Expect(err).NotTo(HaveOccurred())
			Expect(accessURL).To(Equal(fmt.Sprintf("https://example.com/workspaces/default/%s/", workspaceName)))
		})
	})

	// Context for tests with stopped workspace + access strategy
	Context("Workspace with AccessStrategy and desiredState=stopped", func() {
		It("should not add accessURL nor create access resources when desiredState=stopped", func() {
			accessStrategyFilename := "access-strategy-with-resources"
			workspaceFilename := "workspace-stopped-with-access-strategy"
			workspaceName := "workspace-stopped-with-access-strategy"

			By("creating an access strategy with accessResourceTemplates")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "")

			By("creating a Workspace referencing AccessStrategy with desiredState=stopped")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("waiting for the workspace to become stopped")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeStopped,
				ConditionTrue,
			)

			By("verifying conditions Available=False, Progressing=False, Degraded=False, Stopped=True")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeDegraded:    ConditionFalse,
				controller.ConditionTypeAvailable:   ConditionFalse,
				controller.ConditionTypeStopped:     ConditionTrue,
			})

			By("verifying no access resources have been created")
			Consistently(func() bool {
				exists := ResourceExists("networkpolicy", fmt.Sprintf("workspace-network-policy-%s", workspaceName),
					workspaceNamespace, "{.metadata.name}")
				return !exists
			}, "10s", "5s").Should(BeTrue())

			By("verifying no access resources are tracked in status")
			accessResources, _ := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources}")
			Expect(accessResources).To(BeEmpty())

			By("verifying Workspace status does not contain accessURL")
			accessURL, _ := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessURL}")
			Expect(accessURL).To(BeEmpty())
		})
	})

	Context("Workspace with invalid AccessStrategy reference", func() {
		It("should reject creation of Workspace referencing non-existent AccessStrategy", func() {
			workspaceFilename := "workspace-with-nonexistent-access-strategy"
			workspaceName := "workspace-with-nonexistent-access-strategy"

			By("verifying Workspace referencing non-existent AccessStrategy rejected by webhook")
			VerifyCreateWorkspaceRejectedByWebhook(workspaceFilename, groupDir, "", workspaceName)
		})
	})
})

func deleteResourcesForAccessStrategyTest(workspaceNamespace string) {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", workspaceNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=90s")
	_, _ = utils.Run(cmd)

	By("cleaning up access strategies")
	cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)

	// Wait to ensure all resources are fully deleted
	time.Sleep(2 * time.Second)
}
