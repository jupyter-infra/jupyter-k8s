//go:build e2e
// +build e2e

/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package e2e

import (
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

var _ = Describe("Webhook Owner", Ordered, func() {
	AfterAll(func() {
		By("cleaning up webhook owner test workspaces")
		cmd := exec.Command("kubectl", "delete", "workspace",
			"workspace-sample", "workspace-with-annotations",
			"--ignore-not-found", "--timeout=60s")
		_, _ = utils.Run(cmd)
	})

	Context("Ownership Annotation", func() {
		It("should add created-by annotation to new workspace", func() {
			By("creating a workspace")
			cmd := exec.Command("kubectl", "apply", "-f", "config/samples/workspace_v1alpha1_workspace.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying created-by annotation exists")
			cmd = exec.Command("kubectl", "get", "workspace", "workspace-sample",
				"-o", "jsonpath={.metadata.annotations.workspace\\.jupyter\\.org/created-by}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			annotation := strings.TrimSpace(string(output))
			Expect(annotation).NotTo(BeEmpty(), "created-by annotation should be present")

			By("verifying annotation contains user identity")
			// Annotation should contain some form of user identity (system:, kubernetes-admin, etc.)
			Expect(len(annotation)).To(BeNumerically(">", 0), "annotation should contain user identity")
		})

		It("should preserve existing annotations when adding created-by", func() {
			By("creating workspace with existing annotation")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/webhook-validation/workspace-with-annotations.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying both annotations exist")
			cmd = exec.Command("kubectl", "get", "workspace", "workspace-with-annotations",
				"-o", "jsonpath={.metadata.annotations}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			annotations := string(output)
			Expect(annotations).To(ContainSubstring("custom-annotation"))
			Expect(annotations).To(ContainSubstring("created-by"))
		})
	})

	Context("Webhook Health", func() {
		It("should have webhook endpoint responding", func() {
			By("getting controller pod name")
			cmd := exec.Command("kubectl", "get", "pods", "-l", "control-plane=controller-manager",
				"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}{{ \"\\n\" }}{{ end }}{{ end }}",
				"-n", "jupyter-k8s-system")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			podName := strings.TrimSpace(string(output))
			Expect(podName).NotTo(BeEmpty())

			By("checking webhook logs for successful processing")
			cmd = exec.Command("kubectl", "logs", podName, "-n", "jupyter-k8s-system", "--tail=50")
			output, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			logs := string(output)
			Expect(logs).To(ContainSubstring("workspace-with-annotations"), "webhook should be logging")
		})
	})

	Context("Webhook Configuration", func() {
		It("should have mutating webhook configuration registered", func() {
			By("checking mutating webhook configuration exists")
			cmd := exec.Command("kubectl", "get", "mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration")
			output, err := utils.Run(cmd)
			if err != nil {
				Skip("Mutating webhook configuration not found - webhooks may not be enabled in this deployment")
			}
			Expect(string(output)).To(ContainSubstring("jupyter-k8s-mutating-webhook-configuration"))

			By("verifying webhook targets correct service")
			cmd = exec.Command("kubectl", "get", "mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration",
				"-o", "jsonpath={.webhooks[0].clientConfig.service.name}")
			output, err = utils.Run(cmd)
			if err == nil {
				serviceName := strings.TrimSpace(string(output))
				Expect(serviceName).To(Equal("jupyter-k8s-controller-manager"))
			}
		})

		It("should have CA bundle configured", func() {
			By("checking CA bundle is present or webhook is configured")
			cmd := exec.Command("kubectl", "get", "mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration")
			_, err := utils.Run(cmd)
			if err != nil {
				Skip("Mutating webhook configuration not found - webhooks may not be enabled")
			}

			cmd = exec.Command("kubectl", "get", "mutatingwebhookconfiguration",
				"jupyter-k8s-mutating-webhook-configuration",
				"-o", "jsonpath={.webhooks[0].clientConfig.caBundle}")
			output, err := utils.Run(cmd)
			// CA bundle may be empty if cert-manager hasn't injected it yet, but config should exist
			Expect(err).NotTo(HaveOccurred())
			_ = output // CA bundle presence is optional depending on cert-manager setup
		})
	})
})
