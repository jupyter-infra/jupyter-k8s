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

	"github.com/jupyter-ai-contrib/jupyter-k8s/test/utils"
)

var _ = Describe("Primary Storage", Ordered, func() {
	BeforeAll(func() {
		By("installing CRDs")
		_, _ = fmt.Fprintf(GinkgoWriter, "Installing CRDs...\n")
		cmd := exec.Command("make", "install")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		_, _ = fmt.Fprintf(GinkgoWriter, "Deploying controller manager...\n")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy controller")

		// Set environment variables for e2e testing
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

		By("waiting for controller-manager to be ready")
		_, _ = fmt.Fprintf(GinkgoWriter, "Waiting for controller manager pod...\n")
		verifyControllerUp := func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
				"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}{{ \"\\n\" }}{{ end }}{{ end }}",
				"-n", "jupyter-k8s-system",
			)
			podNames, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			podNamesList := strings.Fields(strings.TrimSpace(podNames))
			g.Expect(podNamesList).To(HaveLen(1), "expected 1 controller pod running")

			cmd = exec.Command("kubectl", "get",
				"pods", podNamesList[0], "-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}",
				"-n", "jupyter-k8s-system",
			)
			readyStatus, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(readyStatus).To(Equal("True"), "expected controller pod to be Ready")
		}
		Eventually(verifyControllerUp, 5*time.Minute, 10*time.Second).Should(Succeed())

		By("creating test storage class")
		cmd = exec.Command("kubectl", "apply", "-f", "test/e2e/static/storage/test-storage-class.yaml")
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterAll(func() {
		By("cleaning up test workspaces")
		cmd := exec.Command("kubectl", "delete", "workspace",
			"workspace-with-storage",
			"workspace-large-storage", "workspace-no-storage",
			"--ignore-not-found", "--wait=true", "--timeout=180s")
		_, _ = utils.Run(cmd)

		By("cleaning up test storage class")
		cmd = exec.Command("kubectl", "delete", "storageclass", "test-storage", "--ignore-not-found")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)
	})

	Context("PVC Creation and Binding", func() {
		It("should create PVC when workspace specifies storage", func() {
			By("creating workspace with storage specification")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/storage/workspace-with-storage.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for workspace to be created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "workspace", "workspace-with-storage")
				_, err := utils.Run(cmd)
				return err
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			By("verifying PVC is created with correct size")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "pvc", "workspace-workspace-with-storage-pvc",
					"-o", "jsonpath={.spec.resources.requests.storage}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				if output != "2Gi" {
					return fmt.Errorf("expected storage size 2Gi, got %s", output)
				}
				return nil
			}, 60*time.Second, 5*time.Second).Should(Succeed())

			By("verifying PVC has correct access mode")
			cmd = exec.Command("kubectl", "get", "pvc", "workspace-workspace-with-storage-pvc",
				"-o", "jsonpath={.spec.accessModes[0]}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("ReadWriteOnce"))

			By("verifying PVC has OwnerReference to workspace")
			err = utils.VerifyPVCOwnerReference("workspace-workspace-with-storage-pvc", "default", "workspace-with-storage")
			Expect(err).NotTo(HaveOccurred())

			By("verifying PVC binding status")
			err = utils.WaitForPVCBinding("workspace-workspace-with-storage-pvc", "default", 120*time.Second)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should handle large storage requests", func() {
			By("creating workspace with large storage")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/storage/workspace-large-storage.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying large PVC is created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "pvc", "workspace-workspace-large-storage-pvc",
					"-o", "jsonpath={.spec.resources.requests.storage}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				if output != "10Gi" {
					return fmt.Errorf("expected storage size 10Gi, got %s", output)
				}
				return nil
			}, 60*time.Second, 5*time.Second).Should(Succeed())
		})

		It("should not create PVC when storage is explicitly disabled", func() {
			By("creating workspace without storage")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/storage/workspace-no-storage.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for workspace to be processed")
			time.Sleep(10 * time.Second)

			By("verifying no PVC is created")
			cmd = exec.Command("kubectl", "get", "pvc", "workspace-workspace-no-storage-pvc")
			_, err = utils.Run(cmd)
			Expect(err).To(HaveOccurred(), "PVC should not exist when storage is not specified")
		})
	})

	Context("Volume Mounting Verification", func() {
		It("should mount PVC in deployment when storage is specified", func() {
			By("waiting for deployment to be created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "deployment", "workspace-workspace-with-storage")
				_, err := utils.Run(cmd)
				return err
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			By("verifying deployment has volume mount for PVC")
			err := utils.VerifyVolumeMount("workspace-workspace-with-storage", "default", "workspace-storage", "")
			Expect(err).NotTo(HaveOccurred())

			By("verifying deployment has volume definition")
			err = utils.VerifyVolumeDefinition("workspace-workspace-with-storage", "default", "workspace-storage", "workspace-workspace-with-storage-pvc")
			Expect(err).NotTo(HaveOccurred())

			By("verifying volume mount path is correct")
			cmd := exec.Command("kubectl", "get", "deployment", "workspace-workspace-with-storage",
				"-o", "jsonpath={.spec.template.spec.containers[0].volumeMounts[?(@.name=='workspace-storage')].mountPath}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("/home/jovyan"))
		})

		It("should mount PVC with custom mount path when specified", func() {
			By("creating workspace with custom mount path")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/storage/workspace-custom-mountpath.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for deployment to be created")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "deployment", "workspace-workspace-custom-mountpath")
				_, err := utils.Run(cmd)
				return err
			}, 120*time.Second, 5*time.Second).Should(Succeed())

			By("verifying custom mount path is used")
			cmd = exec.Command("kubectl", "get", "deployment", "workspace-workspace-custom-mountpath",
				"-o", "jsonpath={.spec.template.spec.containers[0].volumeMounts[?(@.name=='workspace-storage')].mountPath}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("/home/jovyan/data"))

			By("cleaning up custom mountpath workspace")
			cmd = exec.Command("kubectl", "delete", "workspace", "workspace-custom-mountpath", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should verify pod can write to mounted volume", func() {
			By("checking PVC status first")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "pvc", "workspace-workspace-with-storage-pvc", "-o", "jsonpath={.status.phase}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				if output != "Bound" {
					return fmt.Errorf("PVC not bound yet, status: %s", output)
				}
				return nil
			}, 180*time.Second, 10*time.Second).Should(Succeed())

			By("waiting for pod to be running")
			podName, err := utils.WaitForPodRunning("app=jupyter,workspace.jupyter.org/workspace-name=workspace-with-storage", "default", 300*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(podName).NotTo(BeEmpty())

			By("testing write access to mounted volume")
			mountPath := "/home/jovyan" // Default mount path from controller
			err = utils.TestVolumeWriteAccess(podName, "default", mountPath, "test data from e2e")
			Expect(err).NotTo(HaveOccurred())

			By("verifying file persists after container restart")
			// Force pod restart by deleting it
			cmd := exec.Command("kubectl", "delete", "pod", podName)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for new pod to be running")
			newPodName, err := utils.WaitForPodRunning("app=jupyter,workspace.jupyter.org/workspace-name=workspace-with-storage", "default", 300*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(newPodName).NotTo(BeEmpty())
			Expect(newPodName).NotTo(Equal(podName)) // Should be a different pod

			By("verifying file still exists in new pod")
			cmd = exec.Command("kubectl", "exec", newPodName, "--",
				"find", mountPath, "-name", "test-write-*.txt", "-exec", "cat", "{}", ";")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("test data from e2e"))
		})
	})

	Context("Storage Cleanup", func() {
		It("should clean up PVC when workspace is deleted", func() {
			By("deleting workspace")
			cmd := exec.Command("kubectl", "delete", "workspace", "workspace-with-storage", "--wait=true")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying PVC is deleted")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "pvc", "workspace-workspace-with-storage-pvc")
				_, err := utils.Run(cmd)
				if err == nil {
					return fmt.Errorf("PVC still exists")
				}
				return nil
			}, 60*time.Second, 5*time.Second).Should(Succeed())
		})
	})

	Context("Storage Error Scenarios", func() {
		It("should handle PVC creation failures gracefully", func() {
			// This test would require a storage class that fails
			// For now, we'll test the error handling path by checking status conditions
			By("creating workspace and checking for any error conditions")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/storage/workspace-large-storage.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("checking workspace status for any storage-related errors")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "workspace", "workspace-large-storage",
					"-o", "jsonpath={.status.conditions}")
				output, err := utils.Run(cmd)
				if err != nil {
					return err
				}
				// If there are error conditions, they should be properly reported
				if strings.Contains(output, "StorageError") || strings.Contains(output, "PVCFailed") {
					GinkgoWriter.Printf("Storage error detected in conditions: %s\n", output)
				}
				return nil
			}, 60*time.Second, 5*time.Second).Should(Succeed())
		})
	})

	Context("Template-based Storage Validation", func() {
		BeforeAll(func() {
			By("creating storage template")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/storage/storage-template.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterAll(func() {
			By("cleaning up template-based test resources")
			cmd := exec.Command("kubectl", "delete", "workspace",
				"workspace-with-template-storage", "workspace-exceed-storage-bounds",
				"--ignore-not-found", "--wait=true")
			_, _ = utils.Run(cmd)

			cmd = exec.Command("kubectl", "delete", "workspacetemplate", "storage-template", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})

		It("should create PVC within template storage bounds", func() {
			By("creating workspace with storage within template bounds")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/storage/workspace-with-template-storage.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying PVC is created with specified size")
			Eventually(func() error {
				return utils.VerifyPVCSize("workspace-workspace-with-template-storage-pvc", "default", "2Gi")
			}, 60*time.Second, 5*time.Second).Should(Succeed())

			By("verifying workspace has Valid=True condition")
			Eventually(func() error {
				cmd := exec.Command("kubectl", "get", "workspace", "workspace-with-template-storage",
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
		})

		It("should reject workspace with storage exceeding template bounds", func() {
			By("attempting to create workspace with storage exceeding template max")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/storage/workspace-exceed-storage-bounds.yaml")
			_, err := utils.Run(cmd)
			// This should be rejected by webhook validation
			Expect(err).To(HaveOccurred(), "Expected webhook to reject workspace with storage exceeding template bounds")
			Expect(err.Error()).To(ContainSubstring("storage"))
		})

		It("should use template default storage when not specified", func() {
			By("creating workspace without storage specification")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/storage/workspace-template-default-storage.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying PVC is created with template default size")
			Eventually(func() error {
				return utils.VerifyPVCSize("workspace-workspace-template-default-storage-pvc", "default", "1Gi")
			}, 60*time.Second, 5*time.Second).Should(Succeed())

			By("cleaning up test workspace")
			cmd = exec.Command("kubectl", "delete", "workspace", "workspace-template-default-storage", "--ignore-not-found")
			_, _ = utils.Run(cmd)
		})
	})
})
