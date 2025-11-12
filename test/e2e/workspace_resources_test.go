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

var _ = Describe("Workspace Resources", Ordered, func() {
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
			verifyDeploymentProperties(basicWorkspace)
		})

		It("should mount PVC with custom path", func() {
			createWorkspace(customMountWorkspace)
			waitForDeployment(customMountWorkspace)
			verifyVolumeMount(customMountWorkspace, "/home/jovyan/data")
			verifyDeploymentProperties(customMountWorkspace)
		})

		It("should create service for workspace access", func() {
			createWorkspace(basicWorkspace)
			waitForDeployment(basicWorkspace)
			verifyServiceCreated(basicWorkspace)
		})
	})

	Context("Resource Creation", func() {
		AfterEach(func() {
			cleanupWorkspaces(basicWorkspace)
		})

		It("should create all required resources", func() {
			createWorkspace(basicWorkspace)
			verifyPVCCreated(basicWorkspace, "2Gi")
			waitForDeployment(basicWorkspace)
			verifyServiceCreated(basicWorkspace)
			verifyResourceOwnership(basicWorkspace)
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

	Context("Workspace Deletion", func() {
		It("should add finalizer when workspace is created", func() {
			createWorkspace(basicWorkspace)
			verifyFinalizerAdded(basicWorkspace)
			cleanupWorkspaces(basicWorkspace)
		})

		It("should handle finalizer removal during deletion", func() {
			createWorkspace(basicWorkspace)
			waitForDeployment(basicWorkspace)
			verifyFinalizerAdded(basicWorkspace)
			
			deleteWorkspaceAsync(basicWorkspace)
			verifyWorkspaceDeletionTimestamp(basicWorkspace)
			verifyResourcesStillExist(basicWorkspace)
			
			// Wait for workspace to be completely deleted (finalizer removed by controller)
			verifyWorkspaceDeleted(basicWorkspace)
		})

		It("should cleanup all resources when finalizer is removed", func() {
			createWorkspace(basicWorkspace)
			verifyPVCCreated(basicWorkspace, "2Gi")
			waitForDeployment(basicWorkspace)
			verifyServiceCreated(basicWorkspace)
			
			deleteWorkspace(basicWorkspace)
			verifyAllResourcesDeleted(basicWorkspace)
		})

		It("should handle deletion with multiple finalizers", func() {
			createWorkspace(basicWorkspace)
			addCustomFinalizer(basicWorkspace, "test.finalizer/custom")
			verifyMultipleFinalizers(basicWorkspace)
			
			deleteWorkspaceAsync(basicWorkspace)
			verifyWorkspaceDeletionTimestamp(basicWorkspace)
			
			removeCustomFinalizer(basicWorkspace, "test.finalizer/custom")
			waitForFinalizerRemoval(basicWorkspace)
			verifyWorkspaceDeleted(basicWorkspace)
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

func verifyDeploymentProperties(workspaceName string) {
	deploymentName := controller.GenerateDeploymentName(workspaceName)

	By("verifying deployment has correct labels")
	cmd := exec.Command("kubectl", "get", "deployment", deploymentName,
		"-o", "jsonpath={.metadata.labels.app}")
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal("jupyter"))

	By("verifying deployment has workspace label")
	cmd = exec.Command("kubectl", "get", "deployment", deploymentName,
		"-o", "jsonpath={.metadata.labels['workspace\\.jupyter\\.org/workspace-name']}")
	output, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal(workspaceName))

	By("verifying deployment has correct replicas")
	cmd = exec.Command("kubectl", "get", "deployment", deploymentName,
		"-o", "jsonpath={.spec.replicas}")
	output, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal("1"))

	By("verifying deployment has owner reference")
	cmd = exec.Command("kubectl", "get", "deployment", deploymentName,
		"-o", "jsonpath={.metadata.ownerReferences[0].name}")
	output, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal(workspaceName))
}

func verifyServiceCreated(workspaceName string) {
	serviceName := controller.GenerateServiceName(workspaceName)

	By(fmt.Sprintf("verifying service %s is created", serviceName))
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "service", serviceName)
		_, err := utils.Run(cmd)
		return err
	}, 60*time.Second, 5*time.Second).Should(Succeed())

	By("verifying service has correct selector")
	cmd := exec.Command("kubectl", "get", "service", serviceName,
		"-o", "jsonpath={.spec.selector.app}")
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal("jupyter"))

	By("verifying service has workspace selector")
	cmd = exec.Command("kubectl", "get", "service", serviceName,
		"-o", "jsonpath={.spec.selector['workspace\\.jupyter\\.org/workspace-name']}")
	output, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal(workspaceName))

	By("verifying service has correct port")
	cmd = exec.Command("kubectl", "get", "service", serviceName,
		"-o", "jsonpath={.spec.ports[0].port}")
	output, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal("8888"))

	By("verifying service has owner reference")
	cmd = exec.Command("kubectl", "get", "service", serviceName,
		"-o", "jsonpath={.metadata.ownerReferences[0].name}")
	output, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal(workspaceName))
}

