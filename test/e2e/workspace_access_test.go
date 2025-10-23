//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-ai-contrib/jupyter-k8s/test/utils"
)

var _ = Describe("Workspace Access Control", Ordered, func() {
	var controllerPodName string

	BeforeAll(func() {
		By("installing CRDs")
		cmd := exec.Command("make", "install")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")

		By("deploying the controller-manager")
		cmd = exec.Command("make", "deploy", fmt.Sprintf("IMG=%s", projectImage))
		_, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to deploy controller")

		By("waiting for controller-manager to be ready")
		verifyControllerUp := func(g Gomega) {
			cmd := exec.Command("kubectl", "get",
				"pods", "-l", "control-plane=controller-manager",
				"-o", "go-template={{ range .items }}"+
					"{{ if not .metadata.deletionTimestamp }}"+
					"{{ .metadata.name }}"+
					"{{ \"\\n\" }}{{ end }}{{ end }}",
				"-n", namespace,
			)
			podOutput, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			podNames := utils.GetNonEmptyLines(podOutput)
			g.Expect(podNames).To(HaveLen(1), "expected 1 controller pod running")
			controllerPodName = podNames[0]

			cmd = exec.Command("kubectl", "get",
				"pods", controllerPodName, "-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}",
				"-n", namespace,
			)
			readyStatus, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(readyStatus).To(Equal("True"), "expected controller pod to be Ready")
		}
		Eventually(verifyControllerUp, 2*time.Minute).Should(Succeed())
	})

	AfterAll(func() {
		By("cleaning up test resources")
		cmd := exec.Command("kubectl", "delete", "workspace",
			"access-test-workspace", "--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)
		
		cmd = exec.Command("kubectl", "delete", "workspaceaccessstrategy",
			"test-access-strategy", "--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)
	})

	Context("WorkspaceAccessStrategy", func() {
		It("should create and configure access strategy", func() {
			By("creating a WorkspaceAccessStrategy")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/workspace-access-strategy.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying access strategy is created")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "workspaceaccessstrategy", "test-access-strategy")
				_, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
			}, 30*time.Second, 5*time.Second).Should(Succeed())

			By("verifying access strategy configuration")
			cmd = exec.Command("kubectl", "get", "workspaceaccessstrategy", "test-access-strategy",
				"-o", "jsonpath={.spec.displayName}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Test Access Strategy"))
		})

		It("should create workspace with access strategy reference", func() {
			By("creating workspace with access strategy")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/static/workspace-with-access-strategy.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying workspace references access strategy")
			cmd = exec.Command("kubectl", "get", "workspace", "access-test-workspace",
				"-o", "jsonpath={.spec.accessStrategy.name}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("test-access-strategy"))
		})
	})
})
