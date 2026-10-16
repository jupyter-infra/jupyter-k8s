//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
)

// A sidecar port only becomes routable if the access strategy opts into it via exposedPorts.
// These specs drive that through a real cluster: the controller has to resolve each listed name
// against the injected containers and publish exactly those ports on the workspace Service.
var _ = Describe("Workspace Sidecar Service Ports", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "access-strategy"

		exposedPortName   = "sidecar-ws"
		exposedPortNumber = "8080"
		// Declared by the sidecar but deliberately left out of exposedPorts.
		unexposedPortName   = "sidecar-admin"
		unexposedPortNumber = "9090"
	)

	AfterEach(func() {
		deleteResourcesForAccessStrategyTest(workspaceNamespace)
	})

	Context("when an access strategy exposes a sidecar port", func() {
		const (
			accessStrategyName = "access-strategy-with-exposed-ports"
			workspaceName      = "workspace-with-exposed-ports-access-strategy"
		)

		It("publishes the exposed port and leaves the unexposed one off the Service", func() {
			By("creating an access strategy whose sidecar declares two ports and exposes one")
			createAccessStrategyForTest(accessStrategyName, groupDir, "")

			By("creating a workspace referencing that access strategy")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying the sidecar was injected into the deployment")
			deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			Expect(deploymentName).NotTo(BeEmpty(), "workspace.status.deploymentName should be set")

			sidecarName, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
				"{.spec.template.spec.containers[?(@.name=='port-sidecar')].name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(sidecarName).To(Equal("port-sidecar"), "access strategy sidecar should be injected")

			serviceName := GetWorkspaceServiceName(workspaceName, workspaceNamespace)

			By("verifying the application port is still published")
			Eventually(func(g Gomega) {
				port, err := kubectlGet("service", serviceName, workspaceNamespace,
					fmt.Sprintf("{.spec.ports[?(@.name=='%s')].port}", "http"))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(port).To(Equal(fmt.Sprintf("%d", controller.JupyterPort)))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying the exposed sidecar port is published and targets the container port")
			Eventually(func(g Gomega) {
				port, err := kubectlGet("service", serviceName, workspaceNamespace,
					fmt.Sprintf("{.spec.ports[?(@.name=='%s')].port}", exposedPortName))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(port).To(Equal(exposedPortNumber))

				targetPort, err := kubectlGet("service", serviceName, workspaceNamespace,
					fmt.Sprintf("{.spec.ports[?(@.name=='%s')].targetPort}", exposedPortName))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(targetPort).To(Equal(exposedPortNumber))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())

			By("verifying the port the strategy did not expose is absent from the Service")
			names, err := GetServicePortNames(serviceName, workspaceNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(names).NotTo(ContainSubstring(unexposedPortName),
				"a declared containerPort must not be published unless exposedPorts lists it")
			Expect(names).NotTo(ContainSubstring(unexposedPortNumber))
		})

		It("adds and removes a port as exposedPorts changes on a running workspace", func() {
			By("creating the access strategy and workspace")
			createAccessStrategyForTest(accessStrategyName, groupDir, "")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			serviceName := GetWorkspaceServiceName(workspaceName, workspaceNamespace)

			By("exposing the second declared port on the live access strategy")
			patchAccessStrategy(groupDir, "updates", "access-strategy-expose-both-ports-patch", accessStrategyName)

			By("verifying the newly exposed port appears on the Service")
			Eventually(func(g Gomega) {
				port, err := kubectlGet("service", serviceName, workspaceNamespace,
					fmt.Sprintf("{.spec.ports[?(@.name=='%s')].port}", unexposedPortName))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(port).To(Equal(unexposedPortNumber))
			}, 3*time.Minute, 5*time.Second).Should(Succeed(),
				"the reconciler should add a port once the access strategy exposes it")

			By("removing it again from exposedPorts")
			patchAccessStrategy(groupDir, "updates", "access-strategy-expose-one-port-patch", accessStrategyName)

			By("verifying the port is withdrawn from the Service")
			Eventually(func(g Gomega) {
				names, err := GetServicePortNames(serviceName, workspaceNamespace)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(names).NotTo(ContainSubstring(unexposedPortName))
				// The originally exposed port must survive the update.
				g.Expect(names).To(ContainSubstring(exposedPortName))
			}, 3*time.Minute, 5*time.Second).Should(Succeed(),
				"the reconciler should withdraw a port once the access strategy stops exposing it")

			By("verifying the workspace stayed healthy across both updates")
			VerifyWorkspaceConditions(workspaceName, workspaceNamespace, map[string]string{
				controller.ConditionTypeProgressing: ConditionFalse,
				controller.ConditionTypeDegraded:    ConditionFalse,
				controller.ConditionTypeAvailable:   ConditionTrue,
				controller.ConditionTypeStopped:     ConditionFalse,
				controller.ConditionTypeDeleting:    ConditionFalse,
			})
		})
	})

	Context("when exposedPorts names a port no container declares", func() {
		const (
			accessStrategyName = "access-strategy-invalid-exposed-port"
			workspaceName      = "workspace-with-invalid-exposed-port"
		)

		// The name cannot be resolved, so the Service cannot be built. That has to surface on the
		// workspace rather than quietly publishing a Service missing the port the author asked for.
		It("reports the failure instead of silently publishing nothing", func() {
			By("creating an access strategy exposing an unresolvable port name")
			createAccessStrategyForTest(accessStrategyName, groupDir, "")

			By("creating a workspace referencing that access strategy")
			createWorkspaceForTest(workspaceName, groupDir, "")

			By("waiting for the workspace to report Degraded")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeDegraded,
				ConditionTrue,
			)

			By("verifying the failure is attributed to the service and names the offending port")
			Eventually(func(g Gomega) {
				reason, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
					fmt.Sprintf("{.status.conditions[?(@.type=='%s')].reason}", controller.ConditionTypeDegraded))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(reason).To(Equal(controller.ReasonServiceError))

				message, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
					fmt.Sprintf("{.status.conditions[?(@.type=='%s')].message}", controller.ConditionTypeDegraded))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(message).To(ContainSubstring("no-such-port"))
			}, 2*time.Minute, 5*time.Second).Should(Succeed())
		})
	})
})
