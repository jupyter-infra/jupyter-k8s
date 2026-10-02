//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// isUsingFinch detects if the test environment is using Finch container runtime
func isUsingFinch() bool {
	// Check CONTAINER_TOOL environment variable
	if containerTool := os.Getenv("CONTAINER_TOOL"); containerTool == "finch" {
		return true
	}
	// Check KIND_EXPERIMENTAL_PROVIDER environment variable
	if kindProvider := os.Getenv("KIND_EXPERIMENTAL_PROVIDER"); kindProvider == "finch" {
		return true
	}
	return false
}

// createStorageClass creates a storage class resource from a YAML file
// filename: name of the YAML file (without .yaml extension)
// the storage classes are defined in test/e2e/static/storage-classes
func createStorageClassForTest(filename string) {
	ginkgo.GinkgoHelper()
	path := BuildTestResourcePath(filename, "storage-classes", "")

	ginkgo.By(fmt.Sprintf("creating storage class %s from %s", filename, path))
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// deleteStorageClassForTest deletes a storage class resource from a YAML file
func deleteStorageClassForTest(storageClassName string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("deleting storage class %s", storageClassName))
	cmd := exec.Command("kubectl", "delete", "storageclass", storageClassName, "--ignore-not-found")
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// createPvcForTest creates an external PVC and waits for it to be created
func createPvcForTest(pvcFilename, groupDir, subgroupDir string) {
	ginkgo.By(fmt.Sprintf("creating external PVC %s", pvcFilename))
	path := BuildTestResourcePath(pvcFilename, groupDir, subgroupDir)
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// WaitForPVCBinding waits for a PVC to be bound
func WaitForPVCBinding(pvcName, namespace string) {
	ginkgo.GinkgoHelper()
	ginkgo.By(fmt.Sprintf("waiting for PVC %s to bind", pvcName))
	gomega.Eventually(func() error {
		phase, phaseErr := kubectlGet("pvc", pvcName, namespace, jsonPathStatusPhase)
		if phaseErr != nil {
			return phaseErr
		}
		if phase != phaseBound {
			return fmt.Errorf("PVC %s not bound, status: %s", pvcName, phase)
		}
		return nil
	}, 30*time.Second, 2*time.Second).To(gomega.Succeed())
}

// VerifyWorkspaceVolumeMount checks if a deployment has the expected volume mount name and path.
// Uses Eventually to retry because the API server can return transient errors shortly after a
// workspace becomes available (e.g. deployment status not yet propagated, brief connectivity blips).
func VerifyWorkspaceVolumeMount(workspaceName, namespace, volumeName, expectMountPath string) {
	ginkgo.GinkgoHelper()

	gomega.Eventually(func() error {
		// Step 1: resolve the deployment name from the workspace status
		deploymentName, nameErr := kubectlGet("workspace", workspaceName, namespace,
			"{.status.deploymentName}")
		if nameErr != nil {
			return fmt.Errorf("failed to get deployment name: %w", nameErr)
		}
		if deploymentName == "" {
			return fmt.Errorf("deployment name is empty for workspace %s", workspaceName)
		}

		// Step 2: fetch all volume mounts and verify the expected volume is present
		volumeMount, volumeMountErr := kubectlGet("deployment", deploymentName, namespace,
			"{.spec.template.spec.containers[0].volumeMounts}")
		if volumeMountErr != nil {
			return fmt.Errorf("failed to get volume mounts from deployment %s: %w", deploymentName, volumeMountErr)
		}
		if !strings.Contains(volumeMount, volumeName) {
			return fmt.Errorf("volume mount %s not found in deployment %s", volumeName, deploymentName)
		}

		// Step 3: verify the specific volume's mount path matches the expectation
		jsonPath := fmt.Sprintf("{.spec.template.spec.containers[0].volumeMounts[?(@.name=='%s')].mountPath}",
			volumeName)
		mountPath, mountPathErr := kubectlGet("deployment", deploymentName, namespace, jsonPath)
		if mountPathErr != nil {
			return fmt.Errorf("failed to get mount path for volume %s: %w", volumeName, mountPathErr)
		}
		if mountPath != expectMountPath {
			return fmt.Errorf("expected mount path %s but got %s", expectMountPath, mountPath)
		}
		return nil
	}, 30*time.Second, 5*time.Second).Should(gomega.Succeed(),
		fmt.Sprintf("Failed to verify volume mount %s at %s", volumeName, expectMountPath))
}

// VerifyPodCanAccessExternalVolumes verifies a workspace pod can write to and read from an
// external volume at the given mount path. It creates a test file via `kubectl exec` and
// then verifies it exists.
//
// Both the write and read steps use Eventually to retry, because `kubectl exec` can fail
// transiently with OCI runtime errors such as "procReady not received (possibly OOM-killed)"
// when the CI node is under memory pressure. Retrying only the exec (rather than the whole
// test) avoids re-creating PVCs and workspaces on each attempt.
//
// No-op when using Finch (known cgroup exec issues in Kind).
func VerifyPodCanAccessExternalVolumes(workspaceName, namespace, pvcName, mountPath string) {
	ginkgo.GinkgoHelper()

	if isUsingFinch() {
		ginkgo.By("skipping exec-based volume access test (Finch has known cgroup access issues)")
		return
	}

	ginkgo.By("waiting for pvc to bound")
	WaitForPVCBinding(pvcName, namespace)

	ginkgo.By("retrieving the pod by label")
	podSelector := fmt.Sprintf("%s=%s", WorkspaceLabelName, workspaceName)
	podName, podErr := kubectlGetByLabels("pod", podSelector, namespace, "{.items[*].metadata.name}")
	gomega.Expect(podErr).NotTo(gomega.HaveOccurred())
	gomega.Expect(podName).NotTo(gomega.BeEmpty())

	WaitForWorkspacePodToBeReady(podName, namespace)

	// Write a test file to the mounted volume. Retries handle transient OCI exec failures.
	ginkgo.By("writing to the volume at the mount path")
	filepath := fmt.Sprintf("%s/test-file-1.txt", mountPath)
	gomega.Eventually(func() error {
		writeCmd := exec.Command("kubectl", "exec", podName, "-n", namespace, "--", "touch", filepath)
		writeOutput, writeErr := utils.Run(writeCmd)
		if writeErr != nil {
			ginkgo.GinkgoWriter.Printf("writing to volume at %s failed (will retry): %v\nOutput: %s\n",
				filepath, writeErr, writeOutput)
			return fmt.Errorf("failed to write to %s: %w", filepath, writeErr)
		}
		return nil
	}, 60*time.Second, 2*time.Second).Should(gomega.Succeed(),
		fmt.Sprintf("Failed to write to %s after retries", filepath))

	// Confirm the file is visible. Also retried for the same transient exec reasons.
	ginkgo.By("verifying the file exists")
	gomega.Eventually(func() error {
		checkCmd := exec.Command("kubectl", "exec", podName, "-n", namespace, "--", "ls", filepath)
		checkOutput, checkErr := utils.Run(checkCmd)
		if checkErr != nil {
			return fmt.Errorf("failed to list file %s: %w", filepath, checkErr)
		}
		if len(strings.TrimSpace(checkOutput)) == 0 {
			return fmt.Errorf("ls output for %s was empty", filepath)
		}
		return nil
	}, 60*time.Second, 2*time.Second).Should(gomega.Succeed(),
		fmt.Sprintf("Failed to verify file %s exists after retries", filepath))
}

// VerifyPodCanAccessHomeVolume verifies pod can access home volumes
func VerifyPodCanAccessHomeVolume(workspaceName, namespace string) {
	ginkgo.GinkgoHelper()
	pvcName := controller.GeneratePVCName(workspaceName)
	VerifyPodCanAccessExternalVolumes(workspaceName, namespace, pvcName, "/home/jovyan")
}

// VerifyHomeVolumeDataPersisted verifies that a file previously written by
// VerifyPodCanAccessExternalVolumes (via VerifyPodCanAccessHomeVolume) survives a pod restart,
// confirming that the home volume PVC persists data correctly.
//
// The `kubectl exec` check is retried with Eventually for the same transient OCI runtime
// reasons described in VerifyPodCanAccessExternalVolumes.
//
// No-op when using Finch (known cgroup exec issues in Kind).
func VerifyHomeVolumeDataPersisted(workspaceName, namespace string) {
	if isUsingFinch() {
		ginkgo.By("skipping exec-based volume persistence test test (Finch has known cgroup access issues)")
		return
	}
	ginkgo.GinkgoHelper()
	pvcName := controller.GeneratePVCName(workspaceName)

	ginkgo.By("waiting for pvc to bound")
	WaitForPVCBinding(pvcName, namespace)

	ginkgo.By("retrieving the pod by label")
	podSelector := fmt.Sprintf("%s=%s", WorkspaceLabelName, workspaceName)
	podName, podErr := kubectlGetByLabels("pod", podSelector, namespace, "{.items[*].metadata.name}")
	gomega.Expect(podErr).NotTo(gomega.HaveOccurred())
	gomega.Expect(podName).NotTo(gomega.BeEmpty())

	WaitForWorkspacePodToBeReady(podName, namespace)

	// Check the file written in a previous test step still exists after pod restart.
	// Retried to handle transient OCI exec failures.
	ginkgo.By("verifying the file still exists")
	filepath := "/home/jovyan/test-file-1.txt"
	gomega.Eventually(func() error {
		checkCmd := exec.Command("kubectl", "exec", podName, "-n", namespace, "--", "ls", filepath)
		checkOutput, checkErr := utils.Run(checkCmd)
		if checkErr != nil {
			return fmt.Errorf("failed to verify persisted file %s: %w", filepath, checkErr)
		}
		if len(strings.TrimSpace(checkOutput)) == 0 {
			return fmt.Errorf("ls output for %s was empty", filepath)
		}
		return nil
	}, 60*time.Second, 2*time.Second).Should(gomega.Succeed(),
		fmt.Sprintf("Failed to verify persisted file %s after retries", filepath))
}

// volumeByName returns the pod volume with the given name, or nil.
func volumeByName(spec corev1.PodSpec, name string) *corev1.Volume {
	for i := range spec.Volumes {
		if spec.Volumes[i].Name == name {
			return &spec.Volumes[i]
		}
	}
	return nil
}

// mountByPath returns the container's volume mount at the given path, or nil.
func mountByPath(container corev1.Container, mountPath string) *corev1.VolumeMount {
	for i := range container.VolumeMounts {
		if container.VolumeMounts[i].MountPath == mountPath {
			return &container.VolumeMounts[i]
		}
	}
	return nil
}

// VerifyShmSize runs `df -k /dev/shm` in the workspace's primary container and checks that the reported
// size equals size, a Kubernetes quantity. The kubelet sizes the tmpfs to the smaller of the volume's
// sizeLimit and the pod's memory limit, and tmpfs reports that size exactly; 64Mi is the container
// default when no volume is mounted there. The pod is resolved once; only the exec is retried, for the
// transient OCI errors described on VerifyPodCanAccessExternalVolumes. No-op when using Finch (known
// cgroup exec issues in Kind).
func VerifyShmSize(workspaceName, namespace, size string) {
	ginkgo.GinkgoHelper()

	if isUsingFinch() {
		ginkgo.By("skipping exec-based /dev/shm size check (Finch has known cgroup access issues)")
		return
	}

	expected := resource.MustParse(size)
	expectedKiB := strconv.FormatInt(expected.Value()/1024, 10)
	ginkgo.By(fmt.Sprintf("verifying /dev/shm in workspace %s reports %s", workspaceName, size))
	podName, err := kubectlGetByLabels("pod", fmt.Sprintf("%s=%s", WorkspaceLabelName, workspaceName),
		namespace, "{.items[0].metadata.name}")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(podName).NotTo(gomega.BeEmpty())

	gomega.Eventually(func(g gomega.Gomega) {
		cmd := exec.Command("kubectl", "exec", podName, "-n", namespace,
			"-c", controller.PrimaryContainerName, "--", "df", "-k", "/dev/shm")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(gomega.HaveOccurred(), output)
		lines := strings.Split(strings.TrimSpace(output), "\n")
		g.Expect(lines).To(gomega.HaveLen(2), output)
		fields := strings.Fields(lines[1])
		g.Expect(len(fields)).To(gomega.BeNumerically(">=", 2), output)
		g.Expect(fields[1]).To(gomega.Equal(expectedKiB), output)
	}, 60*time.Second, 2*time.Second).Should(gomega.Succeed())
}

// sharedMemoryRoundTrip moves 128MiB through /dev/shm between a child process and its parent with
// Python's standard library, which fails on the 64MiB container default and passes with the volume.
const sharedMemoryRoundTrip = `
from multiprocessing import Process, shared_memory
SIZE = 128 * 1024 * 1024
CHUNK = 1024 * 1024
def writer(name):
    shm = shared_memory.SharedMemory(name=name)
    for off in range(0, SIZE, CHUNK):
        shm.buf[off:off + CHUNK] = b"\x5a" * CHUNK
    shm.close()
shm = shared_memory.SharedMemory(create=True, size=SIZE)
try:
    p = Process(target=writer, args=(shm.name,))
    p.start()
    p.join()
    assert p.exitcode == 0, p.exitcode
    assert bytes(shm.buf[:1]) == b"\x5a" and bytes(shm.buf[SIZE - 1:SIZE]) == b"\x5a"
finally:
    shm.close()
    shm.unlink()
print("round trip ok")
`

// VerifySharedMemoryRoundTrip runs sharedMemoryRoundTrip in the workspace's primary container. No-op when
// using Finch, as VerifyShmSize.
func VerifySharedMemoryRoundTrip(workspaceName, namespace string) {
	ginkgo.GinkgoHelper()

	if isUsingFinch() {
		ginkgo.By("skipping exec-based /dev/shm round trip (Finch has known cgroup access issues)")
		return
	}

	ginkgo.By(fmt.Sprintf("moving 128MiB through /dev/shm between processes in workspace %s", workspaceName))
	podName, err := kubectlGetByLabels("pod", fmt.Sprintf("%s=%s", WorkspaceLabelName, workspaceName),
		namespace, "{.items[0].metadata.name}")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(podName).NotTo(gomega.BeEmpty())

	gomega.Eventually(func(g gomega.Gomega) {
		cmd := exec.Command("kubectl", "exec", podName, "-n", namespace,
			"-c", controller.PrimaryContainerName, "--", "python3", "-c", sharedMemoryRoundTrip)
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(gomega.HaveOccurred(), output)
		g.Expect(output).To(gomega.ContainSubstring("round trip ok"))
	}, 60*time.Second, 2*time.Second).Should(gomega.Succeed())
}

// VerifyWorkspaceSharedMemory waits until the workspace has exactly one Running pod carrying the
// operator's /dev/shm volume at size, mounted in the primary container, then checks df inside it.
// Polling covers the Recreate roll after a spec change.
func VerifyWorkspaceSharedMemory(workspaceName, namespace, size string) {
	ginkgo.GinkgoHelper()

	gomega.Eventually(func(g gomega.Gomega) {
		pods, err := workspacePods(workspaceName, namespace)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(pods).To(gomega.HaveLen(1))
		pod := pods[0]
		g.Expect(pod.Status.Phase).To(gomega.Equal(corev1.PodRunning))
		volume := volumeByName(pod.Spec, SharedMemoryVolumeName)
		g.Expect(volume).NotTo(gomega.BeNil(), "the pod must carry the %s volume", SharedMemoryVolumeName)
		g.Expect(volume.EmptyDir).NotTo(gomega.BeNil())
		g.Expect(volume.EmptyDir.Medium).To(gomega.Equal(corev1.StorageMediumMemory))
		g.Expect(volume.EmptyDir.SizeLimit).NotTo(gomega.BeNil())
		g.Expect(volume.EmptyDir.SizeLimit.Cmp(resource.MustParse(size))).To(gomega.BeZero(),
			"sizeLimit %s, expected %s", volume.EmptyDir.SizeLimit.String(), size)
		primary := containerByName(pod.Spec, controller.PrimaryContainerName)
		g.Expect(primary).NotTo(gomega.BeNil())
		mount := mountByPath(*primary, SharedMemoryMountPath)
		g.Expect(mount).NotTo(gomega.BeNil())
		g.Expect(mount.Name).To(gomega.Equal(SharedMemoryVolumeName))
	}, 180*time.Second, 3*time.Second).Should(gomega.Succeed())

	VerifyShmSize(workspaceName, namespace, size)
}

// VerifyWorkspaceNoSharedMemory waits until the workspace's one Running pod carries no operator /dev/shm
// volume, then checks that df inside it reports the 64Mi container default.
func VerifyWorkspaceNoSharedMemory(workspaceName, namespace string) {
	ginkgo.GinkgoHelper()

	gomega.Eventually(func(g gomega.Gomega) {
		pods, err := workspacePods(workspaceName, namespace)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(pods).To(gomega.HaveLen(1))
		g.Expect(pods[0].Status.Phase).To(gomega.Equal(corev1.PodRunning))
		g.Expect(volumeByName(pods[0].Spec, SharedMemoryVolumeName)).To(gomega.BeNil())
	}, 180*time.Second, 3*time.Second).Should(gomega.Succeed())

	VerifyShmSize(workspaceName, namespace, "64Mi")
}
