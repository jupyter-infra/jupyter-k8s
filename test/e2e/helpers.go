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

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
)

// kubectlGet retrieves a Kubernetes resource with optional JSONPath query
func kubectlGet(resource, name, namespace, jsonpath string) (string, error) {
	ginkgo.GinkgoHelper()
	args := []string{"get", resource}
	if name != "" {
		args = append(args, name)
	}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	if jsonpath != "" {
		args = append(args, "-o", "jsonpath="+jsonpath)
	}
	cmd := exec.Command("kubectl", args...)
	output, err := utils.Run(cmd)
	return strings.TrimSpace(output), err
}

// kubectlGetByLabels retrieves Kubernetes resources using label selectors
//
//nolint:unparam
func kubectlGetByLabels(resource, labelSelector, namespace, jsonpath string) (string, error) {
	ginkgo.GinkgoHelper()
	args := []string{"get", resource, "-l", labelSelector}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	if jsonpath != "" {
		args = append(args, "-o", "jsonpath="+jsonpath)
	}
	cmd := exec.Command("kubectl", args...)
	output, err := utils.Run(cmd)
	return strings.TrimSpace(output), err
}

// kubectlGetAllNamespaces retrieves resources across all namespaces
func kubectlGetAllNamespaces(resource, jsonpath string) (string, error) {
	ginkgo.GinkgoHelper()
	args := []string{"get", resource, "-A"}
	if jsonpath != "" {
		args = append(args, "-o", "jsonpath="+jsonpath)
	}
	cmd := exec.Command("kubectl", args...)
	output, err := utils.Run(cmd)
	return strings.TrimSpace(output), err
}

// kubectlDeleteAllNamespaces deletes resources across all namespaces
func kubectlDeleteAllNamespaces(resource string, opts ...string) error {
	ginkgo.GinkgoHelper()
	args := []string{"delete", resource, "--all", "--all-namespaces"}
	args = append(args, opts...)
	cmd := exec.Command("kubectl", args...)
	_, err := utils.Run(cmd)
	return err
}

// ensureCleanState cleans up any leftover CRDs before starting tests
func ensureCleanState() {
	ginkgo.GinkgoHelper()

	ginkgo.By("checking for leftover Workspaces")
	// List all Workspaces across all namespaces
	workspaceList, err := kubectlGetAllNamespaces("workspaces", "{.items[*].metadata.name}")
	if err == nil && workspaceList != "" {
		ginkgo.By("deleting leftover Workspaces")
		_ = kubectlDeleteAllNamespaces("workspaces", "--ignore-not-found", "--wait=true", "--timeout=180s")
	}

	ginkgo.By("checking for leftover WorkspaceTemplates")
	templateList, err := kubectlGetAllNamespaces("workspacetemplates", "{.items[*].metadata.name}")
	if err == nil && templateList != "" {
		ginkgo.By("deleting leftover WorkspaceTemplates")
		_ = kubectlDeleteAllNamespaces("workspacetemplates", "--ignore-not-found", "--wait=true", "--timeout=180s")
	}

	ginkgo.By("checking for leftover WorkspaceAccessStrategies")
	accessStrategyList, err := kubectlGetAllNamespaces("workspaceaccessstrategies", "{.items[*].metadata.name}")
	if err == nil && accessStrategyList != "" {
		ginkgo.By("deleting leftover WorkspaceAccessStrategies")
		_ = kubectlDeleteAllNamespaces("workspaceaccessstrategies", "--ignore-not-found", "--wait=true", "--timeout=180s")
	}

	// Wait to ensure all resources are fully deleted
	// This helps avoid race conditions with finalizers
	time.Sleep(2 * time.Second)
}

// checkAndCleanCluster checks if operator is installed and uninstalls if needed
func checkAndCleanCluster() {
	ginkgo.GinkgoHelper()

	ginkgo.By("checking if operator is already installed")
	cmd := exec.Command("kubectl", "get", "deployment", "jupyter-k8s-controller-manager",
		"-n", OperatorNamespace, "--ignore-not-found")
	output, _ := utils.Run(cmd)

	if strings.TrimSpace(output) != "" {
		ginkgo.By("uninstalling existing operator")
		cmd = exec.Command("make", "undeploy")
		_, _ = utils.Run(cmd)

		// Wait for resources to be cleaned up
		time.Sleep(10 * time.Second)
	}

	ginkgo.By("uninstalling existing CRDs")
	// Use kubectl directly instead of make uninstall to avoid errors when CRDs don't exist
	crds := GetWorkspaceCrds()

	for _, crd := range crds {
		cmd = exec.Command("kubectl", "delete", "crd", crd, "--ignore-not-found")
		_, _ = utils.Run(cmd)
	}

	// Wait for CRDs to be removed
	time.Sleep(5 * time.Second)
}

