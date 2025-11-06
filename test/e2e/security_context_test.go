//go:build e2e
// +build e2e

/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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

var _ = Describe("Pod Security Context", Ordered, func() {
	const (
		templateName = "security-test-template"
		workspaceName = "security-test-workspace"
	)

	BeforeAll(func() {
		By("creating template with pod security context")
		templateYAML := fmt.Sprintf(`
apiVersion: workspace.jupyter.org/v1alpha1
kind: WorkspaceTemplate
metadata:
  name: %s
spec:
  displayName: "Security Test Template"
  description: "Template for testing pod security context"
  defaultImage: "quay.io/jupyter/minimal-notebook:latest"
  defaultPodSecurityContext:
    fsGroup: 1000
    runAsNonRoot: true
    runAsUser: 1000
  primaryStorage:
    defaultSize: "1Gi"
`, templateName)

		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(templateYAML)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for template to be created")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "workspacetemplate", templateName, "-o", "name")
			_, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
		}, time.Minute*1, time.Second*5).Should(Succeed())
	})

	AfterAll(func() {
		By("cleaning up workspace")
		cmd := exec.Command("kubectl", "delete", "workspace", workspaceName, "--ignore-not-found", "--timeout=60s")
		_, _ = utils.Run(cmd)

		By("cleaning up override workspace")
		cmd = exec.Command("kubectl", "delete", "workspace", workspaceName+"-override", "--ignore-not-found", "--timeout=60s")
		_, _ = utils.Run(cmd)

		By("cleaning up template")
		cmd = exec.Command("kubectl", "delete", "workspacetemplate", templateName, "--ignore-not-found", "--timeout=60s")
		_, _ = utils.Run(cmd)
	})

	It("should apply pod security context from template to workspace and deployment", func() {
		By("creating workspace with template reference")
		workspaceYAML := fmt.Sprintf(`
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: %s
  namespace: %s
spec:
  displayName: "Security Test Workspace"
  templateRef: %s
`, workspaceName, namespace, templateName)

		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(workspaceYAML)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("verifying workspace has pod security context defaulted from template")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "workspace", workspaceName,
				"-n", namespace,
				"-o", "jsonpath={.spec.podSecurityContext.fsGroup}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(output)).To(Equal("1000"))
		}, time.Minute*1, time.Second*5).Should(Succeed())

		By("waiting for deployment to be created")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "deployment", 
				fmt.Sprintf("workspace-%s", workspaceName), 
				"-n", namespace, "-o", "name")
			_, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
		}, time.Minute*2, time.Second*5).Should(Succeed())

		By("verifying deployment has pod security context applied")
		cmd = exec.Command("kubectl", "get", "deployment",
			fmt.Sprintf("workspace-%s", workspaceName),
			"-n", namespace,
			"-o", "jsonpath={.spec.template.spec.securityContext.fsGroup}")
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(output)).To(Equal("1000"))

		By("verifying deployment has runAsUser applied")
		cmd = exec.Command("kubectl", "get", "deployment",
			fmt.Sprintf("workspace-%s", workspaceName),
			"-n", namespace,
			"-o", "jsonpath={.spec.template.spec.securityContext.runAsUser}")
		output, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(output)).To(Equal("1000"))

		By("verifying deployment has runAsNonRoot applied")
		cmd = exec.Command("kubectl", "get", "deployment",
			fmt.Sprintf("workspace-%s", workspaceName),
			"-n", namespace,
			"-o", "jsonpath={.spec.template.spec.securityContext.runAsNonRoot}")
		output, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(output)).To(Equal("true"))
	})

	It("should allow workspace to override template security context", func() {
		By("creating workspace with custom pod security context")
		workspaceYAML := fmt.Sprintf(`
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: %s-override
  namespace: %s
spec:
  displayName: "Security Override Test"
  templateRef: %s
  podSecurityContext:
    fsGroup: 2000
    runAsUser: 2000
    runAsNonRoot: true
`, workspaceName, namespace, templateName)

		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(workspaceYAML)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("verifying workspace has custom pod security context")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "workspace", workspaceName+"-override",
				"-n", namespace,
				"-o", "jsonpath={.spec.podSecurityContext.fsGroup}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(output)).To(Equal("2000"))
		}, time.Minute*1, time.Second*5).Should(Succeed())

		By("waiting for override deployment to be created")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "deployment", 
				fmt.Sprintf("workspace-%s-override", workspaceName), 
				"-n", namespace, "-o", "name")
			_, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
		}, time.Minute*2, time.Second*5).Should(Succeed())

		By("verifying custom fsGroup is applied to deployment")
		cmd = exec.Command("kubectl", "get", "deployment",
			fmt.Sprintf("workspace-%s-override", workspaceName),
			"-n", namespace,
			"-o", "jsonpath={.spec.template.spec.securityContext.fsGroup}")
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(output)).To(Equal("2000"))

		By("verifying custom runAsUser is applied to deployment")
		cmd = exec.Command("kubectl", "get", "deployment",
			fmt.Sprintf("workspace-%s-override", workspaceName),
			"-n", namespace,
			"-o", "jsonpath={.spec.template.spec.securityContext.runAsUser}")
		output, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(output)).To(Equal("2000"))
	})

	It("should work without template (no security context applied)", func() {
		By("creating workspace without template reference")
		workspaceYAML := fmt.Sprintf(`
apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: %s-no-template
  namespace: %s
spec:
  displayName: "No Template Test"
  image: "quay.io/jupyter/minimal-notebook:latest"
`, workspaceName, namespace)

		cmd := exec.Command("kubectl", "apply", "-f", "-")
		cmd.Stdin = strings.NewReader(workspaceYAML)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("waiting for no-template deployment to be created")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "deployment", 
				fmt.Sprintf("workspace-%s-no-template", workspaceName), 
				"-n", namespace, "-o", "name")
			_, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
		}, time.Minute*2, time.Second*5).Should(Succeed())

		By("verifying no security context is applied")
		cmd = exec.Command("kubectl", "get", "deployment",
			fmt.Sprintf("workspace-%s-no-template", workspaceName),
			"-n", namespace,
			"-o", "jsonpath={.spec.template.spec.securityContext}")
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(output)).To(BeEmpty())

		By("cleaning up no-template workspace")
		cmd = exec.Command("kubectl", "delete", "workspace", workspaceName+"-no-template", "--ignore-not-found", "--timeout=60s")
		_, _ = utils.Run(cmd)
	})
})
