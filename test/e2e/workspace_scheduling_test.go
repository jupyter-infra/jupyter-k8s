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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

var _ = XDescribe("Workspace Scheduling", Ordered, func() {
	Context("Node Affinity", func() {
		It("should create workspace with node affinity and apply it to deployment", func() {
			By("creating workspace with node affinity")
			cmd := exec.Command("kubectl", "apply", "-f", "static/scheduling/workspace-with-affinity.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace has correct affinity spec")
			cmd = exec.Command("kubectl", "get", "workspace", "workspace-with-affinity",
				"-o", "jsonpath={.spec.affinity.nodeAffinity.requiredDuringSchedulingIgnoredDuringExecution.nodeSelectorTerms[0].matchExpressions[0].key}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("kubernetes.io/arch"))
		})
	})

	Context("Tolerations", func() {
		It("should create workspace with tolerations and apply them to deployment", func() {
			By("creating workspace with tolerations")
			cmd := exec.Command("kubectl", "apply", "-f", "static/scheduling/workspace-with-tolerations.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace has correct tolerations spec")
			cmd = exec.Command("kubectl", "get", "workspace", "workspace-with-tolerations",
				"-o", "jsonpath={.spec.tolerations[0].key}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("dedicated"))
		})
	})

	Context("Node Selector", func() {
		It("should create workspace with node selector and apply it to deployment", func() {
			By("creating workspace with node selector")
			cmd := exec.Command("kubectl", "apply", "-f", "static/scheduling/workspace-with-node-selector.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace has correct node selector spec")
			cmd = exec.Command("kubectl", "get", "workspace", "workspace-with-node-selector",
				"-o", "jsonpath={.spec.nodeSelector}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("kubernetes.io/arch"))
			Expect(output).To(ContainSubstring("amd64"))
		})
	})
})
