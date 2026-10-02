//go:build e2e
// +build e2e

/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// stallDeadlineSeconds is the progress deadline the stall specs set on a workspace Deployment. The
// operator leaves the Kubernetes default of 600s in place; a spec cannot wait that long.
const stallDeadlineSeconds = 30

// WaitForWorkspaceToReachCondition polls a Workspace.status till a condition reaches the expected status
// nolint:unparam // namespace is always "default" now but may change in future tests
func WaitForWorkspaceToReachCondition(
	workspaceName string, namespace string, conditionType string, expectedStatus string) {
	ginkgo.GinkgoHelper()

	gomega.Eventually(func(g gomega.Gomega) {
		jsonPath := fmt.Sprintf("{.status.conditions[?(@.type==\"%s\")].status}", conditionType)
		output, err := kubectlGet("workspace", workspaceName, namespace, jsonPath)

		if err != nil {
			ginkgo.GinkgoWriter.Printf("ERROR getting workspace %s condition: %v", workspaceName, err)
		}
		g.Expect(err).NotTo(gomega.HaveOccurred())

		// If condition isn't met yet, check workspace status again
		if output != expectedStatus {
			statusOutput, statusErr := kubectlGet("workspace", workspaceName, namespace, "{.status.conditions}")
			if statusErr == nil {
				_, _ = fmt.Fprintf(ginkgo.GinkgoWriter, "All conditions for workspace %s: %s\n",
					workspaceName, statusOutput)
			}
		}
		g.Expect(output).To(gomega.Equal(expectedStatus))
	}).WithTimeout(5 * time.Minute).WithPolling(5 * time.Second).Should(gomega.Succeed())
}

// VerifyWorkspaceConditions polls until the workspace has exactly the conditions in
// expectedConditions (condition type to status, e.g. "Progressing" -> "True"), no more, no less.
// The controller re-evaluates readiness on every reconcile and rewrites the whole condition set,
// so a single read right after a transition may not see the settled set (#350).
func VerifyWorkspaceConditions(
	workspaceName string,
	namespace string,
	expectedConditions map[string]string,
) {
	ginkgo.GinkgoHelper()

	// Format: "Type=Status Type=Status ..."
	jsonPath := "{range .status.conditions[*]}{.type}{\"=\"}{.status}{\" \"}{end}"
	gomega.Eventually(func(g gomega.Gomega) {
		output, err := kubectlGet("workspace", workspaceName, namespace, jsonPath)
		g.Expect(err).NotTo(gomega.HaveOccurred())

		actualConditions := make(map[string]string)
		for _, pair := range strings.Fields(output) {
			parts := strings.Split(pair, "=")
			if len(parts) == 2 {
				actualConditions[parts[0]] = parts[1]
			}
		}

		g.Expect(actualConditions).To(gomega.Equal(expectedConditions),
			"conditions of workspace %s/%s", namespace, workspaceName)
	}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(gomega.Succeed())
}

// VerifyConsistentWorkspaceConditions polls workspace conditions for the given duration
// and asserts they remain stable (no oscillation).
func VerifyConsistentWorkspaceConditions(
	workspaceName string,
	namespace string,
	expectedConditions map[string]string,
	duration string,
	interval string,
) {
	ginkgo.GinkgoHelper()

	jsonPath := "{range .status.conditions[*]}{.type}{\"=\"}{.status}{\" \"}{end}"
	matchers := make([]gomega.OmegaMatcher, 0, len(expectedConditions))
	for condType, status := range expectedConditions {
		matchers = append(matchers, gomega.HaveKeyWithValue(condType, status))
	}

	gomega.Consistently(func() map[string]string {
		output, err := kubectlGet("workspace", workspaceName, namespace, jsonPath)
		if err != nil {
			return nil
		}
		conditions := make(map[string]string)
		for _, pair := range strings.Fields(output) {
			parts := strings.Split(pair, "=")
			if len(parts) == 2 {
				conditions[parts[0]] = parts[1]
			}
		}
		return conditions
	}, duration, interval).Should(gomega.SatisfyAll(matchers...),
		"workspace conditions should remain stable without oscillating")
}

// VerifyCreateWorkspaceRejectedByWebhook verifies that a workspace creation is rejected by the admission webhook
//
//nolint:unparam
func VerifyCreateWorkspaceRejectedByWebhook(
	filename string, group string, subgroup string, wsName string, wsNamespace string,
) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("attempting to create workspace %s", wsName))
	path := BuildTestResourcePath(filename, group, subgroup)
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	gomega.Expect(err).To(gomega.HaveOccurred(), fmt.Sprintf("Expected webhook to reject workspace %s", wsName))

	ginkgo.By(fmt.Sprintf("verifying workspace %s was not created", wsName))
	// Note: We don't use kubectlGet() here because we need --ignore-not-found flag
	cmd = exec.Command("kubectl", verbGet, "workspace", wsName, "-n", wsNamespace, "--ignore-not-found")
	output, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(output).To(gomega.BeEmpty(), "Workspace should not exist after webhook rejection")
}

