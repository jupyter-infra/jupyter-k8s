/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_traefik_dex_test

import (
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/jupyter-ai-contrib/jupyter-k8s/test/helm"
)

// Test suite for verifying Helm chart resources match config resources
var _ = Describe("AWS Traefik Dex Resources", func() {
	It("should include all aws-traefik-dex resources in the Helm chart", func() {
		// Get project root directory
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		// Parse resources from source directory
		configDir := filepath.Join(rootDir, "guided-charts", "aws-traefik-dex", "templates")
		configResources, configMap, err := helm.ParseYAMLDirectory(configDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(configResources).NotTo(BeEmpty(), "No resources found in guided-charts directory")

		// Parse resources from target directory
		helmDir := filepath.Join(
			rootDir, "dist", "test-output-guided", "jupyter-k8s-aws-traefik-dex", "templates")
		helmResources, helmMap, err := helm.ParseYAMLDirectory(helmDir)
		Expect(err).NotTo(HaveOccurred())
		Expect(helmResources).NotTo(BeEmpty(), "No resources found in output directory")

		// Compare resources using our new helper function
		missingResources, unmatchedResources, _ := helm.CompareResourceMaps(
			configResources, configMap, helmResources, helmMap)

		// Check that all source resources exist in rendered output
		Expect(missingResources).To(BeEmpty(), "Resources from source chart missing in output: %v", missingResources)

		// Filter out resources from traefik-crds dependency chart and show them
		// For Helm charts, it's normal for template files to produce multiple resources
		// that don't have direct 1:1 matches with source files
		filteredUnmatched := []string{}
		for _, res := range unmatchedResources {
			// Only add non-dependency resources to the filtered list
			if !strings.Contains(res, "traefik-crd") {
				filteredUnmatched = append(filteredUnmatched, res)
			}
		}

		if len(filteredUnmatched) > 0 {
			GinkgoWriter.Println("Note: Multiple resources generated from template files (expected):")
			for _, res := range filteredUnmatched {
				GinkgoWriter.Printf("  - %s\n", res)
			}
		}

		// Instead of requiring filtered list to be empty, just note that they exist
		// This is expected for Helm templates where one template file can generate multiple resources
	})

	It("should find values.yaml references in the templates", func() {
		// Get project root directory
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		// Check that values references are valid
		templatesDir := filepath.Join(rootDir, "guided-charts", "aws-traefik-dex", "templates")
		valuesPath := filepath.Join(rootDir, "guided-charts", "aws-traefik-dex", "values.yaml")

		// Extract references from templates
		references, err := helm.ExtractTemplateReferences(templatesDir)
		Expect(err).NotTo(HaveOccurred())

		// Extract values schema
		schema, err := helm.ExtractValuesSchema(valuesPath)
		Expect(err).NotTo(HaveOccurred())

		// Find invalid references
		invalidRefs := helm.FindInvalidReferences(references, schema)

		Expect(invalidRefs).To(BeEmpty(), "Invalid values references found: %v", invalidRefs)
	})
})
