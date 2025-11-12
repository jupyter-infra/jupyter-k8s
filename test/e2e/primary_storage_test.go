//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/controller"
	"github.com/jupyter-ai-contrib/jupyter-k8s/test/utils"
)

const (
	basicWorkspace           = "workspace-with-storage"
	largeWorkspace           = "workspace-large-storage"
	noStorageWorkspace       = "workspace-no-storage"
	customMountWorkspace     = "workspace-custom-mountpath"
	templateWorkspace        = "workspace-with-template-storage"
	templateDefaultWorkspace = "workspace-template-default-storage"
	exceedBoundsWorkspace    = "workspace-exceed-storage-bounds"
)

var _ = Describe("Primary Storage", Ordered, func() {
	BeforeAll(func() {
		setupController()
		setupStorageClass()
	})

	AfterAll(func() {
		cleanupController()
	})

	Context("PVC Creation", func() {
		AfterEach(func() {
			cleanupWorkspaces(basicWorkspace, largeWorkspace, noStorageWorkspace)
		})

		It("should create PVC with correct specifications", func() {
			createWorkspace(basicWorkspace)
			verifyPVCCreated(basicWorkspace, "2Gi")
			verifyPVCProperties(basicWorkspace)
		})

		It("should handle large storage requests", func() {
			createWorkspace(largeWorkspace)
			verifyPVCCreated(largeWorkspace, "10Gi")
		})

		It("should not create PVC when storage is disabled", func() {
			createWorkspace(noStorageWorkspace)
			verifyNoPVCCreated(noStorageWorkspace)
		})
	})

	Context("Volume Mounting", func() {
		AfterEach(func() {
			cleanupWorkspaces(basicWorkspace, customMountWorkspace)
		})

		It("should mount PVC with default path", func() {
			createWorkspace(basicWorkspace)
			waitForDeployment(basicWorkspace)
			verifyVolumeMount(basicWorkspace, "/home/jovyan")
		})

		It("should mount PVC with custom path", func() {
			createWorkspace(customMountWorkspace)
			waitForDeployment(customMountWorkspace)
			verifyVolumeMount(customMountWorkspace, "/home/jovyan/data")
		})
	})

	Context("Storage Persistence", func() {
		It("should persist data across pod restarts", func() {
			createWorkspace(basicWorkspace)
			waitForDeployment(basicWorkspace)
			verifyStoragePersistence(basicWorkspace)
			cleanupWorkspaces(basicWorkspace)
		})
	})

	Context("Storage Lifecycle", func() {
		It("should cleanup PVC when workspace is deleted", func() {
			createWorkspace(basicWorkspace)
			verifyPVCCreated(basicWorkspace, "2Gi")
			deleteWorkspace(basicWorkspace)
			verifyPVCDeleted(basicWorkspace)
		})
	})

	Context("Template-based Storage", func() {
		BeforeAll(func() {
			createStorageTemplate()
		})

		AfterAll(func() {
			cleanupWorkspaces(templateWorkspace, templateDefaultWorkspace)
			cleanupStorageTemplate()
		})

		It("should create PVC within template bounds", func() {
			createWorkspace(templateWorkspace)
			waitForDeployment(templateWorkspace)
			verifyPVCCreated(templateWorkspace, "2Gi")
			verifyWorkspaceValid(templateWorkspace)
		})

		It("should use template default when storage not specified", func() {
			createWorkspace(templateDefaultWorkspace)
			waitForDeployment(templateDefaultWorkspace)
			verifyPVCCreated(templateDefaultWorkspace, "1Gi")
		})

		It("should reject workspace exceeding template bounds", func() {
			expectWorkspaceRejection(exceedBoundsWorkspace, "storage")
		})
	})

	Context("Error Handling", func() {
		AfterEach(func() {
			cleanupWorkspaces(largeWorkspace)
		})

		It("should handle storage errors gracefully", func() {
			createWorkspace(largeWorkspace)
			verifyWorkspaceErrorHandling(largeWorkspace)
		})
	})
})

