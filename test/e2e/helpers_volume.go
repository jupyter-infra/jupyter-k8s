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
	"strings"
	"time"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
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
		phase, phaseErr := kubectlGet("pvc", pvcName, namespace, "{.status.phase}")
		if phaseErr != nil {
			return phaseErr
		}
		if phase != "Bound" {
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
	}, 30*time.Second, 5*time.Second).Should(gomega.Succeed(),
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
	}, 30*time.Second, 5*time.Second).Should(gomega.Succeed(),
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
	}, 30*time.Second, 5*time.Second).Should(gomega.Succeed(),
		fmt.Sprintf("Failed to verify persisted file %s after retries", filepath))
}
