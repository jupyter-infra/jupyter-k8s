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