// diagnoseAndCleanupStuckResources is a generic function to handle resources stuck with finalizers
// and perform emergency cleanup if needed for suite teardown.
func diagnoseAndCleanupStuckResources(
	resourceKind string,
	refJSONPath string,
	displayName string,
) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("checking for stuck %s with finalizers (emergency cleanup)", displayName))
	// Wait up to 30s for controller to process finalizer removal
	cmd := exec.Command("kubectl", "wait", resourceKind, "--all", "--all-namespaces",
		"--for=delete", "--timeout=30s")
	if err := cmd.Run(); err != nil {
		// Resources still exist - diagnose before emergency cleanup
		cmd = exec.Command("kubectl", "get", resourceKind, "-A",
			"-o", "jsonpath={.items[*].metadata.name}")
		if output, _ := utils.Run(cmd); len(output) > 0 && strings.TrimSpace(output) != "" {
			resources := strings.Fields(strings.TrimSpace(output))
			_, _ = fmt.Fprintf(ginkgo.GinkgoWriter,
				"\n⚠️  WARNING: %s still exist after workspace cleanup: %s\n",
				displayName, strings.Join(resources, ", "))

			// Diagnose: Check which workspaces reference each stuck resource
			for _, resName := range resources {
				cmd = exec.Command("kubectl", "get", "workspace", "-A",
					"-o", fmt.Sprintf("jsonpath={.items[?(@%s==\"%s\")].metadata.name}", refJSONPath, resName))
				if wsOutput, _ := utils.Run(cmd); len(wsOutput) > 0 && strings.TrimSpace(wsOutput) != "" {
					_, _ = fmt.Fprintf(ginkgo.GinkgoWriter,
						"   %s '%s' is referenced by workspaces: %s (test leaked resources)\n",
						displayName, resName, strings.TrimSpace(wsOutput))
				} else {
					_, _ = fmt.Fprintf(ginkgo.GinkgoWriter,
						"   %s '%s' has NO workspace references - CONTROLLER BUG?\n",
						displayName, resName)
				}
			}

			// Emergency cleanup: Force-remove finalizers to allow suite teardown
			ginkgo.By(fmt.Sprintf("EMERGENCY: Force-removing stuck %s finalizers for clean teardown", displayName))
			cmd = exec.Command("kubectl", "get", resourceKind, "-A", "-o", "name")
			if resourceList, err := utils.Run(cmd); err == nil && len(resourceList) > 0 {
				resources := strings.Split(strings.TrimSpace(resourceList), "\n")
				for _, resource := range resources {
					if resource != "" {
						cmd = exec.Command("kubectl", "patch", resource,
							"--type=json", "-p", `[{"op": "remove", "path": "/metadata/finalizers"}]`)
						_, _ = utils.Run(cmd)
					}
				}
				// Retry deletion after removing finalizers
				cmd = exec.Command("kubectl", "delete", resourceKind, "--all", "--all-namespaces",
					"--ignore-not-found", "--timeout=30s")
				_, _ = utils.Run(cmd)
			}
		}
	}
}

// diagnoseAndCleanupStuckAccessStrategies checks for access strategies stuck with finalizers
// and performs emergency cleanup if needed for suite teardown.
func diagnoseAndCleanupStuckAccessStrategies() {
	diagnoseAndCleanupStuckResources(
		"workspaceaccessstrategy",
		".spec.accessStrategy.name",
		"AccessStrategies",
	)
}

// diagnoseAndCleanupStuckTemplates checks for templates stuck with finalizers
// and performs emergency cleanup if needed for suite teardown.
func diagnoseAndCleanupStuckTemplates() {
	diagnoseAndCleanupStuckResources(
		"workspacetemplate",
		".spec.templateRef",
		"Templates",
	)
}

