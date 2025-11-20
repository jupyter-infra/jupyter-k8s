/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package crdonly_test

import (
	"path/filepath"

	"github.com/jupyter-infra/jupyter-k8s/test/helm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CRD-Only Values Consistency", func() {
	It("should have consistent references between templates and values.yaml", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		valuesPath := filepath.Join(rootDir, "dist/chart/values.yaml")
		templatesDir := filepath.Join(rootDir, "dist/chart/templates")

		// Extract schema from values.yaml
		schema, err := helm.ExtractValuesSchema(valuesPath)
		Expect(err).NotTo(HaveOccurred())

		// Extract references from templates
		references, err := helm.ExtractTemplateReferences(templatesDir)
		Expect(err).NotTo(HaveOccurred())

		// Check each reference against schema, ignoring known missing values
		// These are values used in templates but not defined in values.yaml
		// They are typically used as optional fields that default to empty/nil
		ignorePaths := []string{
			"controllerManager.pod",
			"controllerManager.pod.labels",
			"controllerManager.container.env",
			"controllerManager.serviceAccount",
			"controllerManager.serviceAccount.annotations",
		}
		invalidRefs := helm.FindInvalidReferences(references, schema, ignorePaths...)

		// Report any invalid references
		Expect(invalidRefs).To(BeEmpty(), "Found template references that don't exist in values.yaml")
	})
})
