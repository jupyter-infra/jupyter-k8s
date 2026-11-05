//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
)

// status.connectionTypes tells a client which WorkspaceConnection types a workspace accepts without
// reading its access strategy. The controller derives it from the bound strategy, so this spec drives
// the strategy through a real cluster: a browser-only route, the WebSocket route added later, a stop.
var _ = Describe("Workspace Connection Types", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "access-strategy"
		subGroup           = "connection-types"
		accessStrategyName = "access-strategy-connection-types"
		workspaceName      = "workspace-connection-types"
	)

	// connectionTypesShould polls status.connectionTypes as a client decodes it until it satisfies matcher.
	connectionTypesShould := func(matcher OmegaMatcher) {
		GinkgoHelper()
		Eventually(func(g Gomega) {
			var ws workspacev1alpha1.Workspace
			g.Expect(kubectlGetInto("workspace", workspaceName, workspaceNamespace, &ws)).To(Succeed())
			g.Expect(ws.Status.ConnectionTypes).To(matcher)
		}).WithTimeout(2 * time.Minute).WithPolling(2 * time.Second).Should(Succeed())
	}

	AfterEach(func() {
		deleteResourcesForAccessStrategyTest(workspaceNamespace)
	})

	It("publishes the connection types the bound access strategy provides", func() {
		By("creating an access strategy with a bearer auth URL template and no WebSocket URL template")
		createAccessStrategyForTest("access-strategy-web-ui-only", groupDir, subGroup)

		By("creating a running workspace referencing that access strategy")
		createWorkspaceForTest(workspaceName, groupDir, subGroup)
		WaitForWorkspaceToReachCondition(
			workspaceName, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

		By("verifying status.connectionTypes lists web-ui only")
		connectionTypesShould(Equal([]string{"web-ui"}))

		By("adding the WebSocket URL template and a vscode-remote handler to the live access strategy")
		patchAccessStrategy(groupDir, subGroup, "access-strategy-add-websocket-patch", accessStrategyName)

		By("verifying status.connectionTypes is rewritten with ssh-over-websocket and the mapped type")
		connectionTypesShould(Equal([]string{"web-ui", "ssh-over-websocket", "vscode-remote"}))

		By("stopping the workspace")
		UpdateWorkspaceDesiredState(workspaceName, workspaceNamespace, controller.DesiredStateStopped)
		WaitForWorkspaceToReachCondition(
			workspaceName, workspaceNamespace, controller.ConditionTypeStopped, ConditionTrue)

		By("verifying status.connectionTypes is absent while the workspace is stopped")
		connectionTypesShould(BeNil())
	})
})
