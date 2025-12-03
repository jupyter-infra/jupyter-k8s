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

// VerifyWorkspaceVolumeMount checks if deployment has correct volume mount
func VerifyWorkspaceVolumeMount(workspaceName, namespace, volumeName, expectMountPath string) {
	ginkgo.By("retrieving deployment name")
	deploymentName, nameErr := kubectlGet("workspace", workspaceName, namespace,
		"{.status.deploymentName}")

	gomega.Expect(nameErr).NotTo(gomega.HaveOccurred())
	gomega.Expect(deploymentName).NotTo(gomega.BeEmpty())

	ginkgo.By(fmt.Sprintf("retrieving deployment %s", deploymentName))
	volumeMount, volumeMountErr := kubectlGet("deployment", deploymentName, namespace,
		"{.spec.template.spec.containers[0].volumeMounts}")

	gomega.Expect(volumeMountErr).NotTo(gomega.HaveOccurred())
	gomega.Expect(strings.Contains(volumeMount, volumeName)).To(gomega.BeTrue(),
		fmt.Sprintf("volume mount %s not found in deployment %s", volumeName, deploymentName))

	ginkgo.By(fmt.Sprintf("verifying mount path %s", expectMountPath))
	jsonPath := fmt.Sprintf("{.spec.template.spec.containers[0].volumeMounts[?(@.name=='%s')].mountPath}",
		volumeName)
	mountPath, mountPathErr := kubectlGet("deployment", deploymentName, namespace, jsonPath)
	gomega.Expect(mountPathErr).NotTo(gomega.HaveOccurred())
	gomega.Expect(mountPath).To(gomega.Equal(expectMountPath))
}

// VerifyPodCanAccessExternalVolumes verifies pod can access external volumes
// note: no-op when using finch
func VerifyPodCanAccessExternalVolumes(workspaceName, namespace, pvcName, mountPath string) {
	if isUsingFinch() {
		ginkgo.By("skipping exec-based volume access test (Finch has known cgroup timing issues)")
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

	ginkgo.By("writing to the volume at the mount path")
	filepath := fmt.Sprintf("%s/test-file-1.txt", mountPath)
	writeCmd := exec.Command("kubectl", "exec", podName, "--", "touch", filepath)
	_, writeErr := utils.Run(writeCmd)
	gomega.Expect(writeErr).NotTo(gomega.HaveOccurred())

	ginkgo.By("verifying the file exists")
	checkCmd := exec.Command("kubectl", "exec", podName, "--", "ls", filepath)
	checkOutput, checkErr := utils.Run(checkCmd)
	gomega.Expect(checkErr).NotTo(gomega.HaveOccurred())
	gomega.Expect(checkOutput).ToNot(gomega.BeEmpty())
}

// VerifyPodCanAccessHomeVolume verifies pod can access home volumes
func VerifyPodCanAccessHomeVolume(workspaceName, namespace string) {
	pvcName := controller.GeneratePVCName(workspaceName)
	VerifyPodCanAccessExternalVolumes(workspaceName, namespace, pvcName, "/home/jovyan")
}

// VerifyHomeVolumeDataPersisted verifies pod can access home volume and previously
// written file is still present
// note: no-op when using finch
func VerifyHomeVolumeDataPersisted(workspaceName, namespace string) {
	if isUsingFinch() {
		ginkgo.By("skipping exec-based volume persistence test test (Finch has known cgroup timing issues)")
		return
	}
	pvcName := controller.GeneratePVCName(workspaceName)

	ginkgo.By("waiting for pvc to bound")
	WaitForPVCBinding(pvcName, namespace)

	ginkgo.By("retrieving the pod by label")
	podSelector := fmt.Sprintf("%s=%s", WorkspaceLabelName, workspaceName)
	podName, podErr := kubectlGetByLabels("pod", podSelector, namespace, "{.items[*].metadata.name}")
	gomega.Expect(podErr).NotTo(gomega.HaveOccurred())
	gomega.Expect(podName).NotTo(gomega.BeEmpty())

	WaitForWorkspacePodToBeReady(podName, namespace)

	ginkgo.By("verifying the file still exists")
	filepath := "/home/jovyan/test-file-1.txt"
	checkCmd := exec.Command("kubectl", "exec", podName, "--", "ls", filepath)
	checkOutput, checkErr := utils.Run(checkCmd)
	gomega.Expect(checkErr).NotTo(gomega.HaveOccurred())
	gomega.Expect(checkOutput).ToNot(gomega.BeEmpty())
}
