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

// Workspace Integration (WorkspaceIntegration child object).
//
// These specs exercise the full path: the workspace controller creates one WorkspaceIntegration
// child per integrationRefs entry (ownerRef'd to the workspace), the mutating webhook resolves the
// child's spec output (deploymentModifications/statusProbe/shareProcessNamespace) at its admission,
// the deployment is built from the frozen child, and the operator probes the frozen statusProbe
// into workspace.status.integrations[].
var _ = Describe("Workspace Integration", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "integration"
		workspaceName      = "workspace-with-integration"
		// childName must match controller.GenerateWorkspaceIntegrationName(workspaceName, ref).
		childName = "workspace-workspace-with-integration-ray-integration"
	)

	// Install a minimal RayCluster CRD + instances so resourceRefs has a target.
	BeforeAll(func() {
		By("installing the minimal RayCluster CRD for resourceRefs tests")
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
		deleteResourcesForIntegrationTest(workspaceNamespace)
	})

	Context("Workspace with a resolvable integration", func() {
		It("creates a baked WorkspaceIntegration child and injects it into the deployment", func() {
			By("creating the RayCluster instance to look up")
			applyIntegrationFixture("raycluster")

			By("creating the integration template")
			applyIntegrationFixture("ray-integration")

			By("creating a workspace referencing the integration template")
			createWorkspaceForTest("workspace-with-integration", groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			By("verifying a WorkspaceIntegration child was created and ownerRef'd to the workspace")
			ownerName, err := kubectlGet("workspaceintegration", childName, workspaceNamespace,
				"{.metadata.ownerReferences[0].name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(ownerName).To(Equal(workspaceName), "child must be ownerRef'd to the workspace for GC")

			ownerKind, err := kubectlGet("workspaceintegration", childName, workspaceNamespace,
				"{.metadata.ownerReferences[0].kind}")
			Expect(err).NotTo(HaveOccurred())
			Expect(ownerKind).To(Equal("Workspace"))

			By("verifying the child carries the identification label")
			label, err := kubectlGet("workspaceintegration", childName, workspaceNamespace,
				"{.metadata.labels.workspace\\.jupyter\\.org/workspace-name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(label).To(Equal(workspaceName))

			By("verifying the spec output was resolved by the webhook with literal values")
			resolvedEnv, err := kubectlGet("workspaceintegration", childName, workspaceNamespace,
				"{.spec.deploymentModifications.podModifications.primaryContainerModifications.mergeEnv[?(@.name=='RAY_HEAD_SERVICE')].valueTemplate}")
			Expect(err).NotTo(HaveOccurred())
			Expect(resolvedEnv).To(Equal("demo-cluster-head-svc"),
				"the webhook must have resolved the RayCluster head service into the frozen child")

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

			By("verifying the template's shareProcessNamespace was frozen onto the child and applied to the pod")
			childShareProcNs, err := kubectlGet("workspaceintegration", childName, workspaceNamespace,
				"{.spec.shareProcessNamespace}")
			Expect(err).NotTo(HaveOccurred())
			Expect(childShareProcNs).To(Equal("true"),
				"the webhook must freeze shareProcessNamespace onto the child's spec")
			podShareProcNs, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.shareProcessNamespace}")
			Expect(err).NotTo(HaveOccurred())
			Expect(podShareProcNs).To(Equal("true"),
				"the deployment builder must OR-reduce shareProcessNamespace onto the pod")

			By("verifying the WorkspaceIntegration child carries NO status subresource (controller-less)")
			// The WI has no status subresource (it mirrors the controller-less WorkspaceIntegrationTemplate);
			// nothing should ever populate .status. A jsonpath for .status yields empty.
			childStatus, err := kubectlGet("workspaceintegration", childName, workspaceNamespace, "{.status}")
			Expect(err).NotTo(HaveOccurred())
			Expect(childStatus).To(BeEmpty(), "WorkspaceIntegration must not carry a populated status")

			By("verifying the ray-sidecar has NO readinessProbe (Ray health must not gate pod/workspace readiness)")
			// Integration health is reported by the report-only statusProbe (status.integrations[]),
			// NOT by gating pod readiness. A container readinessProbe on the sidecar would drop the
			// Workspace to Available=False on a RayCluster outage even though JupyterLab is fine, so
			// the sidecar must carry none. Lock that in here so it cannot regress.
			sidecarReadiness, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[?(@.name=='ray-sidecar')].readinessProbe}")
			Expect(err).NotTo(HaveOccurred())
			Expect(sidecarReadiness).To(BeEmpty(), "ray-sidecar must not gate readiness on Ray connectivity")

			By("verifying the integration status probe reports ready in workspace.status.integrations[]")
			Eventually(func(g Gomega) {
				ready, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
					"{.status.integrations[?(@.name=='ray-integration')].ready}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(ready).To(Equal("true"))
			}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		})
	})

	Context("Switching the workspace to a different RayCluster", func() {
		It("re-bakes the child and updates the deployment when the workspace is re-applied", func() {
			By("creating both RayCluster instances")
			applyIntegrationFixture("raycluster")
			applyIntegrationFixture("raycluster-2")

			By("creating the integration template")
			applyIntegrationFixture("ray-integration")

			By("creating the workspace pointing at demo-cluster")
			createWorkspaceForTest("workspace-with-integration", groupDir, "")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())

			By("re-applying the workspace pointing at other-cluster")
			createWorkspaceForTest("workspace-switch-cluster", groupDir, "")

			By("verifying the child re-resolved to the new cluster's frozen values")
			Eventually(func(g Gomega) {
				headSvc, err := kubectlGet("workspaceintegration", childName, workspaceNamespace,
					"{.spec.deploymentModifications.podModifications.primaryContainerModifications.mergeEnv[?(@.name=='RAY_HEAD_SERVICE')].valueTemplate}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(headSvc).To(Equal("other-cluster-head-svc"))
			}).WithTimeout(time.Minute).WithPolling(3 * time.Second).Should(Succeed())

			By("verifying the deployment picks up the re-baked env")
			Eventually(func(g Gomega) {
				val, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
					"{.spec.template.spec.containers[0].env[?(@.name=='RAY_HEAD_SERVICE')].value}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(val).To(Equal("other-cluster-head-svc"))
			}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		})
	})

	Context("Workspace referencing a missing resource", func() {
		It("is rejected at the child's admission and the workspace goes Degraded", func() {
			By("creating the integration template (no matching RayCluster will exist)")
			applyIntegrationFixture("ray-integration")

			By("creating a workspace whose parameter points to a nonexistent RayCluster")
			createWorkspaceForTest("workspace-missing-resource", groupDir, "")

			By("waiting for the workspace to become Degraded (the WI child bake fails closed)")
			WaitForWorkspaceToReachCondition(
				"workspace-missing-resource", workspaceNamespace, controller.ConditionTypeDegraded, ConditionTrue)

			By("verifying no WorkspaceIntegration child was persisted (failurePolicy=Fail rejects the create outright)")
			missingChild := controller.GenerateWorkspaceIntegrationName("workspace-missing-resource", "ray-integration")
			WaitForResourceToNotExist("workspaceintegration", missingChild, workspaceNamespace,
				time.Minute, 3*time.Second)

			By("verifying status.integrations[] carries no ready entry for the unresolvable integration")
			// The child never resolved, so the probe path must not report it ready (and must not panic
			// or surface stale data): the entry is either absent or a not-ready Resolving entry.
			ready, err := kubectlGet("workspace", "workspace-missing-resource", workspaceNamespace,
				"{.status.integrations[?(@.name=='ray-integration')].ready}")
			Expect(err).NotTo(HaveOccurred())
			Expect(ready).NotTo(Equal("true"),
				"an integration whose child failed admission must never report ready")
		})
	})

	Context("Garbage collection", func() {
		It("deletes the WorkspaceIntegration child when the workspace is deleted", func() {
			By("creating the RayCluster instance and template")
			applyIntegrationFixture("raycluster")
			applyIntegrationFixture("ray-integration")

			By("creating the workspace")
			createWorkspaceForTest("workspace-with-integration", groupDir, "")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			By("confirming the child exists")
			WaitForResourceToExist("workspaceintegration", childName, workspaceNamespace,
				"{.metadata.name}", time.Minute, 3*time.Second)

			By("deleting the workspace")
			cmd := exec.Command("kubectl", "delete", "workspace", workspaceName,
				"-n", workspaceNamespace, "--wait=true", "--timeout=120s")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the child is garbage-collected via its ownerReference")
			WaitForResourceToNotExist("workspaceintegration", childName, workspaceNamespace,
				2*time.Minute, 5*time.Second)
		})
	})
})

// applyIntegrationFixture applies a fixture YAML from the integration static dir.
func applyIntegrationFixture(filename string) {
	GinkgoHelper()
	path := BuildTestResourcePath(filename, "integration", "")
	By(fmt.Sprintf("applying fixture %s", path))
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

func deleteResourcesForIntegrationTest(workspaceNamespace string) {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", workspaceNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up any leftover WorkspaceIntegration children")
	cmd = exec.Command("kubectl", "delete", "workspaceintegration", "--all", "-n", workspaceNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)

	By("cleaning up integration templates")
	cmd = exec.Command("kubectl", "delete", "workspaceintegrationtemplate", "--all", "-n", workspaceNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)

	By("cleaning up RayCluster instances")
	cmd = exec.Command("kubectl", "delete", "raycluster", "--all", "-n", workspaceNamespace,
		"--ignore-not-found", "--wait=false")
	_, _ = utils.Run(cmd)

	By("waiting a fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
