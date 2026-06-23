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

var _ = Describe("Workspace Readiness Probes", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "readiness-probe"
	)

	AfterEach(func() {
		deleteResourcesForReadinessProbeTest()
	})

	It("should mark the pod ready once the TCP probe passes for the jupyterlab-uv image", func() {
		workspaceName := "workspace-tcp-readiness"

		By("creating a workspace with a TCP readiness probe on :8888")
		createWorkspaceForTest(workspaceName, groupDir, "")

		By("waiting for the workspace to become Available")
		WaitForWorkspaceToReachCondition(
			workspaceName,
			workspaceNamespace,
			controller.ConditionTypeAvailable,
			ConditionTrue,
		)

		By("verifying the TCP readiness probe is set on the deployment's primary container")
		deploymentName, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
			"{.status.deploymentName}")
		Expect(err).NotTo(HaveOccurred())
		Expect(deploymentName).NotTo(BeEmpty())

		probePort, err := kubectlGet("deployment", deploymentName, workspaceNamespace,
			"{.spec.template.spec.containers[0].readinessProbe.tcpSocket.port}")
		Expect(err).NotTo(HaveOccurred())
		Expect(probePort).To(Equal("8888"))

		By("retrieving the workspace pod by label")
		podSelector := fmt.Sprintf("%s=%s", WorkspaceLabelName, workspaceName)
		podName, err := kubectlGetByLabels("pod", podSelector, workspaceNamespace,
			"{.items[*].metadata.name}")
		Expect(err).NotTo(HaveOccurred())
		Expect(podName).NotTo(BeEmpty())

		By("verifying the probe actually passes: pod reaches Running with the container ready")
		WaitForWorkspacePodToBeReady(podName, workspaceNamespace)
	})
})

func deleteResourcesForReadinessProbeTest() {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("cleaning up standalone PVCs")
	cmd = exec.Command("kubectl", "delete", "pvc", "--all", "-n", "default",
		"--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)

	By("waiting for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
