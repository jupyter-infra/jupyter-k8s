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
	"strings"
	"time"

	"github.com/jupyter-ai-contrib/jupyter-k8s/test/utils"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// WaitForWorkspaceToReachCondition polls a Workspace.status till a condition reaches the expected status
// nolint:unparam // namespace is always "default" now but may change in future tests
func WaitForWorkspaceToReachCondition(
	workspaceName string, namespace string, conditionType string, expectedStatus string) {
	gomega.Eventually(func(g gomega.Gomega) {
		cmd := exec.Command("kubectl", "get", "workspace", workspaceName, "-n", namespace,
			"-o", fmt.Sprintf("jsonpath={.status.conditions[?(@.type==\"%s\")].status}", conditionType))
		output, err := utils.Run(cmd)

		// If condition isn't met yet, check workspace status again
		if output != expectedStatus {
			statusCmd := exec.Command("kubectl", "get", "workspace", workspaceName, "-n", namespace,
				"-o", "jsonpath={.status.conditions}")
			statusOutput, _ := utils.Run(statusCmd)
			_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "All conditions for workspace %s: %s\n",
				workspaceName, statusOutput)
		}

		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(output).To(gomega.Equal(expectedStatus))
	}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(gomega.Succeed())
}

// VerifyWorkspaceConditions verifies workspace status conditions
// expectedConditions is a map of condition type to expected status (e.g., "Progressing" -> "True")
func VerifyWorkspaceConditions(
	workspaceName string,
	namespace string,
	expectedConditions map[string]string,
) {
	ginkgo.GinkgoHelper()

	// Get all condition statuses in a single kubectl call
	// Format: "Type=Status Type=Status ..."
	jsonPath := "{range .status.conditions[*]}{.type}{\"=\"}{.status}{\" \"}{end}"
	output, err := kubectlGet("workspace", workspaceName, namespace, jsonPath)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())

	// Debug output
	_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "Conditions for workspace %s: %s\n",
		workspaceName, output)

	// Parse conditions from output (format: "Type=Status Type=Status ...")
	actualConditions := make(map[string]string)
	pairs := strings.Fields(output)
	for _, pair := range pairs {
		parts := strings.Split(pair, "=")
		if len(parts) == 2 {
			actualConditions[parts[0]] = parts[1]
		}
	}

	// Verify the number of conditions matches
	gomega.Expect(actualConditions).To(gomega.HaveLen(len(expectedConditions)),
		"Expected %d conditions but found %d", len(expectedConditions), len(actualConditions))

	// Assert on each expected condition
	for conditionType, expectedStatus := range expectedConditions {
		gomega.Expect(actualConditions[conditionType]).To(gomega.Equal(expectedStatus),
			"%s condition should be %s but got %s", conditionType, expectedStatus, actualConditions[conditionType])
	}
}

// VerifyCreateWorkspaceRejectedByWebhook verifies that a workspace creation is rejected by the admission webhook
//
//nolint:unparam
func VerifyCreateWorkspaceRejectedByWebhook(filename string, group string, subgroup string, wsName string) {
	ginkgo.By(fmt.Sprintf("creating workspace %s", wsName))
	path := BuildTestResourcePath(filename, group, subgroup)
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	gomega.Expect(err).To(gomega.HaveOccurred(), fmt.Sprintf("Expected webhook to reject workspace %s", wsName))

	ginkgo.By(fmt.Sprintf("verifying workspace %s was not created", wsName))
	cmd = exec.Command("kubectl", "get", "workspace", wsName, "--ignore-not-found")
	output, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(output).To(gomega.BeEmpty(), "Workspace should not exist after webhook rejection")
}

// RestartWorkspacePod force a restart of the underlying pod
func RestartWorkspacePod(workspaceName, namespace string) {
	ginkgo.By(fmt.Sprintf("retrieving the pod by label for workspace %s", workspaceName))
	podSelector := fmt.Sprintf("%s=%s", WorkspaceLabelName, workspaceName)
	podName, podErr := kubectlGetByLabels("pod", podSelector, namespace, "{.items[*].metadata.name}")
	gomega.Expect(podErr).NotTo(gomega.HaveOccurred())
	gomega.Expect(podName).NotTo(gomega.BeEmpty())

	ginkgo.By(fmt.Sprintf("deleting pod %s", podName))
	cmd := exec.Command("kubectl", "delete", "pod", podName, "-n", namespace)
	_, deletePodErr := utils.Run(cmd)
	gomega.Expect(deletePodErr).NotTo(gomega.HaveOccurred())

	ginkgo.By("waiting for pod to be recreated by the deployment")
	gomega.Eventually(func(g gomega.Gomega) error {
		name, err := kubectlGetByLabels("pod", podSelector, namespace, "{.items[*].metadata.name}")
		if err != nil {
			return err
		}
		if name == "" {
			return fmt.Errorf("no pods found with label %s", podSelector)
		}
		podName = name
		return nil
	}).WithTimeout(30 * time.Second).WithPolling(1 * time.Second).To(gomega.Succeed())

	ginkgo.By("waiting for pod to be running")
	gomega.Eventually(func(g gomega.Gomega) error {
		phase, err := kubectlGet("pod", podName, namespace, "{.status.phase}")
		if err != nil {
			return err
		}
		if phase != "Running" {
			return fmt.Errorf("pod %s not running yet: %s", podName, phase)
		}
		return nil
	}).WithTimeout(30 * time.Second).WithPolling(1 * time.Second).To(gomega.Succeed())
}

// WaitForWorkspacePodToBeReady polls the pod until it is running and the workspace
// container is ready
func WaitForWorkspacePodToBeReady(podName, namespace string) {
	ginkgo.By(fmt.Sprintf("waiting for pod %s to be ready", podName))
	gomega.Eventually(func() error {
		phase, err := kubectlGet("pod", podName, namespace, "{.status.phase}")
		if err != nil {
			return err
		}
		if phase != "Running" {
			return fmt.Errorf("pod %s not running yet: %s", podName, phase)
		}

		// Also check container ready status
		ready, err := kubectlGet("pod", podName, namespace,
			"{.status.containerStatuses[?(@.name=='workspace')].ready}")
		if err != nil {
			return err
		}
		if ready != "true" {
			return fmt.Errorf("pod %s container not ready yet", podName)
		}
		return nil
	}, 60*time.Second, 1*time.Second).To(gomega.Succeed())
}
