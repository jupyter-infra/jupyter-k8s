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
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// Helpers for the Workspace Integration e2e suite (workspace_integration_test.go): small accessors on
// the decoded Workspace/Deployment plus fixture/patch/cleanup utilities. Kept here rather than inline
// in the spec file so the specs read as assertions, not plumbing.

// findResolvedIntegration returns the frozen resolvedIntegrations entry for the named template, or nil.
//
//nolint:unparam // name is a general parameter; current specs all look up "service-integration"
func findResolvedIntegration(ws *workspacev1alpha1.Workspace, name string) *workspacev1alpha1.ResolvedIntegration {
	for i := range ws.Status.ResolvedIntegrations {
		if ws.Status.ResolvedIntegrations[i].Name == name {
			return &ws.Status.ResolvedIntegrations[i]
		}
	}
	return nil
}

// resolvedIntegrationsNamed returns ALL frozen resolvedIntegrations entries matching name. Used to
// assert there is exactly one (a duplicate would mean the freeze recorded the same integration twice).
//
//nolint:unparam // name is a general parameter; current specs all look up "service-integration"
func resolvedIntegrationsNamed(ws *workspacev1alpha1.Workspace, name string) []workspacev1alpha1.ResolvedIntegration {
	var out []workspacev1alpha1.ResolvedIntegration
	for i := range ws.Status.ResolvedIntegrations {
		if ws.Status.ResolvedIntegrations[i].Name == name {
			out = append(out, ws.Status.ResolvedIntegrations[i])
		}
	}
	return out
}

// findIntegrationStatus returns the status.integrationStatuses[] entry for the named template, or nil.
func findIntegrationStatus(ws *workspacev1alpha1.Workspace, name string) *workspacev1alpha1.IntegrationStatus {
	for i := range ws.Status.IntegrationStatuses {
		if ws.Status.IntegrationStatuses[i].Name == name {
			return &ws.Status.IntegrationStatuses[i]
		}
	}
	return nil
}

// conditionReason returns the Reason of the named condition on an integration status, or "".
func conditionReason(is *workspacev1alpha1.IntegrationStatus, condType string) string {
	if is == nil {
		return ""
	}
	for _, c := range is.Conditions {
		if c.Type == condType {
			return c.Reason
		}
	}
	return ""
}

// containerByName returns the named container from a pod spec, or nil.
func containerByName(spec corev1.PodSpec, name string) *corev1.Container {
	for i := range spec.Containers {
		if spec.Containers[i].Name == name {
			return &spec.Containers[i]
		}
	}
	return nil
}

// envValue returns the value of the named env var on a container, or "".
func envValue(c *corev1.Container, name string) string {
	if c == nil {
		return ""
	}
	for _, e := range c.Env {
		if e.Name == name {
			return e.Value
		}
	}
	return ""
}

