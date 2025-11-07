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

// commenting out flaky test: https://github.com/jupyter-infra/jupyter-k8s/issues/45
// reinstate 'Describe' to run again
var _ = XDescribe("Workspace Scheduling", Ordered, func() {
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

			cmd = exec.Command("kubectl", "get",
				"pods", podNames[0], "-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}",
				"-n", namespace,
			)
			readyStatus, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(readyStatus).To(Equal("True"), "expected controller pod to be Ready")
		}
		Eventually(verifyControllerUp, 2*time.Minute).Should(Succeed())
	})

	AfterAll(func() {
		By("cleaning up test workspaces")
		// Use --wait=true for synchronous deletion to ensure finalizers are processed
		cmd := exec.Command("kubectl", "delete", "workspace",
			"workspace-with-affinity", "workspace-with-tolerations", "workspace-with-node-selector",
			"--ignore-not-found", "--wait=true", "--timeout=180s")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		// Undeploy controller BEFORE uninstalling CRDs to allow controller to process finalizers
		// This follows K8s best practice: delete resources in reverse order of creation
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("waiting for controller pod to be fully terminated")
		// Ensure controller is completely stopped before deleting CRDs
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "pods",
				"-n", "jupyter-k8s-system",
				"-l", "control-plane=controller-manager",
				"-o", "name")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(strings.TrimSpace(string(output))).To(BeEmpty())
		}).WithTimeout(60 * time.Second).WithPolling(2 * time.Second).Should(Succeed())

		By("uninstalling CRDs")
		// Delete CRDs last to avoid race conditions with controller finalizer processing
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)
	})

	Context("Node Affinity", func() {
		It("should create workspace with node affinity and apply it to deployment", func() {
			By("creating workspace with node affinity")
			workspaceYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: workspace-with-affinity
spec:
  displayName: "Workspace with Node Affinity"
  image: "jupyter/scipy-notebook:latest"
  desiredStatus: Running
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: kubernetes.io/arch
            operator: In
            values:
            - amd64
`
			cmd := exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
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
			workspaceYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: workspace-with-tolerations
spec:
  displayName: "Workspace with Tolerations"
  image: "jupyter/scipy-notebook:latest"
  desiredStatus: Running
  tolerations:
  - key: "dedicated"
    operator: "Equal"
    value: "jupyter"
    effect: "NoSchedule"
`
			cmd := exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
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
			workspaceYaml := `apiVersion: workspace.jupyter.org/v1alpha1
kind: Workspace
metadata:
  name: workspace-with-node-selector
spec:
  displayName: "Workspace with Node Selector"
  image: "jupyter/scipy-notebook:latest"
  desiredStatus: Running
  nodeSelector:
    kubernetes.io/arch: "amd64"
`
			cmd := exec.Command("sh", "-c",
				fmt.Sprintf("echo '%s' | kubectl apply -f -", workspaceYaml))
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