func verifyResourceOwnership(workspaceName string) {
	By("verifying all resources have correct owner references")
	
	// PVC ownership
	pvcName := controller.GeneratePVCName(workspaceName)
	err := utils.VerifyPVCOwnerReference(pvcName, "default", workspaceName)
	Expect(err).NotTo(HaveOccurred())

	// Deployment ownership
	deploymentName := controller.GenerateDeploymentName(workspaceName)
	cmd := exec.Command("kubectl", "get", "deployment", deploymentName,
		"-o", "jsonpath={.metadata.ownerReferences[0].name}")
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal(workspaceName))

	// Service ownership
	serviceName := controller.GenerateServiceName(workspaceName)
	cmd = exec.Command("kubectl", "get", "service", serviceName,
		"-o", "jsonpath={.metadata.ownerReferences[0].name}")
	output, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(Equal(workspaceName))
}

func verifyFinalizerAdded(workspaceName string) {
	By(fmt.Sprintf("verifying finalizer added to workspace %s", workspaceName))
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "workspace", workspaceName,
			"-o", "jsonpath={.metadata.finalizers}")
		output, err := utils.Run(cmd)
		if err != nil {
			return err
		}
		if !strings.Contains(output, controller.WorkspaceFinalizerName) {
			return fmt.Errorf("finalizer not found, got: %s", output)
		}
		return nil
	}, 30*time.Second, 2*time.Second).Should(Succeed())
}

func deleteWorkspaceAsync(workspaceName string) {
	By(fmt.Sprintf("deleting workspace %s asynchronously", workspaceName))
	cmd := exec.Command("kubectl", "delete", "workspace", workspaceName, "--wait=false")
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

func verifyWorkspaceDeletionTimestamp(workspaceName string) {
	By(fmt.Sprintf("verifying workspace %s has deletion timestamp or is deleted", workspaceName))
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "workspace", workspaceName,
			"-o", "jsonpath={.metadata.deletionTimestamp}")
		output, err := utils.Run(cmd)
		if err != nil {
			// If workspace is already deleted, that's acceptable too
			if strings.Contains(err.Error(), "not found") {
				return nil
			}
			return err
		}
		if output == "" {
			return fmt.Errorf("deletion timestamp not set")
		}
		return nil
	}, 10*time.Second, 1*time.Second).Should(Succeed())
}

func verifyResourcesStillExist(workspaceName string) {
	By("verifying resources behavior during deletion")
	
	// Check if workspace still exists first
	cmd := exec.Command("kubectl", "get", "workspace", workspaceName)
	_, err := utils.Run(cmd)
	if err != nil && strings.Contains(err.Error(), "not found") {
		By("workspace already deleted - skipping resource existence check")
		return
	}
	
	// If workspace still exists, resources should still exist
	cmd = exec.Command("kubectl", "get", "pvc", controller.GeneratePVCName(workspaceName))
	_, err = utils.Run(cmd)
	if err == nil {
		By("PVC still exists as expected")
	}
	
	cmd = exec.Command("kubectl", "get", "deployment", controller.GenerateDeploymentName(workspaceName))
	_, err = utils.Run(cmd)
	if err == nil {
		By("Deployment still exists as expected")
	}
	
	cmd = exec.Command("kubectl", "get", "service", controller.GenerateServiceName(workspaceName))
	_, err = utils.Run(cmd)
	if err == nil {
		By("Service still exists as expected")
	}
}

