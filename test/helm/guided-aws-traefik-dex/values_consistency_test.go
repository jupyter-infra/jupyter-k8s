package aws_traefik_dex_test

import (
	"path/filepath"

	"github.com/jupyter-ai-contrib/jupyter-k8s/test/helm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AWS-Traefik-Dex Values Consistency", func() {
	It("should have consistent references between aws-traefik-dex templates and values.yaml", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		// Use the guided chart's values file instead of the main chart
		valuesPath := filepath.Join(rootDir, "guided-charts/aws-traefik-dex/values.yaml")
		templatesDir := filepath.Join(rootDir, "guided-charts/aws-traefik-dex/templates")

		// Extract schema from values.yaml
		schema, err := helm.ExtractValuesSchema(valuesPath)
		Expect(err).NotTo(HaveOccurred())

		// Extract references from templates
		references, err := helm.ExtractTemplateReferences(templatesDir)
		Expect(err).NotTo(HaveOccurred())

		// Check each reference against schema, ignoring known missing paths
		// baseChart is a special parent reference that is populated during deployment
		invalidRefs := helm.FindInvalidReferences(
			references, schema, "baseChart.application.imagesPullPolicy", "baseChart.application.imagesRegistry")

		// Report any invalid references
		Expect(invalidRefs).To(BeEmpty(), "Found template references that don't exist in values.yaml")
	})
})
