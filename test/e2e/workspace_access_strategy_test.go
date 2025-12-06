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
			VerifyCreateWorkspaceRejectedByWebhook(workspaceFilename, groupDir, "", workspaceName, workspaceNamespace)
		})
	})

	Context("Workspace with Access Strategy State Transition", func() {
		const (
			subGroup               = "ws-state"
			accessStrategyFilename = "access-strategy-with-env-and-resources"
		)

		It("should remove access resources when transitioning from running to stopped", func() {
			workspaceFilename := "workspace-running-access-strategy"
			workspaceName := "workspace-running-access-strategy"
			networkPolicyName := fmt.Sprintf("workspace-network-policy-%s", workspaceName)
			primaryRouteName := fmt.Sprintf("primary-route-%s", workspaceName)
			authRouteName := fmt.Sprintf("auth-route-%s", workspaceName)

			By("creating an access strategy with both env vars and resources")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, subGroup)

			By("creating a workspace with desiredState=running referencing AccessStrategy")
			createWorkspaceForTest(workspaceFilename, groupDir, subGroup)

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying workspace.status.accessResources is populated")
			accessResourceList, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources[*].kind}")
			Expect(err).NotTo(HaveOccurred())
			resources := strings.Fields(accessResourceList)
			Expect(resources).To(HaveLen(3), "Expected 3 access resources (NetworkPolicy and 2 IngressRoutes)")

			By("verifying workspace.status.accessURL is set")
			accessURL, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessURL}")
			Expect(err).NotTo(HaveOccurred())
			Expect(accessURL).To(Equal(fmt.Sprintf("https://example.com/workspaces/default/%s/", workspaceName)))

			By("updating workspace to desiredState=stopped")
			UpdateWorkspaceDesiredState(workspaceName, workspaceNamespace, "Stopped")

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

			By("consistently verifying access resources do not exist")
			Consistently(func() bool {
				npExists := ResourceExists("networkpolicy", networkPolicyName, workspaceNamespace, "{.metadata.name}")
				primaryExists := ResourceExists("ingressroute.traefik.io", primaryRouteName, workspaceNamespace, "{.metadata.name}")
				authExists := ResourceExists("ingressroute.traefik.io", authRouteName, workspaceNamespace, "{.metadata.name}")
				return !npExists && !primaryExists && !authExists
			}, "10s", "5s").Should(BeTrue())

			By("verifying workspace.status.accessResources is empty")
			accessResources, _ := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources}")
			Expect(accessResources).To(BeEmpty())

			By("verifying workspace.status.accessURL is empty")
			accessURL, _ = kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessURL}")
			Expect(accessURL).To(BeEmpty())
		})

		It("should create access resources when transitioning from stopped to running", func() {
			workspaceFilename := "workspace-stopped-access-strategy"
			workspaceName := "workspace-stopped-access-strategy"
			networkPolicyName := fmt.Sprintf("workspace-network-policy-%s", workspaceName)
			primaryRouteName := fmt.Sprintf("primary-route-%s", workspaceName)
			authRouteName := fmt.Sprintf("auth-route-%s", workspaceName)

			By("creating an access strategy with both env vars and resources")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, subGroup)

			By("creating a workspace with desiredState=stopped referencing AccessStrategy")
			createWorkspaceForTest(workspaceFilename, groupDir, subGroup)

			By("waiting for the workspace to become stopped")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeStopped,
				ConditionTrue,
			)

			By("verifying no access resources exist when stopped")
			Consistently(func() bool {
				npExists := ResourceExists("networkpolicy", networkPolicyName, workspaceNamespace, "{.metadata.name}")
				primaryExists := ResourceExists("ingressroute.traefik.io", primaryRouteName, workspaceNamespace, "{.metadata.name}")
				authExists := ResourceExists("ingressroute.traefik.io", authRouteName, workspaceNamespace, "{.metadata.name}")
				return !npExists && !primaryExists && !authExists
			}, "10s", "5s").Should(BeTrue())

			By("verifying workspace.status.accessResources is empty")
			accessResources, _ := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources}")
			Expect(accessResources).To(BeEmpty())

			By("verifying workspace.status.accessURL is empty")
			accessURL, _ := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessURL}")
			Expect(accessURL).To(BeEmpty())

			By("updating workspace to desiredState=running")
			UpdateWorkspaceDesiredState(workspaceName, workspaceNamespace, "Running")

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

			By("verifying all access resources have been created")
			WaitForResourceToExist("networkpolicy", networkPolicyName, workspaceNamespace,
				"{.metadata.name}", 5*time.Minute, 5*time.Second)
			WaitForResourceToExist("ingressroute.traefik.io", primaryRouteName, workspaceNamespace,
				"{.metadata.name}", 5*time.Minute, 5*time.Second)
			WaitForResourceToExist("ingressroute.traefik.io", authRouteName, workspaceNamespace,
				"{.metadata.name}", 5*time.Minute, 5*time.Second)

			By("verifying workspace.status.accessResources is populated with all three resources")
			accessResourceList, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources[*].kind}")
			Expect(err).NotTo(HaveOccurred())
			resources := strings.Fields(accessResourceList)
			Expect(resources).To(HaveLen(3), "Expected 3 access resources (NetworkPolicy and 2 IngressRoutes)")

			By("verifying workspace.status.accessURL is set")
			accessURL, err = kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessURL}")
			Expect(err).NotTo(HaveOccurred())
			Expect(accessURL).To(Equal(fmt.Sprintf("https://example.com/workspaces/default/%s/", workspaceName)))
		})
	})

	Context("Deletion Protection", func() {
		const (
			basicAccessStrategyName            = "basic-access-strategy"
			basicAccessStrategyFilename        = "basic-access-strategy"
			workspaceWithAccessStrategy        = "workspace-with-access-strategy"
			workspaceStoppedWithAccessStrategy = "workspace-stopped-basic-access-strategy"
		)

		It("should successfully delete AccessStrategy not referenced by any workspace", func() {
			By("creating an AccessStrategy")
			createAccessStrategyForTest(basicAccessStrategyFilename, groupDir, "")

			By("verifying AccessStrategy exists")
			exists := ResourceExists("workspaceaccessstrategy", basicAccessStrategyName, SharedNamespace, "{.metadata.name}")
			Expect(exists).To(BeTrue(), "AccessStrategy should exist after creation")

			By("deleting AccessStrategy")
			cmd := exec.Command("kubectl", "delete", "workspaceaccessstrategy", basicAccessStrategyName,
				"-n", SharedNamespace, "--wait=true")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying AccessStrategy is deleted within 10s")
			Eventually(func() bool {
				exists := ResourceExists("workspaceaccessstrategy", basicAccessStrategyName, SharedNamespace, "{.metadata.name}")
				return !exists
			}, 10*time.Second, 1*time.Second).Should(BeTrue(), "AccessStrategy should be deleted")
		})

		It("should prevent AccessStrategy deletion while running workspace references it", func() {
			By("creating an AccessStrategy")
			createAccessStrategyForTest(basicAccessStrategyFilename, groupDir, "")

			By("creating a Workspace with desiredState=running referencing AccessStrategy")
			createWorkspaceForTest(workspaceWithAccessStrategy, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceWithAccessStrategy,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			verifyAccessStrategyDeletionProtection(
				basicAccessStrategyName,
				workspaceWithAccessStrategy,
				workspaceNamespace,
			)
		})

		It("should prevent AccessStrategy deletion while stopped workspace references it", func() {
			By("creating an AccessStrategy")
			createAccessStrategyForTest(basicAccessStrategyFilename, groupDir, "")

			By("creating a Workspace with desiredState=stopped referencing AccessStrategy")
			createWorkspaceForTest(workspaceStoppedWithAccessStrategy, groupDir, "")

			By("waiting for the workspace to become stopped")
			WaitForWorkspaceToReachCondition(
				workspaceStoppedWithAccessStrategy,
				workspaceNamespace,
				controller.ConditionTypeStopped,
				ConditionTrue,
			)

			verifyAccessStrategyDeletionProtection(
				basicAccessStrategyName,
				workspaceStoppedWithAccessStrategy,
				workspaceNamespace,
			)
		})
	})

	Context("Change to Access Strategy in Running Workspace", func() {
		const (
			subGroup                    = "ws-reference"
			accessStrategyFilename      = "access-strategy-with-env-and-resources"
			workspaceNoAccessStrategy   = "workspace-running-no-access-strategy"
			workspaceWithAccessStrategy = "workspace-running-access-strategy"
		)

		It("should create access resources when AccessStrategy is added to running workspace", func() {
			workspaceFilename := workspaceNoAccessStrategy
			workspaceName := workspaceNoAccessStrategy
			networkPolicyName := fmt.Sprintf("workspace-network-policy-%s", workspaceName)
			primaryRouteName := fmt.Sprintf("primary-route-%s", workspaceName)
			authRouteName := fmt.Sprintf("auth-route-%s", workspaceName)

			By("creating an access strategy with both env vars and resources")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "ws-state")

			By("creating a workspace with desiredState=running WITHOUT AccessStrategy reference")
			createWorkspaceForTest(workspaceFilename, groupDir, subGroup)

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying workspace.status.accessResources is empty")
			accessResources, _ := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources}")
			Expect(accessResources).To(BeEmpty())

			By("verifying workspace.status.accessURL is empty")
			accessURL, _ := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessURL}")
			Expect(accessURL).To(BeEmpty())

			By("updating Workspace to add AccessStrategy reference")
			patchCmd := `{"spec":{"accessStrategy":` +
				`{"name":"access-strategy-with-env-and-resources","namespace":"jupyter-k8s-shared"}}}`
			cmd := exec.Command("kubectl", "patch", "workspace", workspaceName,
				"-n", workspaceNamespace, "--type=merge", "-p", patchCmd)
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for reconciliation and verifying all access resources exist")
			WaitForResourceToExist("networkpolicy", networkPolicyName, workspaceNamespace,
				"{.metadata.name}", 2*time.Minute, 5*time.Second)
			WaitForResourceToExist("ingressroute.traefik.io", primaryRouteName, workspaceNamespace,
				"{.metadata.name}", 2*time.Minute, 5*time.Second)
			WaitForResourceToExist("ingressroute.traefik.io", authRouteName, workspaceNamespace,
				"{.metadata.name}", 2*time.Minute, 5*time.Second)

			By("verifying workspace.status.accessResources is populated with all three resources")
			accessResourceList, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources[*].kind}")
			Expect(err).NotTo(HaveOccurred())
			resources := strings.Fields(accessResourceList)
			Expect(resources).To(HaveLen(3), "Expected 3 access resources (NetworkPolicy and 2 IngressRoutes)")

			By("verifying workspace.status.accessURL is set")
			accessURL, err = kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessURL}")
			Expect(err).NotTo(HaveOccurred())
			Expect(accessURL).To(Equal(fmt.Sprintf("https://example.com/workspaces/default/%s/", workspaceName)))

			By("retrieving deployment name from workspace status")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying deployment now has injected env var")
			envVarValue, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='JUPYTER_BASE_URL')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(envVarValue).To(Equal(fmt.Sprintf("/workspaces/%s/%s/", workspaceNamespace, workspaceName)))
		})

		It("should delete access resources when AccessStrategy is removed from running workspace", func() {
			workspaceFilename := workspaceWithAccessStrategy
			workspaceName := workspaceWithAccessStrategy
			networkPolicyName := fmt.Sprintf("workspace-network-policy-%s", workspaceName)
			primaryRouteName := fmt.Sprintf("primary-route-%s", workspaceName)
			authRouteName := fmt.Sprintf("auth-route-%s", workspaceName)

			By("creating an access strategy with both env vars and resources")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, "ws-state")

			By("creating a workspace with desiredState=running referencing AccessStrategy")
			createWorkspaceForTest(workspaceFilename, groupDir, "ws-state")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying access resources exist")
			Expect(ResourceExists("networkpolicy", networkPolicyName, workspaceNamespace,
				"{.metadata.name}")).To(BeTrue())
			Expect(ResourceExists("ingressroute.traefik.io", primaryRouteName, workspaceNamespace,
				"{.metadata.name}")).To(BeTrue())
			Expect(ResourceExists("ingressroute.traefik.io", authRouteName, workspaceNamespace,
				"{.metadata.name}")).To(BeTrue())

			By("verifying workspace.status.accessResources is populated")
			accessResourceList, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources[*].kind}")
			Expect(err).NotTo(HaveOccurred())
			resources := strings.Fields(accessResourceList)
			Expect(resources).To(HaveLen(3))

			By("verifying workspace.status.accessURL is set")
			accessURL, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessURL}")
			Expect(err).NotTo(HaveOccurred())
			Expect(accessURL).NotTo(BeEmpty())

			By("retrieving deployment name from workspace status")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying deployment has injected env var")
			envVarValue, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='JUPYTER_BASE_URL')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(envVarValue).NotTo(BeEmpty())

			By("updating Workspace to remove AccessStrategy reference")
			patchCmd := `{"spec":{"accessStrategy":null}}`
			cmd := exec.Command("kubectl", "patch", "workspace", workspaceName,
				"-n", workspaceNamespace, "--type=merge", "-p", patchCmd)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for access resources to be deleted")
			Eventually(func() bool {
				npExists := ResourceExists("networkpolicy", networkPolicyName, workspaceNamespace, "{.metadata.name}")
				primaryExists := ResourceExists("ingressroute.traefik.io", primaryRouteName, workspaceNamespace, "{.metadata.name}")
				authExists := ResourceExists("ingressroute.traefik.io", authRouteName, workspaceNamespace, "{.metadata.name}")
				return !npExists && !primaryExists && !authExists
			}, 2*time.Minute, 5*time.Second).Should(BeTrue())

			By("consistently verifying access resources remain deleted")
			Consistently(func() bool {
				npExists := ResourceExists("networkpolicy", networkPolicyName, workspaceNamespace, "{.metadata.name}")
				primaryExists := ResourceExists("ingressroute.traefik.io", primaryRouteName, workspaceNamespace, "{.metadata.name}")
				authExists := ResourceExists("ingressroute.traefik.io", authRouteName, workspaceNamespace, "{.metadata.name}")
				return !npExists && !primaryExists && !authExists
			}, 10*time.Second, 2*time.Second).Should(BeTrue())

			By("verifying workspace.status.accessResources is empty")
			accessResources, _ := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources}")
			Expect(accessResources).To(BeEmpty())

			By("verifying workspace.status.accessURL is empty")
			accessURL, _ = kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessURL}")
			Expect(accessURL).To(BeEmpty())

			By("verifying deployment no longer has JUPYTER_BASE_URL env var")
			envVarValue, err = kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='JUPYTER_BASE_URL')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(envVarValue).To(BeEmpty())
		})
	})

	Context("AccessStrategy Updates", func() {
		const (
			subGroup               = "updates"
			accessStrategyName     = "access-strategy-single-resource"
			accessStrategyFilename = "access-strategy-single-resource"
			workspaceFilename      = "workspace-running-for-update"
			workspaceName          = "workspace-running-for-update"
		)

		It("should create new access resource when added to AccessStrategy", func() {
			networkPolicyName := fmt.Sprintf("workspace-network-policy-%s", workspaceName)
			primaryRouteName := fmt.Sprintf("primary-route-%s", workspaceName)
			authRouteName := fmt.Sprintf("auth-route-%s", workspaceName)

			By("creating AccessStrategy with 1 resource (NetworkPolicy only)")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, subGroup)

			By("creating Workspace with desiredState=running referencing AccessStrategy")
			createWorkspaceForTest(workspaceFilename, groupDir, subGroup)

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying 1 access resource exists (NetworkPolicy)")
			Expect(ResourceExists("networkpolicy", networkPolicyName, workspaceNamespace,
				"{.metadata.name}")).To(BeTrue())

			By("verifying workspace.status.accessResources shows 1 resource")
			accessResourceList, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources[*].kind}")
			Expect(err).NotTo(HaveOccurred())
			resources := strings.Fields(accessResourceList)
			Expect(resources).To(HaveLen(1), "Expected 1 access resource (NetworkPolicy)")

			By("updating AccessStrategy to add 2 IngressRoute templates")
			patchAccessStrategy(
				groupDir,
				subGroup,
				"access-strategy-three-resources-patch",
				accessStrategyName,
			)

			By("waiting for reconciliation and verifying all 3 access resources exist")
			WaitForResourceToExist("networkpolicy", networkPolicyName, workspaceNamespace,
				"{.metadata.name}", 2*time.Minute, 5*time.Second)
			WaitForResourceToExist("ingressroute.traefik.io", primaryRouteName, workspaceNamespace,
				"{.metadata.name}", 2*time.Minute, 5*time.Second)
			WaitForResourceToExist("ingressroute.traefik.io", authRouteName, workspaceNamespace,
				"{.metadata.name}", 2*time.Minute, 5*time.Second)

			By("verifying all three tracked in workspace.status.accessResources")
			accessResourceList, err = kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources[*].kind}")
			Expect(err).NotTo(HaveOccurred())
			resources = strings.Fields(accessResourceList)
			Expect(resources).To(HaveLen(3), "Expected 3 access resources (NetworkPolicy and 2 IngressRoutes)")
		})

		It("should update access resource when template changes in AccessStrategy", func() {
			accessStrategyName := "access-strategy-two-ingressroutes"
			accessStrategyFilename := "access-strategy-two-ingressroutes"
			workspaceFilename := "workspace-running-for-update-two-routes"
			primaryRouteName := fmt.Sprintf("primary-route-%s", workspaceName)
			authRouteName := fmt.Sprintf("auth-route-%s", workspaceName)

			By("creating AccessStrategy with 2 IngressRoutes (priority: 100 and 200)")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, subGroup)

			By("creating Workspace with desiredState=running referencing AccessStrategy")
			createWorkspaceForTest(workspaceFilename, groupDir, subGroup)

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying both IngressRoute exist")
			Expect(ResourceExists("ingressroute.traefik.io", primaryRouteName, workspaceNamespace,
				"{.metadata.name}")).To(BeTrue())
			Expect(ResourceExists("ingressroute.traefik.io", authRouteName, workspaceNamespace,
				"{.metadata.name}")).To(BeTrue())

			By("verifying IngressRoute priorities are 100 and 200 respectively")
			primaryPriority, err := kubectlGet("ingressroute.traefik.io", primaryRouteName,
				workspaceNamespace, "{.spec.routes[0].priority}")
			Expect(err).NotTo(HaveOccurred())
			Expect(primaryPriority).To(Equal("100"))

			authPriority, err := kubectlGet("ingressroute.traefik.io", authRouteName,
				workspaceNamespace, "{.spec.routes[0].priority}")
			Expect(err).NotTo(HaveOccurred())
			Expect(authPriority).To(Equal("200"))

			By("updating AccessStrategy IngressRoute templates (change priority to 110 and 190)")
			patchAccessStrategy(
				groupDir,
				subGroup,
				"access-strategy-update-priorities-patch",
				accessStrategyName,
			)

			By("waiting for reconciliation and verifying priorities are updated")
			Eventually(func() bool {
				primaryPriority, err := kubectlGet("ingressroute.traefik.io", primaryRouteName,
					workspaceNamespace, "{.spec.routes[0].priority}")
				if err != nil {
					return false
				}
				authPriority, err := kubectlGet("ingressroute.traefik.io", authRouteName,
					workspaceNamespace, "{.spec.routes[0].priority}")
				if err != nil {
					return false
				}
				return primaryPriority == "110" && authPriority == "190"
			}, 2*time.Minute, 5*time.Second).Should(BeTrue())
		})

		It("should delete access resource when removed from AccessStrategy", func() {
			accessStrategyName := "access-strategy-three-resources"
			accessStrategyFilename := "access-strategy-three-resources"
			workspaceFilename := "workspace-running-for-update-three-resources"
			networkPolicyName := fmt.Sprintf("workspace-network-policy-%s", workspaceName)
			primaryRouteName := fmt.Sprintf("primary-route-%s", workspaceName)
			authRouteName := fmt.Sprintf("auth-route-%s", workspaceName)

			By("creating AccessStrategy with 3 resources (NetworkPolicy + 2 IngressRoutes)")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, subGroup)

			By("creating Workspace with desiredState=running referencing AccessStrategy")
			createWorkspaceForTest(workspaceFilename, groupDir, subGroup)

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying 3 access resources exist")
			Expect(ResourceExists("networkpolicy", networkPolicyName, workspaceNamespace,
				"{.metadata.name}")).To(BeTrue())
			Expect(ResourceExists("ingressroute.traefik.io", primaryRouteName, workspaceNamespace,
				"{.metadata.name}")).To(BeTrue())
			Expect(ResourceExists("ingressroute.traefik.io", authRouteName, workspaceNamespace,
				"{.metadata.name}")).To(BeTrue())

			By("verifying workspace.status.accessResources shows 3 resources")
			accessResourceList, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources[*].kind}")
			Expect(err).NotTo(HaveOccurred())
			resources := strings.Fields(accessResourceList)
			Expect(resources).To(HaveLen(3), "Expected 3 access resources")

			By("updating AccessStrategy to remove NetworkPolicy and auth IngressRoute (keep only primary)")
			patchAccessStrategy(
				groupDir,
				subGroup,
				"access-strategy-remove-resources-patch",
				accessStrategyName,
			)

			By("waiting for reconciliation and verifying only primary IngressRoute exists")
			Eventually(func() bool {
				primaryExists := ResourceExists("ingressroute.traefik.io", primaryRouteName,
					workspaceNamespace, "{.metadata.name}")
				return primaryExists
			}, 2*time.Minute, 5*time.Second).Should(BeTrue())

			By("consistently verifying NetworkPolicy and auth IngressRoute are deleted")
			Consistently(func() bool {
				npExists := ResourceExists("networkpolicy", networkPolicyName, workspaceNamespace,
					"{.metadata.name}")
				authExists := ResourceExists("ingressroute.traefik.io", authRouteName, workspaceNamespace,
					"{.metadata.name}")
				return !npExists && !authExists
			}, 10*time.Second, 2*time.Second).Should(BeTrue())

			By("verifying workspace.status.accessResources shows only 1 resource")
			accessResourceList, err = kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessResources[*].kind}")
			Expect(err).NotTo(HaveOccurred())
			resources = strings.Fields(accessResourceList)
			Expect(resources).To(HaveLen(1), "Expected 1 access resource (primary IngressRoute)")
		})

		It("should update workspace status and deployment when AccessStrategy templates change", func() {
			accessStrategyName := "access-strategy-with-templates"
			accessStrategyFilename := "access-strategy-with-templates"
			workspaceFilename := "workspace-running-for-update-with-templates"

			By("creating AccessStrategy with old URL and env var templates")
			createAccessStrategyForTest(accessStrategyFilename, groupDir, subGroup)

			By("creating Workspace with desiredState=running referencing AccessStrategy")
			createWorkspaceForTest(workspaceFilename, groupDir, subGroup)

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying workspace.status.accessURL is 'https://old.example.com/<workspace-name>/'")
			accessURL, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.accessURL}")
			Expect(err).NotTo(HaveOccurred())
			Expect(accessURL).To(Equal(fmt.Sprintf("https://old.example.com/%s/", workspaceName)))

			By("retrieving deployment name from workspace status")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying deployment env var JUPYTER_BASE_URL is '/old/<workspace-name>/'")
			envVarValue, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='JUPYTER_BASE_URL')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(envVarValue).To(Equal(fmt.Sprintf("/old/%s/", workspaceName)))

			By("updating AccessStrategy to change URL and env var templates to new values")
			patchAccessStrategy(
				groupDir,
				subGroup,
				"access-strategy-update-templates-patch",
				accessStrategyName,
			)

			By("waiting for reconciliation and verifying workspace.status.accessURL is updated")
			Eventually(func() string {
				accessURL, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
					"{.status.accessURL}")
				if err != nil {
					return ""
				}
				return accessURL
			}, 2*time.Minute, 5*time.Second).Should(Equal(fmt.Sprintf("https://new.example.com/%s/", workspaceName)))

			By("verifying deployment env var JUPYTER_BASE_URL is now '/new/<workspace-name>/'")
			Eventually(func() string {
				envVarValue, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
					"{.spec.template.spec.containers[0].env[?(@.name=='JUPYTER_BASE_URL')].value}")
				if err != nil {
					return ""
				}
				return envVarValue
			}, 2*time.Minute, 5*time.Second).Should(Equal(fmt.Sprintf("/new/%s/", workspaceName)))
		})
	})
})

func deleteResourcesForAccessStrategyTest(workspaceNamespace string) {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", workspaceNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up access strategies")
	cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy", "--all", "-n", SharedNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)

	By("waiting an arbitrary fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