// applyIntegrationFixture applies a fixture YAML from the integration static dir.
func applyIntegrationFixture(filename string) {
	ginkgo.GinkgoHelper()
	path := BuildTestResourcePath(filename, "integration", "")
	ginkgo.By(fmt.Sprintf("applying fixture %s", path))
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// patchWorkspaceFromFile applies a merge patch to a workspace in place -- the idiom the repo uses for
// in-place spec updates instead of re-applying a whole object with the same name. patchPath is the
// full repo-relative path to the JSON patch file (BuildTestResourcePath only builds .yaml paths).
func patchWorkspaceFromFile(name, namespace, patchPath string) {
	ginkgo.GinkgoHelper()
	ginkgo.By(fmt.Sprintf("patching workspace %s/%s with %s", namespace, name, patchPath))
	cmd := exec.Command("kubectl", "patch", "workspace", name,
		"-n", namespace, "--type=merge", "--patch-file", patchPath)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// touchWorkspaceToForceReconcile annotates the workspace with a unique value to enqueue an immediate
// reconcile. The controller watches Workspace but NOT WorkspaceIntegrationTemplate, so a template
// edit alone does not enqueue the referencing workspace; a nudge avoids waiting for the periodic
// reconcile cadence so a spec need not wait for it to observe the new template version.
func touchWorkspaceToForceReconcile(name, namespace string) {
	ginkgo.GinkgoHelper()
	patch := fmt.Sprintf(`{"metadata":{"annotations":{"e2e.jupyter.org/reconcile-nudge":"%d"}}}`, time.Now().UnixNano())
	cmd := exec.Command("kubectl", "patch", "workspace", name, "-n", namespace,
		"--type=merge", "-p", patch)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// verifyNoSidecarInDeployment asserts (and keeps asserting) that no cache-proxy container exists on
// the deployment's pod template -- used for the fail-closed base-only path.
func verifyNoSidecarInDeployment(deploymentName, namespace string) {
	ginkgo.GinkgoHelper()
	gomega.Consistently(func(g gomega.Gomega) {
		names, err := kubectlGet("deployment", deploymentName, namespace,
			"{.spec.template.spec.containers[?(@.name=='cache-proxy')].name}")
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(names).To(gomega.BeEmpty(), "no cache-proxy overlay must be applied when capture failed")
	}, 15*time.Second, 5*time.Second).Should(gomega.Succeed())
}

// deleteResourcesForIntegrationTest removes only the objects this Ordered suite creates, by explicit
// name, so it can never nuke unrelated objects that happen to share the "default" namespace. The
// workspaces live in the workspace namespace; the templates are admin resources in the shared
// namespace. This suite is Ordered (serial), so a fixed, known name set is sufficient.
func deleteResourcesForIntegrationTest(workspaceNamespace string) {
	ginkgo.GinkgoHelper()
	ginkgo.By("cleaning up workspaces")
	cmd := exec.Command("kubectl", "delete", "workspace",
		"workspace-with-integration", "workspace-failprobe", "workspace-missing-resource",
		"-n", workspaceNamespace, "--ignore-not-found", "--wait=true", "--timeout=120s")
	_, _ = utils.Run(cmd)

	ginkgo.By("cleaning up integration templates")
	cmd = exec.Command("kubectl", "delete", "workspaceintegrationtemplate",
		"service-integration", "service-integration-failprobe",
		"-n", SharedNamespace, "--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)

	// Wait for the Services to be fully gone (--wait=true) instead of sleeping a fixed interval: the
	// next spec re-applies these same names, and applying while the prior object is still terminating
	// would conflict. --wait=true is the same deletion-sync idiom used by the deletes above.
	ginkgo.By("cleaning up Services")
	cmd = exec.Command("kubectl", "delete", "service",
		"shared-cache", "other-cache",
		"-n", workspaceNamespace, "--ignore-not-found", "--wait=true", "--timeout=30s")
	_, _ = utils.Run(cmd)
}

// workspacePodUID returns the UID of the workspace's single pod, or "" if none is found. The UID is the
// ground-truth "same pod" signal: a roll creates a new pod with a new UID, so a stable UID across
// reconciles proves the pod was never restarted.
func workspacePodUID(workspaceName, namespace string) string {
	ginkgo.GinkgoHelper()
	uid, err := kubectlGetByLabels("pod",
		fmt.Sprintf("%s=%s", controller.LabelWorkspaceName, workspaceName),
		namespace, "{.items[0].metadata.uid}")
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
	return uid
}

// touchDeploymentToForceReconcile annotates the workspace's OWNED Deployment metadata (NOT the pod
// template, NOT the Workspace). The controller Owns Deployments, so this enqueues a reconcile -- but the
// annotation never reaches the pod template (buildPodAnnotations copies only Workspace annotations), so
// the nudge can't roll the pod itself. That lets a drift spec force reconciles while still asserting the
// pod was never restarted. Best-effort: it only needs to enqueue, and callers poll.
func touchDeploymentToForceReconcile(deploymentName, namespace string) {
	ginkgo.GinkgoHelper()
	patch := fmt.Sprintf(`{"metadata":{"annotations":{"e2e.jupyter.org/reconcile-nudge":"%d"}}}`, time.Now().UnixNano())
	cmd := exec.Command("kubectl", "patch", "deployment", deploymentName, "-n", namespace,
		"--type=merge", "-p", patch)
	_, _ = utils.Run(cmd)
}
