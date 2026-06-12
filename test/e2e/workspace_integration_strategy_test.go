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

var _ = Describe("Workspace Integration Strategy", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "integration-strategy"
	)

	// Install a minimal RayCluster CRD + instance so resourceLookup has a target.
	BeforeAll(func() {
		By("installing the minimal RayCluster CRD for resourceLookup tests")
		applyIntegrationFixture("raycluster-crd")

		By("waiting for the RayCluster CRD to be established")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "crd", "rayclusters.ray.io")
			_, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
		}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
	})

	AfterAll(func() {
		By("removing the RayCluster CRD")
		cmd := exec.Command("kubectl", "delete", "crd", "rayclusters.ray.io", "--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)
	})

	AfterEach(func() {
		deleteResourcesForIntegrationStrategyTest(workspaceNamespace)
	})

	Context("Workspace with IntegrationStrategy and a resolvable resource", func() {
		It("should inject a resolved sidecar and env into the deployment", func() {
			workspaceName := "workspace-with-integration-strategy"

			By("creating the RayCluster instance to look up")
			applyIntegrationFixture("raycluster")

			By("creating the integration strategy")
			applyIntegrationFixture("ray-connector-strategy")

			By("creating a workspace referencing the integration strategy")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("retrieving deployment name from workspace status")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying the ray-sidecar container was added with the resolved image")
			sidecarImage, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[?(@.name=='ray-sidecar')].image}")
			Expect(err).NotTo(HaveOccurred())
			Expect(sidecarImage).To(Equal("rayproject/ray:2.9.0"))

			By("verifying the sidecar args were resolved from the looked-up resource")
			sidecarArgs, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[?(@.name=='ray-sidecar')].args[0]}")
			Expect(err).NotTo(HaveOccurred())
			Expect(sidecarArgs).To(ContainSubstring("demo-cluster-head-svc:6379"))

			By("verifying primary container env was merged with resolved values")
			headSvc, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='RAY_HEAD_SERVICE')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(headSvc).To(Equal("demo-cluster-head-svc"))

			clusterNameEnv, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='RAY_CLUSTER_NAME')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(clusterNameEnv).To(Equal("demo-cluster"))
		})
	})

	Context("Workspace with IntegrationStrategy referencing a missing resource", func() {
		It("should mark the workspace Degraded when the looked-up resource does not exist", func() {
			workspaceName := "workspace-missing-resource"

			By("creating the integration strategy (no matching RayCluster will exist)")
			applyIntegrationFixture("ray-connector-strategy")

			By("creating a workspace whose parameter points to a nonexistent RayCluster")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become Degraded")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeDegraded,
				ConditionTrue,
			)
		})
	})

	Context("IntegrationStrategy update propagation", func() {
		It("should re-reconcile referencing workspaces when the strategy changes", func() {
			workspaceName := "workspace-with-integration-strategy"

			By("creating the RayCluster instance to look up")
			applyIntegrationFixture("raycluster")

			By("creating the integration strategy")
			applyIntegrationFixture("ray-connector-strategy")

			By("creating a workspace referencing the integration strategy")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("patching the strategy to add a new env var")
			patchCmd := `{"spec":{"deploymentModifications":{"podModifications":` +
				`{"primaryContainerModifications":{"mergeEnv":[` +
				`{"name":"RAY_HEAD_SERVICE","valueTemplate":"{{ resource \"{.status.head.serviceName}\" }}"},` +
				`{"name":"RAY_CLUSTER_NAME","valueTemplate":"{{ .Parameters.rayClusterName }}"},` +
				`{"name":"RAY_EXTRA","valueTemplate":"injected"}]}}}}}`
			cmd := exec.Command("kubectl", "patch", "workspaceintegrationstrategy", "ray-connector",
				"-n", workspaceNamespace, "--type=merge", "-p", patchCmd)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the workspace deployment picks up the new env var")
			Eventually(func(g Gomega) {
				val, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
					"{.spec.template.spec.containers[0].env[?(@.name=='RAY_EXTRA')].value}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(val).To(Equal("injected"))
			}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		})
	})
})

// applyIntegrationFixture applies a fixture YAML from the integration-strategy static dir.
func applyIntegrationFixture(filename string) {
	GinkgoHelper()
	path := BuildTestResourcePath(filename, "integration-strategy", "")
	By(fmt.Sprintf("applying fixture %s", path))
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

func deleteResourcesForIntegrationStrategyTest(workspaceNamespace string) {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", workspaceNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up integration strategies")
	cmd = exec.Command("kubectl", "delete", "workspaceintegrationstrategy", "--all", "-n", workspaceNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)

	By("cleaning up RayCluster instances")
	cmd = exec.Command("kubectl", "delete", "raycluster", "--all", "-n", workspaceNamespace,
		"--ignore-not-found", "--wait=false")
	_, _ = utils.Run(cmd)

	By("waiting an arbitrary fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