// RestartWorkspacePod force a restart of the underlying pod
func RestartWorkspacePod(workspaceName, namespace string) {
	ginkgo.GinkgoHelper()

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
		phase, err := kubectlGet("pod", podName, namespace, jsonPathStatusPhase)
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
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("waiting for pod %s to be ready", podName))
	gomega.Eventually(func() error {
		phase, err := kubectlGet("pod", podName, namespace, jsonPathStatusPhase)
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
		if ready != valueTrue {
			return fmt.Errorf("pod %s container not ready yet", podName)
		}
		return nil
	}, 60*time.Second, 1*time.Second).To(gomega.Succeed())
}

// UpdateWorkspaceDesiredState updates the desiredStatus field of a Workspace using kubectl patch
func UpdateWorkspaceDesiredState(workspaceName, namespace, desiredState string) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("updating workspace %s desiredStatus to %s", workspaceName, desiredState))
	patchCmd := fmt.Sprintf(`{"spec":{"desiredStatus":"%s"}}`, desiredState)
	cmd := exec.Command("kubectl", "patch", "workspace", workspaceName,
		"-n", namespace, "--type=merge", "-p", patchCmd)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred(), "Failed to update workspace desiredStatus")
}

// WaitForWorkspaceDeletion polls until the workspace is fully removed from the cluster (NotFound).
func WaitForWorkspaceDeletion(workspaceName, namespace string) {
	ginkgo.GinkgoHelper()

	gomega.Eventually(func(g gomega.Gomega) {
		cmd := exec.Command("kubectl", verbGet, "workspace", workspaceName,
			"-n", namespace, "--ignore-not-found", "-o", "name")
		output, err := utils.Run(cmd)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(strings.TrimSpace(output)).To(gomega.BeEmpty(), "workspace should no longer exist")
	}).WithTimeout(2 * time.Minute).WithPolling(3 * time.Second).Should(gomega.Succeed())
}

// GetWorkspaceDeploymentName waits until the Workspace reports the Deployment it owns and returns its name.
func GetWorkspaceDeploymentName(workspaceName, namespace string) string {
	ginkgo.GinkgoHelper()

	var deploymentName string
	gomega.Eventually(func(g gomega.Gomega) {
		name, err := kubectlGet("workspace", workspaceName, namespace, "{.status.deploymentName}")
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(name).NotTo(gomega.BeEmpty(), "workspace.status.deploymentName should be set")
		deploymentName = name
	}, 2*time.Minute, 2*time.Second).Should(gomega.Succeed())

	return deploymentName
}

// setDeploymentProgressDeadline patches progressDeadlineSeconds on a workspace Deployment. The patch
// survives reconciles: the operator rewrites a Deployment only when its pod template drifted.
func setDeploymentProgressDeadline(deploymentName, namespace string, seconds int) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("setting progressDeadlineSeconds=%d on deployment %s", seconds, deploymentName))
	cmd := exec.Command("kubectl", "patch", "deployment", deploymentName, "-n", namespace,
		"--type=merge", "-p", fmt.Sprintf(`{"spec":{"progressDeadlineSeconds":%d}}`, seconds))
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// waitForWorkspaceStalled polls until the workspace reports Degraded=True with reason ComputeStalled and
// a message matching messageMatcher, and Available=False and Progressing=False under the same reason
// and message.
func waitForWorkspaceStalled(workspaceName, namespace string, messageMatcher gomega.OmegaMatcher) {
	ginkgo.GinkgoHelper()

	gomega.Eventually(func(g gomega.Gomega) {
		var ws workspacev1alpha1.Workspace
		g.Expect(kubectlGetInto("workspace", workspaceName, namespace, &ws)).To(gomega.Succeed())
		degraded := meta.FindStatusCondition(ws.Status.Conditions, ConditionTypeDegraded)
		g.Expect(degraded).NotTo(gomega.BeNil())
		g.Expect(string(degraded.Status)).To(gomega.Equal(ConditionTrue))
		g.Expect(degraded.Reason).To(gomega.Equal(controller.ReasonComputeStalled))
		g.Expect(degraded.Message).To(messageMatcher)
		for _, conditionType := range []string{ConditionTypeAvailable, ConditionTypeProgressing} {
			condition := meta.FindStatusCondition(ws.Status.Conditions, conditionType)
			g.Expect(condition).NotTo(gomega.BeNil(), conditionType)
			g.Expect(string(condition.Status)).To(gomega.Equal(ConditionFalse), conditionType)
			g.Expect(condition.Reason).To(gomega.Equal(controller.ReasonComputeStalled), conditionType)
			g.Expect(condition.Message).To(gomega.Equal(degraded.Message), conditionType)
		}
	}).WithTimeout(2 * time.Minute).WithPolling(3 * time.Second).Should(gomega.Succeed())
}

