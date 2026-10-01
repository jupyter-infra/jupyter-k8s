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
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// Workspace shared memory (#484): every workspace pod gets the operator's memory-backed /dev/shm
// volume sized to the container memory limit; templates turn it off or limit it, workspaces may only
// lower what the template set, a user volume at /dev/shm wins, sidecars stay out of it, and the
// operator restores the volume when it is removed out of band. Sizes are checked in the Deployment,
// on the pod, and with df inside the container.
var _ = Describe("Workspace shared memory", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "shared-memory"
		shmVolumeName      = "workspace-shm"
		shmMountPath       = "/dev/shm"
		kibPerMi           = int64(1024)
		containerDefault   = 64 * kibPerMi
		defaultTemplate    = "shm-template"
		disabledTemplate   = "shm-template-disabled"
		cappedTemplate     = "shm-template-capped"
		noSecondaryTmpl    = "shm-template-no-secondary"
		sidecarStrategy    = "access-strategy-with-exposed-ports"
		sidecarWorkspace   = "workspace-with-exposed-ports-access-strategy"
	)

	templates := []string{defaultTemplate, disabledTemplate, cappedTemplate, noSecondaryTmpl}
	var created []string

	BeforeAll(func() {
		for _, name := range templates {
			createTemplateForTest(name, groupDir, "")
		}
	})

	AfterAll(func() {
		for _, name := range templates {
			deleteShmTemplate(name)
		}
	})

	AfterEach(func() {
		for _, name := range created {
			deleteShmWorkspace(name, workspaceNamespace)
		}
		created = nil
	})

	create := func(name string) {
		GinkgoHelper()
		created = append(created, name)
		createWorkspaceForTest(name, groupDir, "")
		WaitForWorkspaceToReachCondition(name, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)
	}

	deploymentSizeLimit := func(name string) (string, error) {
		deploymentName, err := kubectlGet("workspace", name, workspaceNamespace, "{.status.deploymentName}")
		if err != nil {
			return "", err
		}
		return kubectlGet("deployment", deploymentName, workspaceNamespace,
			fmt.Sprintf("{.spec.template.spec.volumes[?(@.name=='%s')].emptyDir.sizeLimit}", shmVolumeName))
	}

	waitDeploymentSizeLimit := func(name, expected string) {
		GinkgoHelper()
		Eventually(func(g Gomega) {
			sizeLimit, err := deploymentSizeLimit(name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(sizeLimit).To(Equal(expected))
		}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
	}

	// waitPodShm polls until exactly one running pod carries the volume at the expected size, then
	// checks df inside it. Polling covers the Recreate roll after a spec change.
	waitPodShm := func(name, size string, expectedKiB int64) {
		GinkgoHelper()
		Eventually(func(g Gomega) {
			pods, err := workspacePods(name, workspaceNamespace)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pods).To(HaveLen(1))
			pod := pods[0]
			g.Expect(pod.Status.Phase).To(Equal(corev1.PodRunning))
			volume := volumeByName(pod.Spec, shmVolumeName)
			g.Expect(volume).NotTo(BeNil(), "the pod must carry the %s volume", shmVolumeName)
			g.Expect(volume.EmptyDir).NotTo(BeNil())
			g.Expect(volume.EmptyDir.Medium).To(Equal(corev1.StorageMediumMemory))
			g.Expect(volume.EmptyDir.SizeLimit).NotTo(BeNil())
			g.Expect(volume.EmptyDir.SizeLimit.Cmp(resource.MustParse(size))).To(BeZero(),
				"sizeLimit %s, expected %s", volume.EmptyDir.SizeLimit.String(), size)
			primary := containerByName(pod.Spec, controller.PrimaryContainerName)
			g.Expect(primary).NotTo(BeNil())
			mount := mountByPath(*primary, shmMountPath)
			g.Expect(mount).NotTo(BeNil())
			g.Expect(mount.Name).To(Equal(shmVolumeName))
		}).WithTimeout(180 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
		VerifyShmSize(name, workspaceNamespace, expectedKiB)
	}

	waitPodNoShm := func(name string) {
		GinkgoHelper()
		Eventually(func(g Gomega) {
			pods, err := workspacePods(name, workspaceNamespace)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pods).To(HaveLen(1))
			g.Expect(pods[0].Status.Phase).To(Equal(corev1.PodRunning))
			g.Expect(volumeByName(pods[0].Spec, shmVolumeName)).To(BeNil())
		}).WithTimeout(180 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
		VerifyShmSize(name, workspaceNamespace, containerDefault)
	}

	patchWorkspace := func(name, patch string) (string, error) {
		cmd := exec.Command("kubectl", "patch", "workspace", name,
			"-n", workspaceNamespace, "--type=merge", "-p", patch)
		return utils.Run(cmd)
	}

	generationOf := func(deploymentName string) (int64, error) {
		output, err := kubectlGet("deployment", deploymentName, workspaceNamespace, "{.metadata.generation}")
		if err != nil {
			return 0, err
		}
		return strconv.ParseInt(strings.TrimSpace(output), 10, 64)
	}

	patchTemplate := func(name, patch string) {
		GinkgoHelper()
		cmd := exec.Command("kubectl", "patch", "workspacetemplate", name,
			"-n", SharedNamespace, "--type=merge", "-p", patch)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
	}

	Context("Defaults", func() {
		It("gives a workspace the volume sized to its memory limit", func() {
			create("shm-default-workspace")
			waitPodShm("shm-default-workspace", "512Mi", 512*kibPerMi)
		})

		It("sizes the volume to the memory request when the workspace has no limit", func() {
			create("shm-request-only-workspace")
			waitPodShm("shm-request-only-workspace", "384Mi", 384*kibPerMi)
		})

		It("applies the default without a template and honors the workspace's own off switch", func() {
			create("shm-no-template-workspace")
			waitPodShm("shm-no-template-workspace", "512Mi", 512*kibPerMi)

			create("shm-no-template-disabled-workspace")
			waitPodNoShm("shm-no-template-disabled-workspace")
		})

		It("keeps the volume across stop and start", func() {
			create("shm-default-workspace")
			waitPodShm("shm-default-workspace", "512Mi", 512*kibPerMi)

			UpdateWorkspaceDesiredState("shm-default-workspace", workspaceNamespace, controller.DesiredStateStopped)
			WaitForWorkspaceToReachCondition("shm-default-workspace", workspaceNamespace,
				controller.ConditionTypeStopped, ConditionTrue)

			UpdateWorkspaceDesiredState("shm-default-workspace", workspaceNamespace, controller.DesiredStateRunning)
			WaitForWorkspaceToReachCondition("shm-default-workspace", workspaceNamespace,
				controller.ConditionTypeAvailable, ConditionTrue)
			waitPodShm("shm-default-workspace", "512Mi", 512*kibPerMi)
		})

		It("mounts the volume in the primary container only", func() {
			createAccessStrategyForTest(sidecarStrategy, "access-strategy", "")
			DeferCleanup(func() {
				cmd := exec.Command("kubectl", "delete", "workspaceaccessstrategy", sidecarStrategy,
					"-n", SharedNamespace, "--ignore-not-found", "--wait=true", "--timeout=60s")
				_, _ = utils.Run(cmd)
			})
			created = append(created, sidecarWorkspace)
			createWorkspaceForTest(sidecarWorkspace, "access-strategy", "")
			WaitForWorkspaceToReachCondition(sidecarWorkspace, workspaceNamespace,
				controller.ConditionTypeAvailable, ConditionTrue)

			// The workspace fixture limits memory to 512Mi; the sidecar declares none.
			waitPodShm(sidecarWorkspace, "512Mi", 512*kibPerMi)
			pods, err := workspacePods(sidecarWorkspace, workspaceNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(pods).To(HaveLen(1))
			pod := pods[0]
			Expect(pod.Spec.Containers).To(HaveLen(2))
			for _, container := range pod.Spec.Containers {
				if container.Name != controller.PrimaryContainerName {
					Expect(mountByPath(container, shmMountPath)).To(BeNil(), "sidecar %s must not mount /dev/shm", container.Name)
				}
			}
		})
	})

	Context("Template settings", func() {
		It("omits the volume when the template disables it", func() {
			create("shm-disabled-workspace")
			waitPodNoShm("shm-disabled-workspace")
		})

		It("limits the volume to the template's sizeLimit", func() {
			create("shm-capped-workspace")
			waitPodShm("shm-capped-workspace", "128Mi", 128*kibPerMi)
		})

		It("lets a workspace lower the template's size limit and rejects raising it", func() {
			create("shm-lowered-workspace")
			waitPodShm("shm-lowered-workspace", "96Mi", 96*kibPerMi)

			VerifyCreateWorkspaceRejectedByWebhook("shm-over-cap-workspace", groupDir, "",
				"shm-over-cap-workspace", workspaceNamespace)
			VerifyCreateWorkspaceRejectedByWebhook("shm-enable-under-disabled-workspace", groupDir, "",
				"shm-enable-under-disabled-workspace", workspaceNamespace)
		})

		It("admits a workspace under a template that forbids secondary volumes", func() {
			create("shm-no-secondary-workspace")
			waitPodShm("shm-no-secondary-workspace", "512Mi", 512*kibPerMi)
		})

		It("rejects a user volume that takes the reserved name", func() {
			VerifyCreateWorkspaceRejectedByWebhook("shm-reserved-name-workspace", groupDir, "",
				"shm-reserved-name-workspace", workspaceNamespace)
		})
	})

	Context("Changes after creation", func() {
		It("resizes the volume when the workspace memory limit changes", func() {
			create("shm-default-workspace")
			waitPodShm("shm-default-workspace", "512Mi", 512*kibPerMi)

			_, err := patchWorkspace("shm-default-workspace",
				`{"spec":{"resources":{"requests":{"memory":"512Mi"},"limits":{"memory":"1Gi"}}}}`)
			Expect(err).NotTo(HaveOccurred())
			waitDeploymentSizeLimit("shm-default-workspace", "1Gi")
			waitPodShm("shm-default-workspace", "1Gi", 1024*kibPerMi)
		})

		It("follows the workspace's own setting while running", func() {
			create("shm-default-workspace")
			waitPodShm("shm-default-workspace", "512Mi", 512*kibPerMi)

			_, err := patchWorkspace("shm-default-workspace", `{"spec":{"sharedMemory":{"sizeLimit":"256Mi"}}}`)
			Expect(err).NotTo(HaveOccurred())
			waitDeploymentSizeLimit("shm-default-workspace", "256Mi")
			waitPodShm("shm-default-workspace", "256Mi", 256*kibPerMi)

			_, err = patchWorkspace("shm-default-workspace", `{"spec":{"sharedMemory":{"enabled":false}}}`)
			Expect(err).NotTo(HaveOccurred())
			waitDeploymentSizeLimit("shm-default-workspace", "")
			waitPodNoShm("shm-default-workspace")

			_, err = patchWorkspace("shm-default-workspace", `{"spec":{"sharedMemory":{"enabled":true}}}`)
			Expect(err).NotTo(HaveOccurred())
			waitDeploymentSizeLimit("shm-default-workspace", "256Mi")
			waitPodShm("shm-default-workspace", "256Mi", 256*kibPerMi)
		})

		It("applies a size limit added to the template to new workspaces, and to existing ones at their next change", func() {
			create("shm-default-workspace")
			waitPodShm("shm-default-workspace", "512Mi", 512*kibPerMi)

			DeferCleanup(func() { patchTemplate(defaultTemplate, `{"spec":{"sharedMemory":null}}`) })
			patchTemplate(defaultTemplate, `{"spec":{"sharedMemory":{"sizeLimit":"128Mi"}}}`)

			By("verifying the running workspace keeps its volume")
			Consistently(func(g Gomega) {
				sizeLimit, err := deploymentSizeLimit("shm-default-workspace")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sizeLimit).To(Equal("512Mi"))
			}).WithTimeout(15 * time.Second).WithPolling(3 * time.Second).Should(Succeed())

			create("shm-default-workspace-b")
			waitPodShm("shm-default-workspace-b", "128Mi", 128*kibPerMi)

			By("changing the existing workspace so it adopts the template's size limit")
			_, err := patchWorkspace("shm-default-workspace",
				`{"spec":{"resources":{"requests":{"memory":"512Mi"},"limits":{"memory":"1Gi"}}}}`)
			Expect(err).NotTo(HaveOccurred())
			waitDeploymentSizeLimit("shm-default-workspace", "128Mi")
			waitPodShm("shm-default-workspace", "128Mi", 128*kibPerMi)
		})

		It("rejects a workspace whose copied setting a tightened template no longer allows", func() {
			create("shm-capped-workspace")
			waitPodShm("shm-capped-workspace", "128Mi", 128*kibPerMi)

			DeferCleanup(func() { patchTemplate(cappedTemplate, `{"spec":{"sharedMemory":{"sizeLimit":"128Mi"}}}`) })
			patchTemplate(cappedTemplate, `{"spec":{"sharedMemory":{"sizeLimit":"32Mi"}}}`)

			By("verifying the running workspace keeps its volume")
			Consistently(func(g Gomega) {
				sizeLimit, err := deploymentSizeLimit("shm-capped-workspace")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sizeLimit).To(Equal("128Mi"))
			}).WithTimeout(15 * time.Second).WithPolling(3 * time.Second).Should(Succeed())

			By("verifying a change that keeps the old size limit is rejected")
			output, err := patchWorkspace("shm-capped-workspace",
				`{"spec":{"resources":{"requests":{"memory":"512Mi"},"limits":{"memory":"1Gi"}}}}`)
			Expect(err).To(HaveOccurred(), output)
			Expect(output).To(ContainSubstring("exceeds"))

			By("verifying a stop is still accepted")
			UpdateWorkspaceDesiredState("shm-capped-workspace", workspaceNamespace, controller.DesiredStateStopped)
			WaitForWorkspaceToReachCondition("shm-capped-workspace", workspaceNamespace,
				controller.ConditionTypeStopped, ConditionTrue)

			By("verifying the workspace is accepted once it fits the new size limit")
			_, err = patchWorkspace("shm-capped-workspace",
				fmt.Sprintf(`{"spec":{"desiredStatus":%q,"sharedMemory":{"sizeLimit":"32Mi"}}}`, controller.DesiredStateRunning))
			Expect(err).NotTo(HaveOccurred())
			WaitForWorkspaceToReachCondition("shm-capped-workspace", workspaceNamespace,
				controller.ConditionTypeAvailable, ConditionTrue)
			waitPodShm("shm-capped-workspace", "32Mi", 32*kibPerMi)
		})

		It("restores the volume when it is removed from the Deployment out of band", func() {
			create("shm-default-workspace")
			waitPodShm("shm-default-workspace", "512Mi", 512*kibPerMi)

			deploymentName, err := kubectlGet("workspace", "shm-default-workspace", workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			generationBefore, err := generationOf(deploymentName)
			Expect(err).NotTo(HaveOccurred())

			By("removing the volume and its mount from the Deployment")
			patch := fmt.Sprintf(`{"spec":{"template":{"spec":{"volumes":[{"name":%q,"$patch":"delete"}],`+
				`"containers":[{"name":%q,"volumeMounts":[{"mountPath":%q,"$patch":"delete"}]}]}}}}`,
				shmVolumeName, controller.PrimaryContainerName, shmMountPath)
			cmd := exec.Command("kubectl", "patch", "deployment", deploymentName, "-n", workspaceNamespace, "-p", patch)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the operator restores the volume")
			Eventually(func(g Gomega) {
				sizeLimit, err := deploymentSizeLimit("shm-default-workspace")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sizeLimit).To(Equal("512Mi"))
				// The removal and the restore are two spec changes, so the generation advanced twice.
				generation, err := generationOf(deploymentName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(generation).To(BeNumerically(">=", generationBefore+2))
			}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
			WaitForWorkspaceToReachCondition("shm-default-workspace", workspaceNamespace,
				controller.ConditionTypeAvailable, ConditionTrue)
			waitPodShm("shm-default-workspace", "512Mi", 512*kibPerMi)
		})
	})
})

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
func mountByPath(container corev1.Container, path string) *corev1.VolumeMount {
	for i := range container.VolumeMounts {
		if container.VolumeMounts[i].MountPath == path {
			return &container.VolumeMounts[i]
		}
	}
	return nil
}

// deleteShmWorkspace removes one workspace by name so it can never delete unrelated objects
// sharing the "default" namespace.
func deleteShmWorkspace(workspaceName, namespace string) {
	GinkgoHelper()
	cmd := exec.Command("kubectl", "delete", "workspace", workspaceName,
		"-n", namespace, "--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)
}

// deleteShmTemplate removes one of the templates the suite shares across its specs.
func deleteShmTemplate(templateName string) {
	GinkgoHelper()
	cmd := exec.Command("kubectl", "delete", "workspacetemplate", templateName,
		"-n", SharedNamespace, "--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)
}