// createTemplateForTest creates a WorkspaceTemplate resource from a YAML file
// filename: name of the YAML file (without .yaml extension)
// group: primary directory (e.g., "template")
// subgroup: optional subdirectory (e.g., "base" for "template-base/")
// Note: group parameter currently always receives "template" but will be used for other groups in the future
//
//nolint:unparam
func createTemplateForTest(filename, group, subgroup string) {
	ginkgo.GinkgoHelper()
	path := BuildTestResourcePath(filename, group, subgroup)
	ginkgo.By(fmt.Sprintf("creating template %s from %s", filename, path))
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// createWorkspaceForTest creates a Workspace resource from a YAML file
// filename: name of the YAML file (without .yaml extension)
// group: primary directory (e.g., "access-strategy")
// subgroup: optional subdirectory (e.g., "base" for "template-base/")
func createWorkspaceForTest(filename, group, subgroup string) {
	ginkgo.GinkgoHelper()
	path := BuildTestResourcePath(filename, group, subgroup)
	ginkgo.By(fmt.Sprintf("creating workspace %s from %s", filename, path))
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// createAccessStrategyForTest creates a WorkspaceAccessStrategy resource from a YAML file
// filename: name of the YAML file (without .yaml extension)
// group: primary directory (e.g., "access-strategy")
// subgroup: optional subdirectory (e.g., "base" for "template-base/")
func createAccessStrategyForTest(filename, group, subgroup string) {
	ginkgo.GinkgoHelper()
	path := BuildTestResourcePath(filename, group, subgroup)
	ginkgo.By(fmt.Sprintf("creating access strategy %s from %s", filename, path))
	cmd := exec.Command("kubectl", "apply", "-f", path)
	_, err := utils.Run(cmd)
	gomega.Expect(err).NotTo(gomega.HaveOccurred())
}

// BuildTestResourcePath constructs the file path for test resources
// If subgroup is provided, uses {group}-{subgroup}/{filename}.yaml
// Otherwise uses {group}/{filename}.yaml
func BuildTestResourcePath(filename, group, subgroup string) string {
	dir := group
	if subgroup != "" {
		dir = fmt.Sprintf("%s-%s", group, subgroup)
	}
	return fmt.Sprintf("test/e2e/static/%s/%s.yaml", dir, filename)
}

// ResourceExists checks if a Kubernetes resource exists by querying with kubectl (immediate check)
// Returns bool for use in Eventually/Consistently blocks or direct assertions
func ResourceExists(
	kind string,
	name string,
	namespace string,
	jsonpath string,
) bool {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("retrieving resource %s in namespace %s", name, namespace))
	output, err := kubectlGet(kind, name, namespace, jsonpath)

	return err == nil && output != ""
}

// VerifyResourceDoesNotExist checks if a Kubernetes resource does not exist and stays non-existent
func VerifyResourceDoesNotExist(
	kind string,
	name string,
	namespace string,
) {
	ginkgo.GinkgoHelper()

	ginkgo.By(fmt.Sprintf("verifying resource %s does not exist in namespace %s", name, namespace))
	gomega.Consistently(func() error {
		output, err := kubectlGet(kind, name, namespace, "")
		if err != nil {
			// NotFound error is expected and means resource doesn't exist
			if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "not found") {
				return nil
			}
			return err
		}
		// If no error but got output, resource exists - this is a failure
		if output != "" {
			return fmt.Errorf("resource %s/%s still exists", kind, name)
		}
		return nil
	}, 5*time.Second, 1*time.Second).Should(gomega.Succeed())
}

// WaitForResourceToExist waits for a Kubernetes resource to exist using Eventually
func WaitForResourceToExist(
	kind string,
	name string,
	namespace string,
	jsonpath string,
	timeout time.Duration,
	polling time.Duration,
) {
	ginkgo.GinkgoHelper()
	gomega.Eventually(func(g gomega.Gomega) {
		output, err := kubectlGet(kind, name, namespace, jsonpath)
		g.Expect(err).NotTo(gomega.HaveOccurred())
		g.Expect(output).NotTo(gomega.BeEmpty())
	}).WithTimeout(timeout).WithPolling(polling).Should(gomega.Succeed())
}

// WaitForResourceToNotExist waits for a Kubernetes resource to not exist using Eventually
func WaitForResourceToNotExist(
	kind string,
	name string,
	namespace string,
	timeout time.Duration,
	polling time.Duration,
) {
	ginkgo.GinkgoHelper()
	gomega.Eventually(func(g gomega.Gomega) {
		output, err := kubectlGet(kind, name, namespace, "")
		if err != nil {
			g.Expect(err.Error()).To(gomega.ContainSubstring("NotFound"))
		} else {
			g.Expect(output).To(gomega.BeEmpty())
		}
	}).WithTimeout(timeout).WithPolling(polling).Should(gomega.Succeed())
}
