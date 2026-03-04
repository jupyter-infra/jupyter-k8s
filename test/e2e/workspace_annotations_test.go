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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

var _ = Describe("Workspace Annotations", Ordered, func() {
	const (
		workspaceNamespace = "default"
		groupDir           = "annotations"
	)

	AfterEach(func() {
		By("cleaning up test resources")
		_ = kubectlDeleteAllNamespaces("workspaces", "--ignore-not-found", "--wait=true", "--timeout=180s")
	})

	Context("Automatic Annotations", func() {
		It("should add automatic annotations to pod when no custom annotations supplied", func() {
			workspaceName := "no-annotations-workspace"
			workspaceFilename := "no-annotations-workspace"

			By("creating workspace without custom annotations")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("waiting for workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying automatic annotations exist on workspace")
			createdBy, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				fmt.Sprintf("{.metadata.annotations['%s']}", controller.AnnotationCreatedBy))
			Expect(err).NotTo(HaveOccurred())
			Expect(createdBy).NotTo(BeEmpty(), "created-by annotation should be set")

			lastUpdatedBy, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				fmt.Sprintf("{.metadata.annotations['%s']}", controller.AnnotationLastUpdatedBy))
			Expect(err).NotTo(HaveOccurred())
			Expect(lastUpdatedBy).NotTo(BeEmpty(), "last-updated-by annotation should be set")

			By("verifying automatic annotations propagated to pod")
			podName, err := kubectlGetByLabels("pod",
				fmt.Sprintf("%s=%s", controller.LabelWorkspaceName, workspaceName),
				workspaceNamespace,
				"{.items[0].metadata.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(podName).NotTo(BeEmpty())

			podCreatedBy, err := kubectlGet("pod", podName, workspaceNamespace,
				fmt.Sprintf("{.metadata.annotations['%s']}", controller.AnnotationCreatedBy))
			Expect(err).NotTo(HaveOccurred())
			Expect(podCreatedBy).To(Equal(createdBy), "pod should have same created-by annotation")

			podLastUpdatedBy, err := kubectlGet("pod", podName, workspaceNamespace,
				fmt.Sprintf("{.metadata.annotations['%s']}", controller.AnnotationLastUpdatedBy))
			Expect(err).NotTo(HaveOccurred())
			Expect(podLastUpdatedBy).To(Equal(lastUpdatedBy), "pod should have same last-updated-by annotation")
		})
	})

	Context("Custom Annotations", func() {
		It("should propagate custom annotations along with automatic annotations to pod", func() {
			workspaceName := "custom-annotations-workspace"
			workspaceFilename := "custom-annotations-workspace"

			By("creating workspace with custom annotations")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("waiting for workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying custom annotations exist on workspace")
			customAnnotation, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.metadata.annotations['custom\\.io/annotation']}")
			Expect(err).NotTo(HaveOccurred())
			Expect(customAnnotation).To(Equal("test-value"))

			prometheusAnnotation, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.metadata.annotations['prometheus\\.io/scrape']}")
			Expect(err).NotTo(HaveOccurred())
			Expect(prometheusAnnotation).To(Equal("true"))

			By("verifying automatic annotations still exist")
			createdBy, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				fmt.Sprintf("{.metadata.annotations['%s']}", controller.AnnotationCreatedBy))
			Expect(err).NotTo(HaveOccurred())
			Expect(createdBy).NotTo(BeEmpty())

			By("verifying both custom and automatic annotations propagated to pod")
			podName, err := kubectlGetByLabels("pod",
				fmt.Sprintf("%s=%s", controller.LabelWorkspaceName, workspaceName),
				workspaceNamespace,
				"{.items[0].metadata.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(podName).NotTo(BeEmpty())

			podCustomAnnotation, err := kubectlGet("pod", podName, workspaceNamespace,
				"{.metadata.annotations['custom\\.io/annotation']}")
			Expect(err).NotTo(HaveOccurred())
			Expect(podCustomAnnotation).To(Equal("test-value"))

			podPrometheusAnnotation, err := kubectlGet("pod", podName, workspaceNamespace,
				"{.metadata.annotations['prometheus\\.io/scrape']}")
			Expect(err).NotTo(HaveOccurred())
			Expect(podPrometheusAnnotation).To(Equal("true"))

			podCreatedBy, err := kubectlGet("pod", podName, workspaceNamespace,
				fmt.Sprintf("{.metadata.annotations['%s']}", controller.AnnotationCreatedBy))
			Expect(err).NotTo(HaveOccurred())
			Expect(podCreatedBy).To(Equal(createdBy))
		})

		It("should override user-supplied automatic annotations with webhook-managed values", func() {
			workspaceName := "overwrite-attempt-workspace"
			workspaceFilename := "overwrite-attempt-workspace"

			By("creating workspace with user-supplied automatic annotation")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("waiting for workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("verifying automatic annotations were overwritten by webhook")
			createdBy, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				fmt.Sprintf("{.metadata.annotations['%s']}", controller.AnnotationCreatedBy))
			Expect(err).NotTo(HaveOccurred())
			Expect(createdBy).NotTo(Equal("user-supplied-value"), "webhook should override user-supplied created-by")
			Expect(createdBy).NotTo(BeEmpty(), "created-by should be set by webhook")

			By("verifying custom annotation was preserved")
			customAnnotation, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.metadata.annotations['custom\\.io/annotation']}")
			Expect(err).NotTo(HaveOccurred())
			Expect(customAnnotation).To(Equal("test-value"), "custom annotation should be preserved")

			By("verifying annotations propagated to pod")
			podName, err := kubectlGetByLabels("pod",
				fmt.Sprintf("%s=%s", controller.LabelWorkspaceName, workspaceName),
				workspaceNamespace,
				"{.items[0].metadata.name}")
			Expect(err).NotTo(HaveOccurred())

			podCreatedBy, err := kubectlGet("pod", podName, workspaceNamespace,
				fmt.Sprintf("{.metadata.annotations['%s']}", controller.AnnotationCreatedBy))
			Expect(err).NotTo(HaveOccurred())
			Expect(podCreatedBy).To(Equal(createdBy), "pod should have webhook-managed created-by")
		})
	})

	Context("Annotation Updates", func() {
		It("should propagate updated annotations while preserving automatic annotations", func() {
			workspaceName := "update-annotations-workspace"
			workspaceFilename := "update-annotations-workspace"

			By("creating workspace with initial custom annotations")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("waiting for workspace to become available")
			WaitForWorkspaceToReachCondition(
				workspaceName,
				workspaceNamespace,
				controller.ConditionTypeAvailable,
				ConditionTrue,
			)

			By("capturing initial automatic annotations")
			createdBy, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				fmt.Sprintf("{.metadata.annotations['%s']}", controller.AnnotationCreatedBy))
			Expect(err).NotTo(HaveOccurred())
			Expect(createdBy).NotTo(BeEmpty())

			By("updating workspace with new custom annotations")
			patchCmd := exec.Command("kubectl", "patch", "workspace", workspaceName, "-n", workspaceNamespace,
				"--type=merge", "-p", `{"metadata":{"annotations":{"new-annotation":"new-value","updated-annotation":"updated"}}}`)
			_, err = utils.Run(patchCmd)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for deployment to update")
			Eventually(func() string {
				podName, _ := kubectlGetByLabels("pod",
					fmt.Sprintf("%s=%s", controller.LabelWorkspaceName, workspaceName),
					workspaceNamespace,
					"{.items[0].metadata.name}")
				if podName == "" {
					return ""
				}
				annotation, _ := kubectlGet("pod", podName, workspaceNamespace,
					"{.metadata.annotations['new-annotation']}")
				return annotation
			}, "120s", "5s").Should(Equal("new-value"), "new annotation should propagate to pod")

			By("verifying automatic annotations still honored after update")
			lastUpdatedBy, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				fmt.Sprintf("{.metadata.annotations['%s']}", controller.AnnotationLastUpdatedBy))
			Expect(err).NotTo(HaveOccurred())
			Expect(lastUpdatedBy).NotTo(BeEmpty())

			podName, err := kubectlGetByLabels("pod",
				fmt.Sprintf("%s=%s", controller.LabelWorkspaceName, workspaceName),
				workspaceNamespace,
				"{.items[0].metadata.name}")
			Expect(err).NotTo(HaveOccurred())

			podLastUpdatedBy, err := kubectlGet("pod", podName, workspaceNamespace,
				fmt.Sprintf("{.metadata.annotations['%s']}", controller.AnnotationLastUpdatedBy))
			Expect(err).NotTo(HaveOccurred())
			Expect(podLastUpdatedBy).To(Equal(lastUpdatedBy))

			podCreatedBy, err := kubectlGet("pod", podName, workspaceNamespace,
				fmt.Sprintf("{.metadata.annotations['%s']}", controller.AnnotationCreatedBy))
			Expect(err).NotTo(HaveOccurred())
			Expect(podCreatedBy).To(Equal(createdBy), "created-by should remain unchanged")
		})
	})
})