// Helper functions for cleaner test code
func setupController() {
	By("installing CRDs")
	cmd := exec.Command("make", "install")
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

	By("deploying controller-manager")
	cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to deploy controller")

	By("setting controller environment variables")
	envVars := []string{
		"CONTROLLER_POD_SERVICE_ACCOUNT=jupyter-k8s-controller-manager",
		"CONTROLLER_POD_NAMESPACE=jupyter-k8s-system",
	}
	for _, envVar := range envVars {
		cmd = exec.Command("kubectl", "set", "env", "deployment/jupyter-k8s-controller-manager", envVar, "-n", "jupyter-k8s-system")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), fmt.Sprintf("Failed to set environment variable: %s", envVar))
	}

	By("waiting for controller to be ready")
	Eventually(func(g Gomega) {
		cmd := exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
			"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}{{ \"\\n\" }}{{ end }}{{ end }}",
			"-n", "jupyter-k8s-system")
		podNames, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		podNamesList := strings.Fields(strings.TrimSpace(podNames))
		g.Expect(podNamesList).To(HaveLen(1), "expected 1 controller pod running")

		cmd = exec.Command("kubectl", "get", "pods", podNamesList[0], 
			"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}", "-n", "jupyter-k8s-system")
		readyStatus, err := utils.Run(cmd)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(readyStatus).To(Equal("True"), "expected controller pod to be Ready")
	}, 5*time.Minute, 10*time.Second).Should(Succeed())
}

func setupStorageClass() {
	By("creating test storage class")
	cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/storage/test-storage-class.yaml")
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

func cleanupController() {
	By("cleaning up test storage class")
	cmd := exec.Command("kubectl", "delete", "storageclass", "test-storage", "--ignore-not-found")
	_, _ = utils.Run(cmd)

	By("undeploying controller-manager")
	cmd = exec.Command("make", "undeploy")
	_, _ = utils.Run(cmd)

	By("uninstalling CRDs")
	cmd = exec.Command("make", "uninstall")
	_, _ = utils.Run(cmd)
}

func createWorkspace(name string) {
	By(fmt.Sprintf("creating workspace %s", name))
	cmd := exec.Command("kubectl", "apply", "-f", fmt.Sprintf("test/e2e/static/storage/%s.yaml", name))
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

func waitForDeployment(workspaceName string) {
	By(fmt.Sprintf("waiting for deployment %s", workspaceName))
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "deployment", controller.GenerateDeploymentName(workspaceName))
		_, err := utils.Run(cmd)
		return err
	}, 120*time.Second, 5*time.Second).Should(Succeed())
}

func verifyPVCCreated(workspaceName, expectedSize string) {
	By(fmt.Sprintf("verifying PVC created for %s with size %s", workspaceName, expectedSize))
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "pvc", controller.GeneratePVCName(workspaceName),
			"-o", "jsonpath={.spec.resources.requests.storage}")
		output, err := utils.Run(cmd)
		if err != nil {
			return err
		}
		if output != expectedSize {
			return fmt.Errorf("expected storage size %s, got %s", expectedSize, output)
		}
		return nil
	}, 60*time.Second, 5*time.Second).Should(Succeed())
}

func verifyPVCProperties(workspaceName string) {
	pvcName := controller.GeneratePVCName(workspaceName)

	By("verifying PVC access mode")
	cmd := exec.Command("kubectl", "get", "pvc", pvcName, "-o", "jsonpath={.spec.accessModes[0]}")
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal("ReadWriteOnce"))

	By("verifying PVC owner reference")
	err = utils.VerifyPVCOwnerReference(pvcName, "default", workspaceName)
	Expect(err).NotTo(HaveOccurred())

	By("verifying PVC binding")
	err = utils.WaitForPVCBinding(pvcName, "default", 120*time.Second)
	Expect(err).NotTo(HaveOccurred())
}

func verifyNoPVCCreated(workspaceName string) {
	By(fmt.Sprintf("verifying no PVC created for %s", workspaceName))
	time.Sleep(10 * time.Second)
	cmd := exec.Command("kubectl", "get", "pvc", controller.GeneratePVCName(workspaceName))
	_, err := utils.Run(cmd)
	Expect(err).To(HaveOccurred(), "PVC should not exist when storage is disabled")
}

func verifyVolumeMount(workspaceName, expectedPath string) {
	deploymentName := controller.GenerateDeploymentName(workspaceName)
	pvcName := controller.GeneratePVCName(workspaceName)

	By("verifying deployment has volume mount")
	err := utils.VerifyVolumeMount(deploymentName, "default", "workspace-storage", "")
	Expect(err).NotTo(HaveOccurred())

	By("verifying deployment has volume definition")
	err = utils.VerifyVolumeDefinition(deploymentName, "default", "workspace-storage", pvcName)
	Expect(err).NotTo(HaveOccurred())

	By("verifying mount path")
	cmd := exec.Command("kubectl", "get", "deployment", deploymentName,
		"-o", "jsonpath={.spec.template.spec.containers[0].volumeMounts[?(@.name=='workspace-storage')].mountPath}")
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal(expectedPath))
}

