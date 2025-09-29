package crdonly_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-ai-contrib/jupyter-k8s/test/helm"
)

// Test suite for verifying CRD-Only Helm chart resources match config resources
var _ = Describe("CRD-Only Helm Resources", func() {

	// Directories to exclude from scanning
	var excludeDirs = map[string]bool{
		"samples": true,
		"default": true,
	}

	It("should include all CRD-only resources in the Helm chart", func() {
		// Get project root directory
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		// Get all subdirectories in config except excludeDir
		configRoot := filepath.Join(rootDir, "config")
		entries, err := os.ReadDir(configRoot)
		Expect(err).NotTo(HaveOccurred(), "Failed to read config directory")

		// Collect all directory names except the excluded ones
		srcDirs := []string{}
		for _, entry := range entries {
			if entry.IsDir() && !excludeDirs[entry.Name()] {
				srcDirs = append(srcDirs, entry.Name())
			}
		}
		Expect(srcDirs).NotTo(BeEmpty(), "No directories found in config")

		// Create a map to store all resources from all source directories
		allConfigResources := make(map[helm.ResourceIdentifier]bool)
		allHelmResources := make(map[helm.ResourceIdentifier]bool)

		// Process each source directory
		for _, dirName := range srcDirs {
			configDir := filepath.Join(rootDir, "config", dirName)

			// Parse resources from config directory
			configResources, _, err := helm.ParseYAMLDirectory(configDir)
			if err != nil {
				// Skip directories that don't have readable YAML files
				continue
			}

			// Add resources to the combined map
			for res := range configResources {
				allConfigResources[res] = true
			}

			// Target directory name in the Helm templates is the same as the source directory
			targetDir := dirName

			// Parse resources from helm template output
			helmDir := filepath.Join(rootDir, "dist", "test-output-crd-only", "jupyter-k8s", "templates", targetDir)
			if _, err := os.Stat(helmDir); os.IsNotExist(err) {
				continue
			}

			helmResources, _, err := helm.ParseYAMLDirectory(helmDir)
			if err == nil {
				// Add resources to the combined map
				for res := range helmResources {
					allHelmResources[res] = true
				}
			}
		}

		// Verify we found resources in both places
		Expect(allConfigResources).NotTo(BeEmpty(), "No resources found in config directories")
		Expect(allHelmResources).NotTo(BeEmpty(), "No resources found in Helm templates")

		// Check that all config resources exist in Helm resources
		// We create a list of missing resources for better error reporting
		missingResources := []string{}
		for res := range allConfigResources {
			// Skip CRDs since those are handled differently
			if res.Kind == "CustomResourceDefinition" {
				continue
			}

			// Skip resources with special transformations
			if shouldSkipResourceCheck(res) {
				continue
			}

			if _, exists := allHelmResources[res]; !exists {
				missingResources = append(missingResources, res.String())
			}
		}

		// Output more helpful error messages by grouping by kind
		Expect(missingResources).To(BeEmpty(), "Resources from config directory missing in Helm chart: %v", missingResources)
	})
})

// shouldSkipResourceCheck returns true if the resource should be skipped in comparisons
func shouldSkipResourceCheck(res helm.ResourceIdentifier) bool {
	// Skip resources that are transformed during template rendering

	// Skip namespace resources as they are handled differently in Helm
	if res.Kind == "Namespace" {
		return true
	}

	// Skip system namespace resources - they are transformed in Helm templates
	// In the config they often have 'system' namespace but in Helm they use release namespace
	if res.Namespace == "system" {
		return true
	}

	// Skip kube-system namespace resources
	if res.Namespace == "kube-system" {
		return true
	}

	// Skip resources with special prefixes that get transformed
	if strings.HasPrefix(res.Name, "manager-") {
		return true
	}

	if strings.HasPrefix(res.Name, "controller-manager-") {
		return true
	}

	// Skip Role and ClusterRole resources as they often get transformed
	if res.Kind == "Role" || res.Kind == "ClusterRole" ||
		res.Kind == "RoleBinding" || res.Kind == "ClusterRoleBinding" {
		return true
	}

	// Skip ServiceAccount resources
	if res.Kind == "ServiceAccount" {
		return true
	}

	// Skip NetworkPolicy resources
	if res.Kind == "NetworkPolicy" {
		return true
	}

	// Skip Deployment resources
	if res.Kind == "Deployment" {
		return true
	}

	// Skip ServiceMonitor resources
	if res.Kind == "ServiceMonitor" {
		return true
	}

	// Explicitly skip common Kubernetes resources that are transformed
	skipResources := []string{
		"leader-election-role",
		"leader-election-rolebinding",
		"metrics-reader",
		"manager-role",
		"metrics-auth-role",
		"metrics-auth-rolebinding",
		"manager-rolebinding",
		"allow-metrics-traffic",
	}
	for _, name := range skipResources {
		if res.Name == name {
			return true
		}
	}

	return false
}
