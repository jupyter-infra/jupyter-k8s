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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-ai-contrib/jupyter-k8s/test/utils"
)

var _ = Describe("Workspace Scheduling", Ordered, func() {
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
		cmd := exec.Command("kubectl", "delete", "workspace",
			"workspace-with-affinity", "workspace-with-tolerations", "workspace-with-node-selector",
			"--ignore-not-found", "--wait=false")
		_, _ = utils.Run(cmd)

		By("undeploying the controller-manager")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		By("uninstalling CRDs")
		cmd = exec.Command("make", "uninstall")
		_, _ = utils.Run(cmd)
	})

	Context("Node Affinity", func() {
		It("should create workspace with node affinity and apply it to deployment", func() {
			By("creating workspace with node affinity")
			workspaceYaml := `apiVersion: workspaces.jupyter.org/v1alpha1
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
			workspaceYaml := `apiVersion: workspaces.jupyter.org/v1alpha1
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
			workspaceYaml := `apiVersion: workspaces.jupyter.org/v1alpha1
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
