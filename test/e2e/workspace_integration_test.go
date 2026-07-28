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
	appsv1 "k8s.io/api/apps/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// Workspace Integration: values-in-status freeze.
//
// These specs exercise the freeze path end to end against a real cluster: the operator resolves the
// referenced resource ONCE (when the integration's parametersHash or observedIntegrationTemplateVersion
// changes), freezes the resolved substitution values into workspace.status.resolvedIntegrations, and
// replays those frozen values on every subsequent reconcile WITHOUT re-reading the resource. The
// deployment gets the resolved sidecar overlay; the report-only statusProbe surfaces integration health
// in workspace.status.integrationStatuses[].
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

			By("fetching the workspace once and validating the frozen status against it")
			// Get the whole Workspace object once (rather than a kubectl call per field) and assert on the
			// decoded spec/status -- one round-trip, all workspace-side checks read from the same snapshot.
			var ws workspacev1alpha1.Workspace
			Expect(kubectlGetInto("workspace", workspaceName, workspaceNamespace, &ws)).To(Succeed())

			// Assert EXACTLY ONE frozen entry for the template: a plain "an entry named X exists" check is
			// tautological (we filter by that name), and it would also miss a duplicate-overlay regression.
			frozenMatches := resolvedIntegrationsNamed(&ws, "service-integration")
			Expect(frozenMatches).To(HaveLen(1),
				"the operator must record exactly one frozen resolvedIntegrations entry for the template")
			Expect(frozenMatches[0].ParametersHash).NotTo(BeEmpty(),
				"a frozen integration must carry a parametersHash (hash of templateRef+parameters)")

			deploymentName := ws.Status.DeploymentName
			Expect(deploymentName).NotTo(BeEmpty())

			By("fetching the deployment once and validating the injected overlay against it")
			// Same idea for the deployment: one get, then assert every injected field against the decoded
			// pod template.
			var deploy appsv1.Deployment
			Expect(kubectlGetInto("deployment", deploymentName, workspaceNamespace, &deploy)).To(Succeed())
			podSpec := deploy.Spec.Template.Spec

			sidecar := containerByName(podSpec, "cache-proxy")
			Expect(sidecar).NotTo(BeNil(), "the cache-proxy sidecar must be injected")
			Expect(sidecar.Image).To(Equal("busybox:1.36"), "the sidecar must be injected with its pinned image")
			Expect(sidecar.Args).NotTo(BeEmpty())
			Expect(sidecar.Args[0]).To(ContainSubstring("shared-cache:6379"),
				"the sidecar args must be resolved from the looked-up Service (name:port)")

			// The sidecar must carry NO readinessProbe: integration health is reported by the report-only
			// statusProbe (status.integrationStatuses[]), never by gating pod readiness. A readinessProbe
			// here would drop the Workspace to Available=False on a backing-service outage even though
			// JupyterLab is fine. Lock that in so it cannot regress.
			Expect(sidecar.ReadinessProbe).To(BeNil(),
				"the sidecar must not gate readiness on backing-service connectivity")

			primary := containerByName(podSpec, controller.PrimaryContainerName)
			Expect(primary).NotTo(BeNil(), "the primary workspace container must be present")
			Expect(envValue(primary, "CACHE_HOST")).To(Equal("shared-cache"))
			Expect(envValue(primary, "CACHE_PORT")).To(Equal("6379"),
				"the port must be resolved from the Service's spec.ports")
			Expect(envValue(primary, "CACHE_SERVICE_NAME")).To(Equal("shared-cache"))

			Expect(podSpec.ShareProcessNamespace).NotTo(BeNil(),
				"the deployment builder must OR-reduce shareProcessNamespace onto the pod")
			Expect(*podSpec.ShareProcessNamespace).To(BeTrue(),
				"the template's shareProcessNamespace must be OR-reduced onto the pod")

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
				var ws workspacev1alpha1.Workspace
				g.Expect(kubectlGetInto("workspace", "workspace-failprobe", workspaceNamespace, &ws)).To(Succeed())
				is := findIntegrationStatus(&ws, "service-integration-failprobe")
				g.Expect(is).NotTo(BeNil())
				g.Expect(is.State).To(Equal("Degraded"), "a failing statusProbe must report Degraded")
				g.Expect(conditionReason(is, "Ready")).To(Equal("ProbeFailed"),
					"the Ready condition reason must be ProbeFailed")
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
		It("replays the FROZEN port with no restart while frozen, then rolls once a template edit frees it", func() {
			By("creating the Service and template")
			applyIntegrationFixture("service-cache")
			applyIntegrationFixture("service-integration")

			By("creating the workspace and waiting for it to become available")
			createWorkspaceForTest("workspace-with-integration", groupDir, "")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			var ws workspacev1alpha1.Workspace
			Expect(kubectlGetInto("workspace", workspaceName, workspaceNamespace, &ws)).To(Succeed())
			deploymentName := ws.Status.DeploymentName
			Expect(deploymentName).NotTo(BeEmpty())

			originalPodUID := workspacePodUID(workspaceName, workspaceNamespace)
			Expect(originalPodUID).NotTo(BeEmpty())

			// --- Phase A: freeze holds -> drift is ignored AND the pod is never restarted --------------
			By("drifting the Service's port underneath the workspace (6379 -> 6390)")
			// Same Service, new port: the freeze key (parametersHash + template generation) is unchanged, so
			// the operator must replay the frozen 6379 and never read this value.
			applyIntegrationFixture("service-cache-drifted")

			By("re-reconciling the drift without disturbing the pod (nudge the owned Deployment, not the workspace)")
			// Forces a real reconcile (else the only requeue is the 5m cadence and the check below is vacuous).
			// A Deployment-metadata nudge isn't in the pod template, so it can't roll the pod itself.
			touchDeploymentToForceReconcile(deploymentName, workspaceNamespace)

			By("verifying the operator replays the frozen port AND never restarts the pod")
			// Frozen CACHE_PORT stays 6379 (drift ignored) and the pod UID is unchanged (not recreated). UID,
			// not deployment.generation, is the clean no-restart signal since nothing annotates the workspace.
			Consistently(func(g Gomega) {
				var deploy appsv1.Deployment
				g.Expect(kubectlGetInto("deployment", deploymentName, workspaceNamespace, &deploy)).To(Succeed())
				g.Expect(envValue(containerByName(deploy.Spec.Template.Spec, controller.PrimaryContainerName), "CACHE_PORT")).
					To(Equal("6379"), "drift must be ignored: the frozen port must be replayed")
				g.Expect(workspacePodUID(workspaceName, workspaceNamespace)).
					To(Equal(originalPodUID), "the running pod must NOT be restarted while the freeze holds")
			}, 20*time.Second, 5*time.Second).Should(Succeed())

			// --- Phase B (negative control): freeze released -> the SAME drift now rolls the pod ---------
			// The drift (6390) was live all along. A generation-only edit (displayName; nothing new on the pod)
			// flips hasIntegrationChanged, so the operator re-reads it and the pod adopts 6390 with a new UID.
			// That proves the value was always reachable -- the freeze was Phase A's only reason, not vacuous.
			By("editing the template to bump its generation (no rendered field changes)")
			applyIntegrationFixture("service-integration-genbump")

			By("verifying the freed reconcile re-reads the drifted Service and rolls the pod")
			// WIT isn't watched, so nudge inside the poll to enqueue the reconcile that sees the bump.
			Eventually(func(g Gomega) {
				touchDeploymentToForceReconcile(deploymentName, workspaceNamespace)
				var deploy appsv1.Deployment
				g.Expect(kubectlGetInto("deployment", deploymentName, workspaceNamespace, &deploy)).To(Succeed())
				g.Expect(envValue(containerByName(deploy.Spec.Template.Spec, controller.PrimaryContainerName), "CACHE_PORT")).
					To(Equal("6390"), "once the freeze key moves, the operator must adopt the drifted port")
				g.Expect(workspacePodUID(workspaceName, workspaceNamespace)).
					NotTo(Equal(originalPodUID), "re-resolving the drifted port must roll the pod (freeze was Phase A's only reason)")
			}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
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

			By("capturing the deployment name and the original frozen input token from one workspace read")
			var ws workspacev1alpha1.Workspace
			Expect(kubectlGetInto("workspace", workspaceName, workspaceNamespace, &ws)).To(Succeed())
			deploymentName := ws.Status.DeploymentName
			Expect(deploymentName).NotTo(BeEmpty())
			frozen := findResolvedIntegration(&ws, "service-integration")
			Expect(frozen).NotTo(BeNil())
			tokenA := frozen.ParametersHash
			Expect(tokenA).NotTo(BeEmpty())

			By("patching the workspace to point at other-cache")
			// A merge patch updates the existing workspace's integrationTemplateRef in place -- a
			// controlled parameter change (serviceName -> other-cache), not a new workspace.
			patchWorkspaceFromFile(workspaceName, workspaceNamespace,
				"test/e2e/static/integration/patch-workspace-switch-cluster.json")

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

	Context("Editing the referenced integration template under a stable workspace input", func() {
		It("re-resolves on the template-version bump (not the parameter hash) and re-renders the pod with the edit", func() {
			By("creating the Service and the initial template")
			applyIntegrationFixture("service-cache")
			applyIntegrationFixture("service-integration")

			By("creating the workspace and waiting for it to become available")
			createWorkspaceForTest("workspace-with-integration", groupDir, "")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			By("capturing the deployment name and pre-edit frozen record from one workspace read")
			// The workspace's parameters do not change in this spec, so parametersHash must stay fixed --
			// only the template Generation (observedIntegrationTemplateVersion) moves. Capturing both lets
			// the assertions prove the re-resolve was driven by the TEMPLATE edit, not a parameter change.
			var ws workspacev1alpha1.Workspace
			Expect(kubectlGetInto("workspace", workspaceName, workspaceNamespace, &ws)).To(Succeed())
			deploymentName := ws.Status.DeploymentName
			Expect(deploymentName).NotTo(BeEmpty())
			frozenBefore := findResolvedIntegration(&ws, "service-integration")
			Expect(frozenBefore).NotTo(BeNil())
			paramHashBefore := frozenBefore.ParametersHash
			Expect(paramHashBefore).NotTo(BeEmpty())
			tmplVersionBefore := frozenBefore.ObservedIntegrationTemplateVersion
			Expect(tmplVersionBefore).NotTo(BeEmpty())

			By("confirming the edit's env var is absent before the template is edited")
			markerBefore, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[0].env[?(@.name=='CACHE_EDITED_MARKER')].value}")
			Expect(err).NotTo(HaveOccurred())
			Expect(markerBefore).To(BeEmpty(), "the edited-marker env must not exist before the template edit")

			By("editing the template in place (adds a resolved env var; parameters unchanged)")
			applyIntegrationFixture("service-integration-edited")

			By("nudging the workspace to reconcile so it observes the new template version")
			// The controller watches Workspace (and owns Deployment/Service/PVC) but does NOT watch
			// WorkspaceIntegrationTemplate, so a template edit alone does not enqueue the referencing
			// workspace -- the re-resolve otherwise waits for the next periodic reconcile (the integration
			// probe cadence, 5m in the chart). A real admin gets the update on that timer; here we annotate
			// the workspace to trigger an immediate reconcile so the spec is deterministic without a 5m wait.
			touchWorkspaceToForceReconcile(workspaceName, workspaceNamespace)

			By("verifying observedIntegrationTemplateVersion bumped while parametersHash stayed the same")
			Eventually(func(g Gomega) {
				var wsAfter workspacev1alpha1.Workspace
				g.Expect(kubectlGetInto("workspace", workspaceName, workspaceNamespace, &wsAfter)).To(Succeed())
				frozenAfter := findResolvedIntegration(&wsAfter, "service-integration")
				g.Expect(frozenAfter).NotTo(BeNil())
				g.Expect(frozenAfter.ObservedIntegrationTemplateVersion).NotTo(Equal(tmplVersionBefore),
					"editing the template must bump observedIntegrationTemplateVersion")
				g.Expect(frozenAfter.ParametersHash).To(Equal(paramHashBefore),
					"parametersHash must NOT change: the re-resolve is driven by the template edit, not a parameter change")
			}).WithTimeout(2 * time.Minute).WithPolling(3 * time.Second).Should(Succeed())

			By("verifying the pod was re-rendered with the edit's resolved env var")
			Eventually(func(g Gomega) {
				marker, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
					"{.spec.template.spec.containers[0].env[?(@.name=='CACHE_EDITED_MARKER')].value}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(marker).To(Equal("shared-cache-edited"),
					"the edited template's new env var must be resolved from the Service and rendered onto the pod")
			}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())
		})
	})

	Context("Removing the only integration ref from a running workspace", func() {
		It("prunes the frozen status entry and re-renders a base-only pod while staying Available", func() {
			By("creating the Service and template")
			applyIntegrationFixture("service-cache")
			applyIntegrationFixture("service-integration")

			By("creating the workspace with the integration and waiting for it to become available")
			createWorkspaceForTest("workspace-with-integration", groupDir, "")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())

			By("confirming the sidecar and frozen record exist before removal")
			sidecarName, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[?(@.name=='cache-proxy')].name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(sidecarName).To(Equal("cache-proxy"))

			By("patching the workspace to drop its only integrationTemplateRef")
			// A merge patch sets integrationTemplateRefs to [], removing the only integration in place.
			patchWorkspaceFromFile(workspaceName, workspaceNamespace,
				"test/e2e/static/integration/patch-workspace-remove-integration.json")

			By("verifying the frozen resolvedIntegrations entry is pruned")
			Eventually(func(g Gomega) {
				frozenName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
					"{.status.resolvedIntegrations[?(@.name=='service-integration')].name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(frozenName).To(BeEmpty(),
					"removing the ref must prune its frozen resolvedIntegrations entry")
			}).WithTimeout(time.Minute).WithPolling(3 * time.Second).Should(Succeed())

			By("verifying the integration status entry is pruned")
			Eventually(func(g Gomega) {
				statusName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
					"{.status.integrationStatuses[?(@.name=='service-integration')].name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statusName).To(BeEmpty(),
					"removing the ref must prune its integrationStatuses entry")
			}).WithTimeout(time.Minute).WithPolling(3 * time.Second).Should(Succeed())

			By("verifying the pod re-rendered base-only (the sidecar was removed)")
			Eventually(func(g Gomega) {
				names, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
					"{.spec.template.spec.containers[?(@.name=='cache-proxy')].name}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(names).To(BeEmpty(), "the sidecar overlay must be removed when its ref is dropped")
			}).WithTimeout(2 * time.Minute).WithPolling(5 * time.Second).Should(Succeed())

			By("verifying the workspace remains Available after the integration is removed")
			WaitForWorkspaceToReachCondition(
				workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)
		})
	})

	Context("Workspace referencing a missing resource", func() {
		It("is fail-closed and non-fatal: the workspace becomes Available with a base-only pod", func() {
			By("creating the integration template (no matching Service will exist)")
			applyIntegrationFixture("service-integration")

			By("creating a workspace whose parameter points to a nonexistent Service")
			createWorkspaceForTest("workspace-missing-resource", groupDir, "")

			By("waiting for the workspace to become Available despite the unresolvable integration")
			// A first-attach failure is non-fatal: no frozen values exist yet, so the operator deploys
			// the pod base-only and the base reconcile still succeeds. The workspace must NOT go Degraded;
			// the integration's failure is surfaced only in status.integrationStatuses[].
			WaitForWorkspaceToReachCondition(
				"workspace-missing-resource", workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			By("reading the workspace once for the deployment name and frozen-record state")
			var ws workspacev1alpha1.Workspace
			Expect(kubectlGetInto("workspace", "workspace-missing-resource", workspaceNamespace, &ws)).To(Succeed())
			deploymentName := ws.Status.DeploymentName
			Expect(deploymentName).NotTo(BeEmpty())

			By("verifying NO cache-proxy was injected (capture failed, so no overlay is applied)")
			verifyNoSidecarInDeployment(deploymentName, workspaceNamespace)

			By("verifying no frozen values were recorded for the unresolvable integration")
			// Capture never succeeded, so the freeze must not advance to a partial/empty frozen record.
			Expect(findResolvedIntegration(&ws, "service-integration")).To(BeNil(),
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
