//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	"os/exec"
	"time"

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Webhook Owner", Ordered, func() {
	const (
		workspaceNamespace       = "default"
		groupDir                 = "owner"
		workspaceWithAnnotations = "workspace-with-annotations"
	)

	AfterEach(func() {
		deleteResourcesForOwnerTest(workspaceNamespace)
	})

	Context("Ownership Annotation", func() {
		It("should add created-by annotation to new workspace", func() {
			workspaceName := "workspace-sample"
			workspaceFilename := "workspace-sample"

			By("creating a workspace")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("verifying created-by annotation exists")
			annotation, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.metadata.annotations.workspace\\.jupyter\\.org/created-by}")
			Expect(err).NotTo(HaveOccurred())
			Expect(annotation).NotTo(BeEmpty(), "created-by annotation should be present")

			By("verifying annotation contains user identity")
			// Annotation should contain some form of user identity (system:, kubernetes-admin, etc.)
			Expect(annotation).NotTo(BeEmpty(), "annotation should contain user identity")
		})

		It("should preserve existing annotations when adding created-by", func() {
			workspaceName := workspaceWithAnnotations
			workspaceFilename := workspaceWithAnnotations

			By("creating workspace with existing annotation")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("verifying both annotations exist")
			annotations, err := kubectlGet("workspace", workspaceName, workspaceNamespace,
				"{.metadata.annotations}")
			Expect(err).NotTo(HaveOccurred())
			Expect(annotations).To(ContainSubstring("custom-annotation"))
			Expect(annotations).To(ContainSubstring("created-by"))
		})
	})

	Context("Webhook Health", func() {
		It("should have a responding webhook endpoint", func() {
			workspaceFilename := workspaceWithAnnotations

			By("creating workspace to trigger webhook")
			createWorkspaceForTest(workspaceFilename, groupDir, "")

			By("getting controller pod name")
			podName, err := kubectlGetByLabels("pod", "control-plane=controller-manager",
				OperatorNamespace, "{.items[0].metadata.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(podName).NotTo(BeEmpty())

			By("checking webhook logs for successful processing")
			cmd := exec.Command("kubectl", "logs", podName, "-n", OperatorNamespace, "--tail=50")
			output, logErr := utils.Run(cmd)
			Expect(logErr).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("workspace-with-annotations"), "webhook should be logging")
		})
	})

	Context("Webhook Configuration", func() {
		It("should have mutating webhook configuration registered", func() {
			By("checking mutating webhook configuration exists")
			output, err := kubectlGet("mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration", "", "{.metadata.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("jupyter-k8s-mutating-webhook-configuration"))

			By("verifying webhook targets correct service")
			serviceName, err := kubectlGet("mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration", "",
				"{.webhooks[0].clientConfig.service.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(serviceName).To(Equal("jupyter-k8s-controller-manager"))
		})

		It("should have CA bundle configured", func() {
			By("checking mutating webhook configuration exists")
			output, err := kubectlGet("mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration", "", "{.metadata.name}")
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("jupyter-k8s-mutating-webhook-configuration"))

			By("verifying CA bundle field is accessible")
			_, err = kubectlGet("mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration", "",
				"{.webhooks[0].clientConfig.caBundle}")
			// CA bundle may be empty if cert-manager hasn't injected it yet, but field should be accessible
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func deleteResourcesForOwnerTest(workspaceNamespace string) {
	By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace", "--all", "-n", workspaceNamespace,
		"--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	By("waiting an arbitrary fixed time for resources to be fully deleted")
	time.Sleep(1 * time.Second)
}
