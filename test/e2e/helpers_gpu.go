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

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// Helpers for the Workspace GPU e2e suite (workspace_gpu_test.go). Kind nodes have no GPUs, so the
// suite advertises nvidia.com/gpu on a node through the status subresource, the documented way to
// advertise an extended resource without a device plugin. The scheduler fits pods against the
// patched allocatable and the kubelet runs them; only CUDA itself would need real hardware.

const (
	// fakeGPUResourceName is the extended resource advertised on the fake GPU node.
	fakeGPUResourceName = "nvidia.com/gpu"
	// fakeGPUNodeLabel marks the advertising node; the GPU template's defaultNodeSelector targets it.
	fakeGPUNodeLabel = "e2e.jupyter.org/fake-gpu"
	// fakeGPUCapacity leaves headroom over the largest single request (2) so a pod still
	// terminating from the previous spec cannot make the next one unschedulable.
	fakeGPUCapacity = "4"
)

// fakeGPUResource is fakeGPUResourceName as a typed key into ResourceList maps.
var fakeGPUResource = corev1.ResourceName(fakeGPUResourceName)

// setupFakeGPUNode advertises fake GPU capacity on the cluster's first node and labels it for
// nodeSelector-based placement. Returns the node name.
func setupFakeGPUNode() string {
	ginkgo.GinkgoHelper()

	nodeName, err := kubectlGet("nodes", "", "", "{.items[0].metadata.name}")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(nodeName).NotTo(gomega.BeEmpty(), "expected at least one node in the cluster")

	advertiseFakeGPU(nodeName)

	ginkgo.By(fmt.Sprintf("labeling node %s with %s=true", nodeName, fakeGPUNodeLabel))
	cmd := exec.Command("kubectl", "label", "--overwrite", "node", nodeName, fakeGPUNodeLabel+"=true")
	_, err = utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return nodeName
}

// advertiseFakeGPU patches fake GPU capacity onto the node status. Capacity and allocatable are
// patched together: the kubelet only recomputes allocatable from capacity on its periodic status
// sync, too slow for a test to wait on.
func advertiseFakeGPU(nodeName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("advertising %s=%s on node %s", fakeGPUResourceName, fakeGPUCapacity, nodeName))
	patch := fmt.Sprintf(
		`[{"op":"add","path":"/status/capacity/%[1]s","value":"%[2]s"},`+
			`{"op":"add","path":"/status/allocatable/%[1]s","value":"%[2]s"}]`,
		jsonPatchEscapedGPUResource(), fakeGPUCapacity)
	cmd := exec.Command("kubectl", "patch", "node", nodeName,
		"--subresource=status", "--type=json", "-p", patch)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	ginkgo.By("verifying the node reports the fake GPU allocatable")
	var node corev1.Node
	gomega.Expect(kubectlGetInto("node", nodeName, "", &node)).To(gomega.Succeed())
	gomega.Expect(node.Status.Allocatable).To(gomega.HaveKey(fakeGPUResource))
}

// removeFakeGPUAdvertisement removes the fake GPU capacity and allocatable from the node status,
// leaving the node label in place.
func removeFakeGPUAdvertisement(nodeName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("removing the %s advertisement from node %s", fakeGPUResourceName, nodeName))
	patch := fmt.Sprintf(
		`[{"op":"remove","path":"/status/capacity/%[1]s"},`+
			`{"op":"remove","path":"/status/allocatable/%[1]s"}]`,
		jsonPatchEscapedGPUResource())
	cmd := exec.Command("kubectl", "patch", "node", nodeName,
		"--subresource=status", "--type=json", "-p", patch)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// teardownFakeGPUNode removes the fake GPU advertisement and label. Best-effort: CI deletes the
// Kind cluster after the suite; this matters for reruns against a kept dev cluster.
func teardownFakeGPUNode(nodeName string) {
	ginkgo.GinkgoHelper()
	if nodeName == "" {
		return
	}
	patch := fmt.Sprintf(
		`[{"op":"remove","path":"/status/capacity/%[1]s"},`+
			`{"op":"remove","path":"/status/allocatable/%[1]s"}]`,
		jsonPatchEscapedGPUResource())
	cmd := exec.Command("kubectl", "patch", "node", nodeName,
		"--subresource=status", "--type=json", "-p", patch)
	_, _ = utils.Run(cmd)
	cmd = exec.Command("kubectl", "label", "node", nodeName, fakeGPUNodeLabel+"-")
	_, _ = utils.Run(cmd)
}

// jsonPatchEscapedGPUResource returns fakeGPUResourceName with '/' escaped as '~1' (RFC 6901).
func jsonPatchEscapedGPUResource() string {
	return strings.ReplaceAll(fakeGPUResourceName, "/", "~1")
}

// gpuQuantity returns the nvidia.com/gpu value in a resource list and whether the key is present.
func gpuQuantity(list corev1.ResourceList) (int64, bool) {
	q, ok := list[fakeGPUResource]
	if !ok {
		return 0, false
	}
	return q.Value(), true
}

// hasToleration reports whether tolerations contains an exact key/operator/effect match.
func hasToleration(
	tolerations []corev1.Toleration,
	key string,
	op corev1.TolerationOperator,
	effect corev1.TaintEffect,
) bool {
	for _, t := range tolerations {
		if t.Key == key && t.Operator == op && t.Effect == effect {
			return true
		}
	}
	return false
}

// workspacePod returns the workspace's pod, decoded.
func workspacePod(workspaceName, namespace string) *corev1.Pod {
	ginkgo.GinkgoHelper()
	podName, err := kubectlGetByLabels("pod",
		fmt.Sprintf("%s=%s", controller.LabelWorkspaceName, workspaceName),
		namespace, "{.items[0].metadata.name}")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(podName).NotTo(gomega.BeEmpty())
	var pod corev1.Pod
	gomega.Expect(kubectlGetInto("pod", podName, namespace, &pod)).To(gomega.Succeed())
	return &pod
}

// deleteResourcesForGPUTest removes only the objects this Ordered suite creates, by explicit name,
// so it can never nuke unrelated objects sharing the "default" namespace.
func deleteResourcesForGPUTest(workspaceNamespace string) {
	ginkgo.GinkgoHelper()

	ginkgo.By("cleaning up GPU test workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace",
		"gpu-default-workspace", "gpu-override-workspace", "gpu-exceed-workspace",
		"gpu-cpu-only-workspace", "gpu-no-template-workspace",
		"-n", workspaceNamespace, "--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	ginkgo.By("cleaning up the GPU template")
	cmd = exec.Command("kubectl", "delete", "workspacetemplate", "gpu-template",
		"-n", SharedNamespace, "--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)
}
