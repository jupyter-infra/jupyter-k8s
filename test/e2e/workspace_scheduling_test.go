//go:build e2e
// +build e2e

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
