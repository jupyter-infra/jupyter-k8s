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

// Workspace Integration (Approach 2b: values-in-status freeze).
//
// These specs exercise the freeze path end to end against a real cluster: the operator resolves the
// referenced resource ONCE (when the integration's parametersHash or observedIntegrationTemplateVersion
// changes), freezes the resolved substitution values into workspace.status.resolvedIntegrations, and
// replays those frozen values on every subsequent reconcile WITHOUT re-reading the resource. The
// deployment gets the resolved sidecar overlay; the report-only statusProbe surfaces integration health
// in workspace.status.integrationStatuses[]. There is NO WorkspaceIntegration child object in 2b.
//
// The referenced resource is a built-in Service (a "shared-cache" the workspace connects to). Using a
// built-in kind keeps the suite CRD-free, and the operator already has get on Services, so no extra
// RBAC is needed -- the test targets the generic resolve-any-resource mechanism, not a specific CRD.
var _ = Describe("Workspace Integration", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "integration"
		workspaceName      = "workspace-with-integration"
	)

	AfterEach(func() {
		deleteResourcesForIntegrationTest(workspaceNamespace)
	})

	Context("Workspace with a resolvable integration", func() {
		It("freezes resolved values in status and injects the resolved sidecar into the deployment", func() {
			By("creating the Service to look up")
			applyIntegrationFixture("service-cache")

			By("creating the integration template")
			applyIntegrationFixture("service-integration")

			By("creating a workspace referencing the integration template")
			createWorkspaceForTest("workspace-with-integration", groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			By("verifying the resolved values were FROZEN into status.resolvedIntegrations")
			frozenName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.resolvedIntegrations[?(@.name=='service-integration')].name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(frozenName).To(Equal("service-integration"),
				"the operator must record a frozen resolvedIntegrations entry for the template")

			By("verifying the frozen record carries a parametersHash (hash of templateRef+parameters)")
			frozenToken, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.resolvedIntegrations[?(@.name=='service-integration')].parametersHash}")
			Expect(err).NotTo(HaveOccurred())
			Expect(frozenToken).NotTo(BeEmpty(), "a frozen integration must carry a parametersHash")

			By("retrieving deployment name from workspace status")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying the cache-proxy sidecar container was injected")
			sidecarImage, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[?(@.name=='cache-proxy')].image}")
			Expect(err).NotTo(HaveOccurred())
			Expect(sidecarImage).To(Equal("busybox:1.36"),
				"the sidecar must be injected with its pinned image")

			By("verifying the sidecar args were resolved from the looked-up Service (name:port)")
			sidecarArgs, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[?(@.name=='cache-proxy')].args[0]}")
			Expect(err).NotTo(HaveOccurred())
			Expect(sidecarArgs).To(ContainSubstring("shared-cache:6379"))

			By("verifying primary container env was merged with resolved values")
			cacheHost, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='CACHE_HOST')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cacheHost).To(Equal("shared-cache"))

			cachePort, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='CACHE_PORT')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cachePort).To(Equal("6379"), "the port must be resolved from the Service's spec.ports")

			cacheNameEnv, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='CACHE_SERVICE_NAME')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(cacheNameEnv).To(Equal("shared-cache"))

			By("verifying the template's shareProcessNamespace was OR-reduced onto the pod")
			podShareProcNs, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.shareProcessNamespace}")
			Expect(err).NotTo(HaveOccurred())
			Expect(podShareProcNs).To(Equal("true"),
				"the deployment builder must OR-reduce shareProcessNamespace onto the pod")

			By("verifying the sidecar has NO readinessProbe (integration health must not gate pod/workspace readiness)")
			// Integration health is reported by the report-only statusProbe (status.integrationStatuses[]),
			// NOT by gating pod readiness. A container readinessProbe on the sidecar would drop the
			// Workspace to Available=False on a backing-service outage even though JupyterLab is fine, so
			// the sidecar must carry none. Lock that in here so it cannot regress.
			sidecarReadiness, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[?(@.name=='cache-proxy')].readinessProbe}")
			Expect(err).NotTo(HaveOccurred())
			Expect(sidecarReadiness).To(BeEmpty(), "the sidecar must not gate readiness on backing-service connectivity")

			By("verifying the integration status probe reports ready in workspace.status.integrationStatuses[]")
			Eventually(func(g Gomega) {
				state, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
					"{.status.integrationStatuses[?(@.name=='service-integration')].state}")
				g.Expect(err).NotTo(HaveOccurred())
				// "Ready" == controller.IntegrationStateReady (asserted as a literal to avoid importing
				// the controller package for a constant, matching this suite's convention).
				g.Expect(state).To(Equal("Ready"))
			}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		})
	})

	Context("Resolvable integration whose statusProbe fails", func() {
		It("reports Degraded on the integration status but keeps the workspace Available (report-only)", func() {
			By("creating the Service and the failing-probe template")
			applyIntegrationFixture("service-cache")
			applyIntegrationFixture("service-integration-failprobe")

			By("creating a workspace that references the failing-probe template")
			createWorkspaceForTest("workspace-failprobe", groupDir, "")

			By("waiting for the workspace to become Available (a failing probe must NOT gate availability)")
			WaitForWorkspaceToReachCondition(
				"workspace-failprobe", workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			By("verifying the integration still RESOLVED (sidecar injected) despite the failing probe")
			deploymentName, err := kubectlGet("workspace", "workspace-failprobe", workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			sidecarName, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[?(@.name=='cache-proxy')].name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(sidecarName).To(Equal("cache-proxy"), "the sidecar must be injected; only the probe fails")

			By("verifying the integration status flips to Degraded with reason ProbeFailed")
			Eventually(func(g Gomega) {
				state, err := kubectlGet("workspace", "workspace-failprobe", workspaceNamespace,
					"{.status.integrationStatuses[?(@.name=='service-integration-failprobe')].state}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(state).To(Equal("Degraded"), "a failing statusProbe must report Degraded")

				reason, err := kubectlGet("workspace", "workspace-failprobe", workspaceNamespace,
					"{.status.integrationStatuses[?(@.name=='service-integration-failprobe')].conditions[?(@.type=='Ready')].reason}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(reason).To(Equal("ProbeFailed"), "the Ready condition reason must be ProbeFailed")
			}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

			By("verifying the workspace REMAINS Available while the integration is Degraded (report-only contract)")
			// The whole point of a report-only probe: integration health is surfaced on
			// status.integrationStatuses[] but never pulls the workspace's Available condition to False.
			Consistently(func(g Gomega) {
				avail, err := kubectlGet("workspace", "workspace-failprobe", workspaceNamespace,
					"{.status.conditions[?(@.type=='Available')].status}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(avail).To(Equal("True"), "a Degraded integration must not drop workspace availability")
			}, 20*time.Second, 5*time.Second).Should(Succeed())
		})
	})

	Context("Backing-service drift under a stable integration input", func() {
		It("replays the FROZEN values and does not roll the pod when the Service drifts", func() {
			By("creating the Service and template")
			applyIntegrationFixture("service-cache")
			applyIntegrationFixture("service-integration")

			By("creating the workspace and waiting for it to become available")
			createWorkspaceForTest("workspace-with-integration", groupDir, "")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())

			By("capturing the pre-drift deployment generation")
			// metadata.generation is the roll signal: it increments only when the operator patches the
			// pod template. (pod-template-hash is NOT usable here -- it lives on the ReplicaSet and pods,
			// never on deployment.spec.template.metadata.labels, so it would read empty on both sides.)
			preGen, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.metadata.generation}")
			Expect(err).NotTo(HaveOccurred())

			By("drifting the Service's port underneath the workspace")
			// Same Service (shared-cache), new port. The integration's parametersHash is unchanged, so 2b
			// must replay the frozen port 6379 and never read this value.
			applyIntegrationFixture("service-cache-drifted")

			By("verifying the frozen value is REPLAYED and the deployment stays byte-stable over time")
			// The env must remain the pre-drift frozen value, and the deployment generation must not
			// change -- a bump would mean the operator re-rendered the pod template and rolled the pod.
			Consistently(func(g Gomega) {
				cachePort, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
					"{.spec.template.spec.containers[0].env[?(@.name=='CACHE_PORT')].value}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(cachePort).To(Equal("6379"),
					"drift must be ignored: the frozen port must be replayed")

				postGen, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
					"{.metadata.generation}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(postGen).To(Equal(preGen), "deployment generation must not change (no roll)")
			}, 30*time.Second, 5*time.Second).Should(Succeed())
		})
	})

	Context("Switching the workspace to a different Service", func() {
		It("re-resolves and updates the deployment when the integration input token changes", func() {
			By("creating both Services")
			applyIntegrationFixture("service-cache")
			applyIntegrationFixture("service-cache-2")

			By("creating the integration template")
			applyIntegrationFixture("service-integration")

			By("creating the workspace pointing at shared-cache")
			createWorkspaceForTest("workspace-with-integration", groupDir, "")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())

			By("capturing the original frozen input token")
			tokenA, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.resolvedIntegrations[?(@.name=='service-integration')].parametersHash}")
			Expect(err).NotTo(HaveOccurred())
			Expect(tokenA).NotTo(BeEmpty())

			By("re-applying the workspace pointing at other-cache")
			// workspace-switch-cluster.yaml carries the SAME metadata.name (workspace-with-integration)
			// with a different serviceName, so this apply UPDATES the existing workspace in place --
			// it is a controlled parameter change, not a new workspace.
			createWorkspaceForTest("workspace-switch-cluster", groupDir, "")

			By("verifying the frozen input token changed (a controlled re-resolve, not drift)")
			Eventually(func(g Gomega) {
				tokenB, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
					"{.status.resolvedIntegrations[?(@.name=='service-integration')].parametersHash}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(tokenB).NotTo(BeEmpty())
				g.Expect(tokenB).NotTo(Equal(tokenA), "changing parameters must flip the parametersHash")
			}).WithTimeout(time.Minute).WithPolling(3 * time.Second).Should(Succeed())

			By("verifying the deployment re-resolved to the new Service's port")
			Eventually(func(g Gomega) {
				val, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
					"{.spec.template.spec.containers[0].env[?(@.name=='CACHE_PORT')].value}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(val).To(Equal("6380"))
			}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

			By("verifying exactly one cache-proxy exists after the switch (no duplicate overlay)")
			sidecarNames, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[?(@.name=='cache-proxy')].name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(sidecarNames).To(Equal("cache-proxy"),
				"a switch must replace, not append, the sidecar overlay")
		})
	})

	Context("Workspace referencing a missing resource", func() {
		It("is fail-closed and non-fatal: the workspace becomes Available with a base-only pod", func() {
			By("creating the integration template (no matching Service will exist)")
			applyIntegrationFixture("service-integration")

			By("creating a workspace whose parameter points to a nonexistent Service")
			createWorkspaceForTest("workspace-missing-resource", groupDir, "")

			By("waiting for the workspace to become Available despite the unresolvable integration")
			// 2b first-attach failure is non-fatal: no frozen values exist yet, so the operator deploys
			// the pod base-only and the base reconcile still succeeds. The workspace must NOT go Degraded
			// (that was the old admission-child design).
			WaitForWorkspaceToReachCondition(
				"workspace-missing-resource", workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			deploymentName, err := kubectlGet("workspace", "workspace-missing-resource", workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying NO cache-proxy was injected (capture failed, so no overlay is applied)")
			verifyNoSidecarInDeployment(deploymentName, workspaceNamespace)

			By("verifying no frozen values were recorded for the unresolvable integration")
			// Capture never succeeded, so the freeze must not advance to a partial/empty frozen record.
			frozenToken, err := kubectlGet("workspace", "workspace-missing-resource", workspaceNamespace,
				"{.status.resolvedIntegrations[?(@.name=='service-integration')].parametersHash}")
			Expect(err).NotTo(HaveOccurred())
			Expect(frozenToken).To(BeEmpty(),
				"an integration whose first-attach capture failed must not record frozen values")

			By("verifying the unresolved integration surfaces a Degraded status (not logs-only)")
			// A first-attach capture failure must be visible on the Workspace status so an admin can see
			// it without reading operator logs. "Degraded" == controller.IntegrationStateDegraded.
			Eventually(func(g Gomega) {
				state, err := kubectlGet("workspace", "workspace-missing-resource", workspaceNamespace,
					"{.status.integrationStatuses[?(@.name=='service-integration')].state}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(state).To(Equal("Degraded"),
					"an unresolved integration must surface a Degraded status entry")
			}).WithTimeout(time.Minute).WithPolling(3 * time.Second).Should(Succeed())
		})
	})

	Context("Garbage collection", func() {
		It("deletes the workspace-owned deployment when the workspace is deleted", func() {
			By("creating the Service and template")
			applyIntegrationFixture("service-cache")
			applyIntegrationFixture("service-integration")

			By("creating the workspace")
			createWorkspaceForTest("workspace-with-integration", groupDir, "")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())

			By("confirming the deployment exists")
			WaitForResourceToExist("deployment", deploymentName, workspaceNamespace,
				"{.metadata.name}", time.Minute, 3*time.Second)

			By("deleting the workspace")
			cmd := exec.Command("kubectl", "delete", "workspace", workspaceName,
				"-n", workspaceNamespace, "--wait=true", "--timeout=120s")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the deployment is garbage-collected via its ownerReference")
			WaitForResourceToNotExist("deployment", deploymentName, workspaceNamespace,
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

// verifyNoSidecarInDeployment asserts (and keeps asserting) that no cache-proxy container exists on
// the deployment's pod template -- used for the fail-closed base-only path.
func verifyNoSidecarInDeployment(deploymentName, namespace string) {
	GinkgoHelper()
	Consistently(func(g Gomega) {
		names, err := kubectlGet("deployment", deploymentName, namespace,
			"{.spec.template.spec.containers[?(@.name=='cache-proxy')].name}")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(names).To(BeEmpty(), "no cache-proxy overlay must be applied when capture failed")
	}, 15*time.Second, 5*time.Second).Should(Succeed())
}

// deleteResourcesForIntegrationTest removes only the objects this Ordered suite creates, by explicit
// name, so it can never nuke unrelated objects that happen to share the "default" namespace. The names
// track the fixtures applied above (workspace-*, service-integration*, shared-cache/other-cache). This
// suite is Ordered (serial), so a fixed, known name set is sufficient -- there are no other concurrent
// specs creating integration objects in this namespace.
func deleteResourcesForIntegrationTest(workspaceNamespace string) {
	GinkgoHelper()
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace",
		"workspace-with-integration", "workspace-failprobe", "workspace-missing-resource",
		"-n", workspaceNamespace, "--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up integration templates")
	cmd = exec.Command("kubectl", "delete", "workspaceintegrationtemplate",
		"service-integration", "service-integration-failprobe",
		"-n", workspaceNamespace, "--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)

	// Wait for the Services to be fully gone (--wait=true) instead of sleeping a fixed interval: the
	// next spec re-applies these same names, and applying while the prior object is still terminating
	// would conflict. --wait=true is the same deletion-sync idiom used by the deletes above.
	By("cleaning up Services")
	cmd = exec.Command("kubectl", "delete", "service",
		"shared-cache", "other-cache",
		"-n", workspaceNamespace, "--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)
}