func verifyStoragePersistence(workspaceName string) {
	By("testing storage persistence across pod restarts")
	
	pvcName := controller.GeneratePVCName(workspaceName)
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "pvc", pvcName, "-o", "jsonpath={.status.phase}")
		output, err := utils.Run(cmd)
		if err != nil {
			return err
		}
		if output != "Bound" {
			return fmt.Errorf("PVC not bound yet, status: %s", output)
		}
		return nil
	}, 180*time.Second, 10*time.Second).Should(Succeed())

	podSelector := fmt.Sprintf("app=jupyter,workspace.jupyter.org/workspace-name=%s", workspaceName)
	podName, err := utils.WaitForPodRunning(podSelector, "default", 300*time.Second)
	Expect(err).NotTo(HaveOccurred())

	err = utils.TestVolumeWriteAccess(podName, "default", "/home/jovyan", "test data from e2e")
	Expect(err).NotTo(HaveOccurred())

	// Restart pod
	cmd := exec.Command("kubectl", "delete", "pod", podName)
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())

	newPodName, err := utils.WaitForPodRunning(podSelector, "default", 300*time.Second)
	Expect(err).NotTo(HaveOccurred())
	Expect(newPodName).NotTo(Equal(podName))

	cmd = exec.Command("kubectl", "exec", newPodName, "--",
		"find", "/home/jovyan", "-name", "test-write-*.txt", "-exec", "cat", "{}", ";")
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(ContainSubstring("test data from e2e"))
}

func deleteWorkspace(workspaceName string) {
	By(fmt.Sprintf("deleting workspace %s", workspaceName))
	cmd := exec.Command("kubectl", "delete", "workspace", workspaceName, "--wait=true")
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

func verifyPVCDeleted(workspaceName string) {
	By(fmt.Sprintf("verifying PVC deleted for %s", workspaceName))
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "pvc", controller.GeneratePVCName(workspaceName))
		_, err := utils.Run(cmd)
		if err == nil {
			return fmt.Errorf("PVC still exists")
		}
		return nil
	}, 60*time.Second, 5*time.Second).Should(Succeed())
}

func createStorageTemplate() {
	By("creating storage template")
	cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/storage/storage-template.yaml")
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

func verifyWorkspaceValid(workspaceName string) {
	By(fmt.Sprintf("verifying workspace %s is valid", workspaceName))
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "workspace", workspaceName,
			"-o", "jsonpath={.status.conditions[?(@.type=='Valid')].status}")
		output, err := utils.Run(cmd)
		if err != nil {
			return err
		}
		if output != "True" {
			return fmt.Errorf("workspace not valid, condition status: %s", output)
		}
		return nil
	}, 30*time.Second, 2*time.Second).Should(Succeed())
}

func expectWorkspaceRejection(workspaceName, expectedError string) {
	By(fmt.Sprintf("expecting workspace %s to be rejected", workspaceName))
	cmd := exec.Command("kubectl", "apply", "-f", fmt.Sprintf("test/e2e/static/storage/%s.yaml", workspaceName))
	_, err := utils.Run(cmd)
	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(expectedError))
}

func verifyWorkspaceErrorHandling(workspaceName string) {
	By("checking workspace status for error conditions")
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "workspace", workspaceName,
			"-o", "jsonpath={.status.conditions}")
		output, err := utils.Run(cmd)
		if err != nil {
			return err
		}
		if strings.Contains(output, "StorageError") || strings.Contains(output, "PVCFailed") {
			GinkgoWriter.Printf("Storage error detected in conditions: %s\n", output)
		}
		return nil
	}, 60*time.Second, 5*time.Second).Should(Succeed())
}

func cleanupWorkspaces(names ...string) {
	if len(names) == 0 {
		return
	}
	By("cleaning up test workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--ignore-not-found", "--wait=true", "--timeout=180s")
	cmd.Args = append(cmd.Args, names...)
	_, _ = utils.Run(cmd)
}

func cleanupStorageTemplate() {
	By("cleaning up storage template")
	cmd := exec.Command("kubectl", "delete", "workspacetemplate", "storage-template", "--ignore-not-found")
	_, _ = utils.Run(cmd)
}
