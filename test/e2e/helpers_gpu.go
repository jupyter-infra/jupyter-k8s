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
// advertise an extended resource without a device plugin; the scheduler then fits pods against it.

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

// setupFakeGPUNode advertises fake GPU capacity on the first schedulable node and labels it for
// nodeSelector-based placement. Returns the node name. Capacity and allocatable are patched
// together: the kubelet only recomputes allocatable from capacity on its periodic status sync,
// too slow for a test to wait on.
func setupFakeGPUNode() string {
	ginkgo.GinkgoHelper()

	node := firstSchedulableNode()
	nodeName := node.Name
	// A JSON patch "add" on an existing path overwrites it. Capacity advertised by a node that does
	// not carry this suite's label comes from real hardware or a device plugin, not from an earlier run.
	if node.Labels[fakeGPUNodeLabel] != valueTrue {
		gomega.Expect(node.Status.Capacity).NotTo(gomega.HaveKey(fakeGPUResource),
			"node %s already advertises %s; refusing to overwrite real GPU capacity", nodeName, fakeGPUResourceName)
	}

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
	var patched corev1.Node
	gomega.Expect(kubectlGetInto("node", nodeName, "", &patched)).To(gomega.Succeed())
	gomega.Expect(patched.Status.Allocatable).To(gomega.HaveKey(fakeGPUResource))

	ginkgo.By(fmt.Sprintf("labeling node %s with %s=true", nodeName, fakeGPUNodeLabel))
	cmd = exec.Command("kubectl", "label", "--overwrite", "node", nodeName, fakeGPUNodeLabel+"=true")
	_, err = utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	return nodeName
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

// firstSchedulableNode returns the first node without a NoSchedule or NoExecute taint. On a
// multi-node Kind cluster the first listed node is the tainted control plane, which the GPU
// template's tolerations do not cover.
func firstSchedulableNode() *corev1.Node {
	ginkgo.GinkgoHelper()
	var nodes corev1.NodeList
	gomega.Expect(kubectlGetInto("nodes", "", "", &nodes)).To(gomega.Succeed())
	for i := range nodes.Items {
		if !hasSchedulingTaint(nodes.Items[i].Spec.Taints) {
			return &nodes.Items[i]
		}
	}
	ginkgo.Fail("no schedulable node in the cluster")
	return nil
}

func hasSchedulingTaint(taints []corev1.Taint) bool {
	for _, t := range taints {
		if t.Effect == corev1.TaintEffectNoSchedule || t.Effect == corev1.TaintEffectNoExecute {
			return true
		}
	}
	return false
}

// jsonPatchEscapedGPUResource returns fakeGPUResourceName as a JSON patch path segment. A path
// like /status/capacity/nvidia.com/gpu would read the '/' as a separator, so it is written '~1'
// (JSON Pointer, RFC 6901).
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

// workspacePods returns the workspace's pods, decoded. Errors are returned rather than asserted so
// callers can poll: a pod listed while the workspace restarts may be gone by the time it is read.
func workspacePods(workspaceName, namespace string) ([]corev1.Pod, error) {
	names, err := kubectlGetByLabels("pod",
		fmt.Sprintf("%s=%s", controller.LabelWorkspaceName, workspaceName),
		namespace, "{.items[*].metadata.name}")
	if err != nil {
		return nil, err
	}
	var pods []corev1.Pod
	for _, name := range strings.Fields(names) {
		var pod corev1.Pod
		if err := kubectlGetInto("pod", name, namespace, &pod); err != nil {
			return nil, err
		}
		pods = append(pods, pod)
	}
	return pods, nil
}

// patchWorkspaceGPU sets the workspace's nvidia.com/gpu request and limit to gpus with a merge
// patch, leaving cpu and memory as they are. Returns kubectl's output and error so callers can
// assert either acceptance or the webhook's rejection.
func patchWorkspaceGPU(workspaceName, namespace, gpus string) (string, error) {
	patch := fmt.Sprintf(`{"spec":{"resources":{"requests":{%[1]q:%[2]q},"limits":{%[1]q:%[2]q}}}}`,
		fakeGPUResourceName, gpus)
	cmd := exec.Command("kubectl", "patch", "workspace", workspaceName,
		"-n", namespace, "--type=merge", "-p", patch)
	return utils.Run(cmd)
}

// deleteResourcesForGPUTest removes only the objects this Ordered suite creates, by explicit name,
// so it can never delete unrelated objects sharing the "default" namespace.
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
