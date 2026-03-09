//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
)

var _ = Describe("Workspace Security", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "security"
	)

	AfterEach(func() {
		By("cleaning up test resources")
		_ = kubectlDeleteAllNamespaces("workspaces", "--ignore-not-found", "--wait=true", "--timeout=180s")
	})

	It("should pass the workspace container security context onto the main container of the pod", func() {
		workspaceName := "container-sc-workspace"
		workspaceFilename := "container-sc-workspace"

		By("creating workspace with container security context")
		createWorkspaceForTest(workspaceFilename, groupDir, "")

		By("waiting for workspace to become available")
		WaitForWorkspaceToReachCondition(
			workspaceName,
			workspaceNamespace,
			controller.ConditionTypeAvailable,
			ConditionTrue,
		)

		By("getting the pod name")
		podName, err := kubectlGetByLabels("pod",
			fmt.Sprintf("%s=%s", controller.LabelWorkspaceName, workspaceName),
			workspaceNamespace,
			"{.items[0].metadata.name}")
		Expect(err).NotTo(HaveOccurred())
		Expect(podName).NotTo(BeEmpty())

		By("verifying container security context on the main container")
		runAsNonRoot, err := kubectlGet("pod", podName, workspaceNamespace,
			"{.spec.containers[0].securityContext.runAsNonRoot}")
		Expect(err).NotTo(HaveOccurred())
		Expect(runAsNonRoot).To(Equal("true"))

		runAsUser, err := kubectlGet("pod", podName, workspaceNamespace,
			"{.spec.containers[0].securityContext.runAsUser}")
		Expect(err).NotTo(HaveOccurred())
		Expect(runAsUser).To(Equal("1000"))
	})

	It("should pass the workspace pod security context into the underlying pod", func() {
		workspaceName := "pod-sc-workspace"
		workspaceFilename := "pod-sc-workspace"

		By("creating workspace with pod security context")
		createWorkspaceForTest(workspaceFilename, groupDir, "")

		By("waiting for workspace to become available")
		WaitForWorkspaceToReachCondition(
			workspaceName,
			workspaceNamespace,
			controller.ConditionTypeAvailable,
			ConditionTrue,
		)

		By("getting the pod name")
		podName, err := kubectlGetByLabels("pod",
			fmt.Sprintf("%s=%s", controller.LabelWorkspaceName, workspaceName),
			workspaceNamespace,
			"{.items[0].metadata.name}")
		Expect(err).NotTo(HaveOccurred())
		Expect(podName).NotTo(BeEmpty())

		By("verifying pod security context on the pod")
		fsGroup, err := kubectlGet("pod", podName, workspaceNamespace,
			"{.spec.securityContext.fsGroup}")
		Expect(err).NotTo(HaveOccurred())
		Expect(fsGroup).To(Equal("1000"))

		runAsUser, err := kubectlGet("pod", podName, workspaceNamespace,
			"{.spec.securityContext.runAsUser}")
		Expect(err).NotTo(HaveOccurred())
		Expect(runAsUser).To(Equal("1000"))
	})
})
