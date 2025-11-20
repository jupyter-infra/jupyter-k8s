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
	"strings"

	. "github.com/onsi/ginkgo/v2"

	"github.com/jupyter-infra/jupyter-k8s/test/utils"
)

// kubectlGet retrieves a Kubernetes resource with optional JSONPath query
func kubectlGet(resource, name, namespace, jsonpath string) (string, error) {
	GinkgoHelper()
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
	return strings.TrimSpace(string(output)), err
}

// kubectlGetAllNamespaces retrieves resources across all namespaces
func kubectlGetAllNamespaces(resource, jsonpath string) (string, error) {
	GinkgoHelper()
	args := []string{"get", resource, "-A"}
	if jsonpath != "" {
		args = append(args, "-o", "jsonpath="+jsonpath)
	}
	cmd := exec.Command("kubectl", args...)
	output, err := utils.Run(cmd)
	return strings.TrimSpace(string(output)), err
}

// kubectlDelete deletes Kubernetes resources with options
func kubectlDelete(resource string, names []string, namespace string, opts ...string) error {
	GinkgoHelper()
	args := []string{"delete", resource}
	args = append(args, names...)
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	args = append(args, opts...)
	cmd := exec.Command("kubectl", args...)
	_, err := utils.Run(cmd)
	return err
}

// kubectlDeleteAllNamespaces deletes resources across all namespaces
func kubectlDeleteAllNamespaces(resource string, opts ...string) error {
	GinkgoHelper()
	args := []string{"delete", resource, "--all", "--all-namespaces"}
	args = append(args, opts...)
	cmd := exec.Command("kubectl", args...)
	_, err := utils.Run(cmd)
	return err
}

// kubectlApplyYAML applies YAML content via kubectl
func kubectlApplyYAML(yamlContent string, dryRun bool) error {
	GinkgoHelper()
	args := "kubectl apply -f -"
	if dryRun {
		args = "kubectl apply --dry-run=server -f -"
	}
	cmd := exec.Command("sh", "-c", "echo '"+yamlContent+"' | "+args)
	_, err := utils.Run(cmd)
	return err
}

// kubectlWait waits for a resource condition with timeout
func kubectlWait(resource, name, namespace, condition, timeout string) error {
	GinkgoHelper()
	args := []string{"wait", resource}
	if name != "" {
		args = append(args, name)
	}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	args = append(args, "--for="+condition, "--timeout="+timeout)
	cmd := exec.Command("kubectl", args...)
	_, err := utils.Run(cmd)
	return err
}

// kubectlPatch patches a Kubernetes resource
func kubectlPatch(resourceType, name, patchType, patch string) error {
	GinkgoHelper()
	cmd := exec.Command("kubectl", "patch", resourceType, name,
		"--type="+patchType, "-p", patch)
	_, err := utils.Run(cmd)
	return err
}

// getControllerPodName retrieves the name of the controller manager pod
func getControllerPodName(namespace string) (string, error) {
	GinkgoHelper()
	jsonpath := "{.items[?(@.metadata.deletionTimestamp==null)].metadata.name}"
	output, err := kubectlGet("pods", "", namespace, jsonpath)
	if err != nil {
		return "", err
	}
	return strings.Fields(output)[0], nil
}

// deleteClusterResources deletes multiple cluster-scoped resources
func deleteClusterResources(resources []string) {
	GinkgoHelper()
	for _, resource := range resources {
		cmd := exec.Command("kubectl", "delete", resource, "--ignore-not-found")
		_, _ = utils.Run(cmd)
	}
}

// diagnoseAndCleanupStuckTemplates checks for templates stuck with finalizers
// and performs emergency cleanup if needed for suite teardown.
// This function waits for controller to process finalizer removal, then diagnoses
// stuck templates and force-removes finalizers as a last resort.
func diagnoseAndCleanupStuckTemplates() {
	GinkgoHelper()

	By("checking for stuck templates with finalizers (emergency cleanup)")
	// Wait up to 30s for controller to process finalizer removal
	cmd := exec.Command("kubectl", "wait", "workspacetemplate", "--all", "--all-namespaces",
		"--for=delete", "--timeout=30s")
	if err := cmd.Run(); err != nil {
		// Templates still exist - diagnose before emergency cleanup
		cmd = exec.Command("kubectl", "get", "workspacetemplate", "-A",
			"-o", "jsonpath={.items[*].metadata.name}")
		if output, _ := utils.Run(cmd); len(output) > 0 && strings.TrimSpace(string(output)) != "" {
			templates := strings.Fields(strings.TrimSpace(string(output)))
			_, _ = GinkgoWriter.Write([]byte("\n⚠️  WARNING: Templates still exist after workspace cleanup: " + strings.Join(templates, ", ") + "\n"))

			// Diagnose: Check which workspaces reference each stuck template
			for _, tmplName := range templates {
				cmd = exec.Command("kubectl", "get", "workspace", "-A",
					"-o", "jsonpath={.items[?(@.spec.templateRef==\""+tmplName+"\")].metadata.name}")
				if wsOutput, _ := utils.Run(cmd); len(wsOutput) > 0 && strings.TrimSpace(string(wsOutput)) != "" {
					_, _ = GinkgoWriter.Write([]byte("   Template '" + tmplName + "' is referenced by workspaces: " +
						strings.TrimSpace(string(wsOutput)) + " (test leaked resources)\n"))
				} else {
					_, _ = GinkgoWriter.Write([]byte("   Template '" + tmplName + "' has NO workspace references - CONTROLLER BUG?\n"))
				}
			}

			// Emergency cleanup: Force-remove finalizers to allow suite teardown
			By("EMERGENCY: Force-removing stuck template finalizers for clean teardown")
			cmd = exec.Command("kubectl", "get", "workspacetemplate", "-A", "-o", "name")
			if templateList, err := utils.Run(cmd); err == nil && len(templateList) > 0 {
				templates := strings.Split(strings.TrimSpace(string(templateList)), "\n")
				for _, template := range templates {
					if template != "" {
						cmd = exec.Command("kubectl", "patch", template,
							"--type=json", "-p", `[{"op": "remove", "path": "/metadata/finalizers"}]`)
						_, _ = utils.Run(cmd)
					}
				}
				// Retry deletion after removing finalizers
				cmd = exec.Command("kubectl", "delete", "workspacetemplate", "--all", "--all-namespaces",
					"--ignore-not-found", "--timeout=30s")
				_, _ = utils.Run(cmd)
			}
		}
	}
}