// workspaceStallEvents returns the WorkspaceComputeStalled events recorded on the workspace with the
// given UID. Events outlive the object they were recorded on, so a selection by name would also return
// the events of an earlier workspace of the same name, which specs reuse across runs.
func workspaceStallEvents(workspaceUID, namespace string) ([]corev1.Event, error) {
	cmd := exec.Command("kubectl", verbGet, "events", "-n", namespace,
		"--field-selector", fmt.Sprintf("involvedObject.uid=%s,reason=%s",
			workspaceUID, controller.EventWorkspaceComputeStalled),
		"-o", "json")
	output, err := utils.Run(cmd)
	if err != nil {
		return nil, err
	}
	var events corev1.EventList
	if err := json.Unmarshal([]byte(output), &events); err != nil {
		return nil, err
	}
	return events.Items, nil
}

// expectSingleStallEvent asserts that exactly one Warning WorkspaceComputeStalled event is recorded on
// the workspace, once, with a message matching messageMatcher.
func expectSingleStallEvent(workspaceName, namespace string, messageMatcher gomega.OmegaMatcher) {
	ginkgo.GinkgoHelper()

	workspaceUID, err := kubectlGet("workspace", workspaceName, namespace, "{.metadata.uid}")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	gomega.Expect(workspaceUID).NotTo(gomega.BeEmpty())

	gomega.Eventually(func(g gomega.Gomega) {
		events, err := workspaceStallEvents(workspaceUID, namespace)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(events).To(gomega.HaveLen(1))
		g.Expect(events[0].Type).To(gomega.Equal(corev1.EventTypeWarning))
		g.Expect(events[0].Count).To(gomega.BeNumerically("<=", 1))
		g.Expect(events[0].Message).To(messageMatcher)
	}).WithTimeout(30 * time.Second).WithPolling(2 * time.Second).Should(gomega.Succeed())
}

// GetWorkspaceServiceName waits until the Workspace reports the Service it owns and returns its name.
func GetWorkspaceServiceName(workspaceName, namespace string) string {
	ginkgo.GinkgoHelper()

	var serviceName string
	gomega.Eventually(func(g gomega.Gomega) {
		name, err := kubectlGet("workspace", workspaceName, namespace, "{.status.serviceName}")
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(name).NotTo(gomega.BeEmpty(), "workspace.status.serviceName should be set")
		serviceName = name
	}, 2*time.Minute, 5*time.Second).Should(gomega.Succeed())

	return serviceName
}

// GetServicePortNames returns the names of every port on a Service, space separated.
func GetServicePortNames(serviceName, namespace string) (string, error) {
	ginkgo.GinkgoHelper()

	return kubectlGet("service", serviceName, namespace,
		"{range .spec.ports[*]}{.name}{\" \"}{end}")
}

// deleteWorkspaceAsUser deletes a workspace with kubectl impersonation
func deleteWorkspaceAsUser(name, user string, groups []string) error {
	ginkgo.GinkgoHelper()
	args := make([]string, 0, 6+len(groups))
	args = append(args, "delete", "workspace", name, "-n", "default", "--as="+user)
	for _, group := range groups {
		args = append(args, "--as-group="+group)
	}
	cmd := exec.Command("kubectl", args...)
	_, err := utils.Run(cmd)
	return err
}

// deleteWorkspaceForTest removes one workspace by name, so it can never delete unrelated objects
// sharing the namespace. A missing workspace is not an error.
//
//nolint:unparam // helper kept general; current callers happen to share the namespace
func deleteWorkspaceForTest(workspaceName, namespace string) {
	ginkgo.GinkgoHelper()
	if workspaceName == "" {
		return
	}
	ginkgo.By(fmt.Sprintf("cleaning up workspace %s", workspaceName))
	cmd := exec.Command("kubectl", "delete", "workspace", workspaceName,
		"-n", namespace, "--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)
}

// patchWorkspaceForTest applies a JSON merge patch to a workspace and returns kubectl's output, so a
// caller can assert on either an accepted or a rejected change.
//
//nolint:unparam // helper kept general; current callers happen to share the namespace
func patchWorkspaceForTest(workspaceName, namespace, patch string) (string, error) {
	cmd := exec.Command("kubectl", "patch", "workspace", workspaceName,
		"-n", namespace, "--type=merge", "-p", patch)
	return utils.Run(cmd)
}

// patchTemplateForTest applies a JSON merge patch to a template in the shared namespace.
func patchTemplateForTest(templateName, patch string) {
	ginkgo.GinkgoHelper()
	cmd := exec.Command("kubectl", "patch", "workspacetemplate", templateName,
		"-n", SharedNamespace, "--type=merge", "-p", patch)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// deleteTemplateForTest removes one template from the shared namespace by name.
func deleteTemplateForTest(templateName string) {
	ginkgo.GinkgoHelper()
	ginkgo.By(fmt.Sprintf("cleaning up template %s", templateName))
	cmd := exec.Command("kubectl", "delete", "workspacetemplate", templateName,
		"-n", SharedNamespace, "--ignore-not-found", "--wait=true", "--timeout=60s")
	_, _ = utils.Run(cmd)
}