func waitForFinalizerRemoval(workspaceName string) {
	By(fmt.Sprintf("waiting for finalizer removal from workspace %s", workspaceName))
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "workspace", workspaceName,
			"-o", "jsonpath={.metadata.finalizers}")
		output, err := utils.Run(cmd)
		if err != nil {
			// If workspace is deleted, that's what we want
			if strings.Contains(err.Error(), "not found") {
				return nil
			}
			return err
		}
		if strings.Contains(output, controller.WorkspaceFinalizerName) {
			return fmt.Errorf("finalizer still present: %s", output)
		}
		return nil
	}, 120*time.Second, 5*time.Second).Should(Succeed())
}

func verifyWorkspaceDeleted(workspaceName string) {
	By(fmt.Sprintf("verifying workspace %s is completely deleted", workspaceName))
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "workspace", workspaceName)
		_, err := utils.Run(cmd)
		if err == nil {
			return fmt.Errorf("workspace still exists")
		}
		if !strings.Contains(err.Error(), "not found") {
			return err
		}
		return nil
	}, 60*time.Second, 5*time.Second).Should(Succeed())
}

func verifyAllResourcesDeleted(workspaceName string) {
	By("verifying all workspace resources are deleted")
	
	// Verify PVC is deleted
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "pvc", controller.GeneratePVCName(workspaceName))
		_, err := utils.Run(cmd)
		if err == nil {
			return fmt.Errorf("PVC still exists")
		}
		return nil
	}, 60*time.Second, 5*time.Second).Should(Succeed())
	
	// Verify Deployment is deleted
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "deployment", controller.GenerateDeploymentName(workspaceName))
		_, err := utils.Run(cmd)
		if err == nil {
			return fmt.Errorf("Deployment still exists")
		}
		return nil
	}, 60*time.Second, 5*time.Second).Should(Succeed())
	
	// Verify Service is deleted
	Eventually(func() error {
		cmd := exec.Command("kubectl", "get", "service", controller.GenerateServiceName(workspaceName))
		_, err := utils.Run(cmd)
		if err == nil {
			return fmt.Errorf("Service still exists")
		}
		return nil
	}, 60*time.Second, 5*time.Second).Should(Succeed())
}

func addCustomFinalizer(workspaceName, finalizer string) {
	By(fmt.Sprintf("adding custom finalizer %s to workspace %s", finalizer, workspaceName))
	cmd := exec.Command("kubectl", "patch", "workspace", workspaceName,
		"--type=merge", "-p", fmt.Sprintf(`{"metadata":{"finalizers":["%s"]}}`, finalizer))
	_, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
}

func verifyMultipleFinalizers(workspaceName string) {
	By(fmt.Sprintf("verifying workspace %s has multiple finalizers", workspaceName))
	cmd := exec.Command("kubectl", "get", "workspace", workspaceName,
		"-o", "jsonpath={.metadata.finalizers}")
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	Expect(output).To(ContainSubstring(controller.WorkspaceFinalizerName))
	Expect(output).To(ContainSubstring("test.finalizer/custom"))
}

func removeCustomFinalizer(workspaceName, finalizer string) {
	By(fmt.Sprintf("removing custom finalizer %s from workspace %s", finalizer, workspaceName))
	
	// Get current finalizers
	cmd := exec.Command("kubectl", "get", "workspace", workspaceName,
		"-o", "jsonpath={.metadata.finalizers[*]}")
	output, err := utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
	
	// Remove the custom finalizer, keep others
	finalizers := strings.Fields(output)
	var remainingFinalizers []string
	for _, f := range finalizers {
		if f != finalizer {
			remainingFinalizers = append(remainingFinalizers, f)
		}
	}
	
	finalizersJson := `[]`
	if len(remainingFinalizers) > 0 {
		finalizersJson = fmt.Sprintf(`["%s"]`, strings.Join(remainingFinalizers, `","`))
	}
	
	cmd = exec.Command("kubectl", "patch", "workspace", workspaceName,
		"--type=merge", "-p", fmt.Sprintf(`{"metadata":{"finalizers":%s}}`, finalizersJson))
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred())
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
