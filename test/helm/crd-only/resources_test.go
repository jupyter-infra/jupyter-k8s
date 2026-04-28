/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
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
		"samples":     true,
		"default":     true,
		"jwt-rotator": true, // opt-in via extensionApi.jwtSecret.enable
	}

	It("should include all CRD-only resources in the Helm chart", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		// Collect resources from config/ subdirectories (except excluded ones)
		configRoot := filepath.Join(rootDir, "config")
		configEntries, err := os.ReadDir(configRoot)
		Expect(err).NotTo(HaveOccurred(), "Failed to read config directory")

		allConfigResources := make(map[helm.ResourceIdentifier]bool)
		for _, entry := range configEntries {
			if !entry.IsDir() || excludeDirs[entry.Name()] {
				continue
			}
			configResources, _, err := helm.ParseYAMLDirectory(filepath.Join(configRoot, entry.Name()))
			if err != nil {
				continue
			}
			for res := range configResources {
				allConfigResources[res] = true
			}
		}

		// Collect resources from ALL helm template subdirectories
		helmRoot := filepath.Join(rootDir, "dist", "test-output-crd-only", "jupyter-k8s", "templates")
		allHelmResources := make(map[helm.ResourceIdentifier]bool)
		helmEntries, err := os.ReadDir(helmRoot)
		Expect(err).NotTo(HaveOccurred(), "Failed to read helm templates directory")
		for _, entry := range helmEntries {
			if !entry.IsDir() {
				continue
			}
			helmResources, _, err := helm.ParseYAMLDirectory(filepath.Join(helmRoot, entry.Name()))
			if err != nil {
				continue
			}
			for res := range helmResources {
				allHelmResources[res] = true
			}
		}

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

		missingResources := []string{}

		println("Helm keys -----")
		for key := range allHelmKeys {
			println(key)
		}
		println("-----")

		for key := range allConfigKeys {
			if _, exists := allHelmKeys[key]; exists {
				continue
			}

			parts := strings.Split(key, ":")

			// Helm chart prefixes resource names with fullnameOverride ("jupyter-k8s").
			prefixedKey := fmt.Sprintf("%s:jupyter-k8s-%s", parts[0], parts[1])
			if _, prefixedExists := allHelmKeys[prefixedKey]; prefixedExists {
				continue
			}

			if !shouldSkipResourceCheck(parts[0], parts[1]) {
				missingResources = append(missingResources, key)
			}
		}

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
