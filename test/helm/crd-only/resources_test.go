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

package crdonly_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-infra/jupyter-k8s/test/helm"
)

// Test suite for verifying CRD-Only Helm chart resources match config resources
var _ = Describe("CRD-Only Helm Resources", func() {

	// Directories to exclude from scanning
	var excludeDirs = map[string]bool{
		"samples":         true,
		"default":         true,
		"samples_routing": true,
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

		allConfigKeys := make(map[string]bool)
		allHelmKeys := make(map[string]bool)

		for r := range allConfigResources {
			key := fmt.Sprintf("%s:%s", r.Kind, r.Name)
			allConfigKeys[key] = true
		}
		for r := range allHelmResources {
			key := fmt.Sprintf("%s:%s", r.Kind, r.Name)
			allHelmKeys[key] = true
		}

		// Check that all config resources exist in Helm resources
		// We create a list of missing resources for better error reporting
		missingResources := []string{}

		// keepMe: helpful for debug when test fail
		println("Helm keys -----")
		for key := range allHelmKeys {
			println(key)
		}
		println("-----")

		for key := range allConfigKeys {
			// first: lookup exact resource name (will match CRDs)
			if _, exists := allHelmKeys[key]; !exists {
				parts := strings.Split(key, ":")

				// second: if not found, lookup with 'jupyter-k8s-' prefix
				prefixedKey := fmt.Sprintf("%s:jupyter-k8s-%s", parts[0], parts[1])
				if _, prefixedExists := allHelmKeys[prefixedKey]; !prefixedExists {

					// third: if still not found, check resources that are not translated
					if !shouldSkipResourceCheck(parts[0], parts[1]) {
						missingResources = append(missingResources, key)
					}
				}
			}
		}

		// Output more helpful error messages by grouping by kind
		Expect(missingResources).To(BeEmpty(), "Resources from config directory missing in Helm chart: %v", missingResources)
	})
})

// shouldSkipResourceCheck returns true if the resource should be skipped in comparisons

func shouldSkipResourceCheck(resKind string, resName string) bool {
	switch resKind {
	case "Namespace":
		return true
	case "NetworkPolicy":
		// FIXME: consider enabling network policy in /config/default/kustomization.yaml
		return true
	case "ServiceMonitor":
		return true
	case "Service":
		if resName == "controller-manager" {
			return true
		}
		return false
	case "CustomResourceDefinition":
		if strings.Contains(resName, "connection.workspace.jupyter.org") {
			return true
		}
		return false
	default:
		return false
	}
}
