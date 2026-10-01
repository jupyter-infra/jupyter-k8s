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

// Workspace shared memory (#484): the primary container of every workspace gets the operator's
// memory-backed /dev/shm volume sized to its memory limit; templates set the default and bound what
// workspaces may change, sidecars stay out of it unless an integration shares its own volume, and the
// operator restores the volume when it is removed out of band.
var _ = Describe("Workspace shared memory", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "shared-memory"
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
			deleteTemplateForTest(name)
		}
	})

	AfterEach(func() {
		for _, name := range created {
			deleteWorkspaceForTest(name, workspaceNamespace)
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
			fmt.Sprintf("{.spec.template.spec.volumes[?(@.name=='%s')].emptyDir.sizeLimit}", SharedMemoryVolumeName))
	}

	waitDeploymentSizeLimit := func(name, expected string) {
		GinkgoHelper()
		Eventually(func(g Gomega) {
			sizeLimit, err := deploymentSizeLimit(name)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(sizeLimit).To(Equal(expected))
		}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
	}

	patchWorkspace := func(name, patch string) (string, error) {
		cmd := exec.Command("kubectl", "patch", "workspace", name,
			"-n", workspaceNamespace, "--type=merge", "-p", patch)
		return utils.Run(cmd)
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
			VerifyWorkspaceSharedMemory("shm-default-workspace", workspaceNamespace, "512Mi")
		})

		It("sizes the volume to the memory request when the workspace has no limit", func() {
			create("shm-request-only-workspace")
			VerifyWorkspaceSharedMemory("shm-request-only-workspace", workspaceNamespace, "384Mi")
		})

		It("applies the default without a template and honors the workspace's own off switch", func() {
			create("shm-no-template-workspace")
			VerifyWorkspaceSharedMemory("shm-no-template-workspace", workspaceNamespace, "512Mi")

			create("shm-no-template-disabled-workspace")
			VerifyWorkspaceNoSharedMemory("shm-no-template-disabled-workspace", workspaceNamespace)
		})

		It("keeps the volume across stop and start", func() {
			create("shm-default-workspace")
			VerifyWorkspaceSharedMemory("shm-default-workspace", workspaceNamespace, "512Mi")

			UpdateWorkspaceDesiredState("shm-default-workspace", workspaceNamespace, controller.DesiredStateStopped)
			WaitForWorkspaceToReachCondition("shm-default-workspace", workspaceNamespace,
				controller.ConditionTypeStopped, ConditionTrue)

			UpdateWorkspaceDesiredState("shm-default-workspace", workspaceNamespace, controller.DesiredStateRunning)
			WaitForWorkspaceToReachCondition("shm-default-workspace", workspaceNamespace,
				controller.ConditionTypeAvailable, ConditionTrue)
			VerifyWorkspaceSharedMemory("shm-default-workspace", workspaceNamespace, "512Mi")
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
			VerifyWorkspaceSharedMemory(sidecarWorkspace, workspaceNamespace, "512Mi")
			pods, err := workspacePods(sidecarWorkspace, workspaceNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(pods).To(HaveLen(1))
			Expect(pods[0].Spec.Containers).To(HaveLen(2))
			for _, container := range pods[0].Spec.Containers {
				if container.Name != controller.PrimaryContainerName {
					Expect(mountByPath(container, SharedMemoryMountPath)).To(BeNil(),
						"sidecar %s must not mount /dev/shm", container.Name)
				}
			}
		})
	})

	Context("Template settings", func() {
		It("omits the volume when the template disables it", func() {
			create("shm-disabled-workspace")
			VerifyWorkspaceNoSharedMemory("shm-disabled-workspace", workspaceNamespace)
		})

		It("limits the volume to the template's sizeLimit", func() {
			create("shm-capped-workspace")
			VerifyWorkspaceSharedMemory("shm-capped-workspace", workspaceNamespace, "128Mi")
		})

		It("lets a workspace lower the template's size limit and rejects raising it", func() {
			create("shm-lowered-workspace")
			VerifyWorkspaceSharedMemory("shm-lowered-workspace", workspaceNamespace, "96Mi")

			VerifyCreateWorkspaceRejectedByWebhook("shm-over-cap-workspace", groupDir, "",
				"shm-over-cap-workspace", workspaceNamespace)
			VerifyCreateWorkspaceRejectedByWebhook("shm-enable-under-disabled-workspace", groupDir, "",
				"shm-enable-under-disabled-workspace", workspaceNamespace)
		})

		It("applies the template's setting to a workspace's own /dev/shm volume", func() {
			create("shm-user-volume-workspace")
			Eventually(func(g Gomega) {
				pods, err := workspacePods("shm-user-volume-workspace", workspaceNamespace)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(pods).To(HaveLen(1))
				g.Expect(volumeByName(pods[0].Spec, SharedMemoryVolumeName)).To(BeNil(),
					"the user's volume replaces the operator's")
				g.Expect(volumeByName(pods[0].Spec, "shm")).NotTo(BeNil())
			}).WithTimeout(60 * time.Second).WithPolling(3 * time.Second).Should(Succeed())
			VerifyShmSize("shm-user-volume-workspace", workspaceNamespace, "96Mi")

			VerifyCreateWorkspaceRejectedByWebhook("shm-user-volume-over-limit-workspace", groupDir, "",
				"shm-user-volume-over-limit-workspace", workspaceNamespace)
			VerifyCreateWorkspaceRejectedByWebhook("shm-user-volume-under-disabled-workspace", groupDir, "",
				"shm-user-volume-under-disabled-workspace", workspaceNamespace)
		})

		It("admits a workspace under a template that forbids secondary volumes", func() {
			create("shm-no-secondary-workspace")
			VerifyWorkspaceSharedMemory("shm-no-secondary-workspace", workspaceNamespace, "512Mi")
		})

		It("yields to an integration that mounts its own /dev/shm into the workspace and its sidecar", func() {
			// The jupyter-k8s-aws Ray integration shares one memory-backed volume between its sidecar and
			// the workspace container this way; the fixture reproduces that shape with a busybox sidecar.
			const name = "workspace-with-shm-integration"
			applyIntegrationFixture("service-cache")
			applyIntegrationFixture("shm-integration")
			DeferCleanup(func() {
				deleteWorkspaceForTest(name, workspaceNamespace)
				cmd := exec.Command("kubectl", "delete", "workspaceintegrationtemplate", "shm-integration",
					"-n", SharedNamespace, "--ignore-not-found", "--wait=true", "--timeout=30s")
				_, _ = utils.Run(cmd)
				cmd = exec.Command("kubectl", "delete", "service", "shared-cache",
					"-n", workspaceNamespace, "--ignore-not-found", "--wait=true", "--timeout=30s")
				_, _ = utils.Run(cmd)
			})
			createWorkspaceForTest(name, "integration", "")
			WaitForWorkspaceToReachCondition(name, workspaceNamespace, controller.ConditionTypeAvailable, ConditionTrue)

			pods, err := workspacePods(name, workspaceNamespace)
			Expect(err).NotTo(HaveOccurred())
			Expect(pods).To(HaveLen(1))
			podSpec := pods[0].Spec
			Expect(volumeByName(podSpec, SharedMemoryVolumeName)).To(BeNil(),
				"the operator must add no volume of its own when the integration mounts /dev/shm")
			Expect(volumeByName(podSpec, "dshm")).NotTo(BeNil())
			Expect(podSpec.Containers).To(HaveLen(2))
			for _, container := range podSpec.Containers {
				mounts := 0
				for _, mount := range container.VolumeMounts {
					if mount.MountPath == SharedMemoryMountPath {
						mounts++
						Expect(mount.Name).To(Equal("dshm"), container.Name)
					}
				}
				Expect(mounts).To(Equal(1), container.Name)
			}
			VerifyShmSize(name, workspaceNamespace, "192Mi")
		})

		It("rejects a user volume that takes the reserved name", func() {
			VerifyCreateWorkspaceRejectedByWebhook("shm-reserved-name-workspace", groupDir, "",
				"shm-reserved-name-workspace", workspaceNamespace)
		})
	})

	Context("Changes after creation", func() {
		// Two specs below edit the shared templates; restore them before the Describe-level cleanup runs.
		AfterEach(func() {
			patchTemplate(defaultTemplate, `{"spec":{"defaultSharedMemory":null,"sharedMemoryOverrides":null}}`)
			patchTemplate(cappedTemplate,
				`{"spec":{"defaultSharedMemory":{"sizeLimit":"128Mi"},"sharedMemoryOverrides":{"maxSizeLimit":"128Mi"}}}`)
		})

		It("resizes the volume when the workspace memory limit changes", func() {
			create("shm-default-workspace")
			VerifyWorkspaceSharedMemory("shm-default-workspace", workspaceNamespace, "512Mi")

			_, err := patchWorkspace("shm-default-workspace",
				`{"spec":{"resources":{"requests":{"memory":"512Mi"},"limits":{"memory":"1Gi"}}}}`)
			Expect(err).NotTo(HaveOccurred())
			waitDeploymentSizeLimit("shm-default-workspace", "1Gi")
			VerifyWorkspaceSharedMemory("shm-default-workspace", workspaceNamespace, "1Gi")
		})

		It("follows the workspace's own setting while running", func() {
			create("shm-default-workspace")
			VerifyWorkspaceSharedMemory("shm-default-workspace", workspaceNamespace, "512Mi")

			_, err := patchWorkspace("shm-default-workspace", `{"spec":{"sharedMemory":{"sizeLimit":"256Mi"}}}`)
			Expect(err).NotTo(HaveOccurred())
			waitDeploymentSizeLimit("shm-default-workspace", "256Mi")
			VerifyWorkspaceSharedMemory("shm-default-workspace", workspaceNamespace, "256Mi")

			_, err = patchWorkspace("shm-default-workspace", `{"spec":{"sharedMemory":{"enabled":false}}}`)
			Expect(err).NotTo(HaveOccurred())
			waitDeploymentSizeLimit("shm-default-workspace", "")
			VerifyWorkspaceNoSharedMemory("shm-default-workspace", workspaceNamespace)

			_, err = patchWorkspace("shm-default-workspace", `{"spec":{"sharedMemory":{"enabled":true}}}`)
			Expect(err).NotTo(HaveOccurred())
			waitDeploymentSizeLimit("shm-default-workspace", "256Mi")
			VerifyWorkspaceSharedMemory("shm-default-workspace", workspaceNamespace, "256Mi")
		})

		It("applies a size limit added to the template to new workspaces, and to existing ones at their next change", func() {
			create("shm-default-workspace")
			VerifyWorkspaceSharedMemory("shm-default-workspace", workspaceNamespace, "512Mi")

			patchTemplate(defaultTemplate,
				`{"spec":{"defaultSharedMemory":{"sizeLimit":"128Mi"},"sharedMemoryOverrides":{"maxSizeLimit":"128Mi"}}}`)

			By("verifying the running workspace keeps its volume")
			Consistently(func(g Gomega) {
				sizeLimit, err := deploymentSizeLimit("shm-default-workspace")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sizeLimit).To(Equal("512Mi"))
			}).WithTimeout(15 * time.Second).WithPolling(3 * time.Second).Should(Succeed())

			create("shm-default-workspace-b")
			VerifyWorkspaceSharedMemory("shm-default-workspace-b", workspaceNamespace, "128Mi")

			By("changing the existing workspace so it adopts the template's size limit")
			_, err := patchWorkspace("shm-default-workspace",
				`{"spec":{"resources":{"requests":{"memory":"512Mi"},"limits":{"memory":"1Gi"}}}}`)
			Expect(err).NotTo(HaveOccurred())
			waitDeploymentSizeLimit("shm-default-workspace", "128Mi")
			VerifyWorkspaceSharedMemory("shm-default-workspace", workspaceNamespace, "128Mi")
		})

		It("rejects a workspace whose copied setting a lowered template maximum no longer allows", func() {
			create("shm-capped-workspace")
			VerifyWorkspaceSharedMemory("shm-capped-workspace", workspaceNamespace, "128Mi")

			patchTemplate(cappedTemplate,
				`{"spec":{"defaultSharedMemory":{"sizeLimit":"32Mi"},"sharedMemoryOverrides":{"maxSizeLimit":"32Mi"}}}`)

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
			VerifyWorkspaceSharedMemory("shm-capped-workspace", workspaceNamespace, "32Mi")
		})

		It("restores the volume when it is removed from the Deployment out of band", func() {
			create("shm-default-workspace")
			VerifyWorkspaceSharedMemory("shm-default-workspace", workspaceNamespace, "512Mi")

			deploymentName, err := kubectlGet("workspace", "shm-default-workspace", workspaceNamespace,
				"{.status.deploymentName}")
			Expect(err).NotTo(HaveOccurred())
			generationBefore, err := deploymentGeneration(deploymentName, workspaceNamespace)
			Expect(err).NotTo(HaveOccurred())

			By("removing the volume and its mount from the Deployment")
			patch := fmt.Sprintf(`{"spec":{"template":{"spec":{"volumes":[{"name":%q,"$patch":"delete"}],`+
				`"containers":[{"name":%q,"volumeMounts":[{"mountPath":%q,"$patch":"delete"}]}]}}}}`,
				SharedMemoryVolumeName, controller.PrimaryContainerName, SharedMemoryMountPath)
			cmd := exec.Command("kubectl", "patch", "deployment", deploymentName, "-n", workspaceNamespace, "-p", patch)
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the operator restores the volume")
			Eventually(func(g Gomega) {
				sizeLimit, err := deploymentSizeLimit("shm-default-workspace")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(sizeLimit).To(Equal("512Mi"))
				// The removal and the restore are two spec changes, so the generation advanced twice.
				generation, err := deploymentGeneration(deploymentName, workspaceNamespace)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(generation).To(BeNumerically(">=", generationBefore+2))
			}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())
			WaitForWorkspaceToReachCondition("shm-default-workspace", workspaceNamespace,
				controller.ConditionTypeAvailable, ConditionTrue)
			VerifyWorkspaceSharedMemory("shm-default-workspace", workspaceNamespace, "512Mi")
		})
	})
})
