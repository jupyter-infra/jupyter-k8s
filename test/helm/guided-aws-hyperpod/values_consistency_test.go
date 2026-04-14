/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package aws_hyperpod_test

import (
	"path/filepath"

	"github.com/jupyter-infra/jupyter-k8s/test/helm"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("AWS-HyperPod Values Consistency", func() {
	It("should have consistent references between aws-hyperpod templates and values.yaml", func() {
		rootDir, err := filepath.Abs("../../..")
		Expect(err).NotTo(HaveOccurred())

		valuesPath := filepath.Join(rootDir, "guided-charts/aws-hyperpod/values.yaml")
		templatesDir := filepath.Join(rootDir, "guided-charts/aws-hyperpod/templates")

		schema, err := helm.ExtractValuesSchema(valuesPath)
		Expect(err).NotTo(HaveOccurred())

		references, err := helm.ExtractTemplateReferences(templatesDir)
		Expect(err).NotTo(HaveOccurred())

		invalidRefs := helm.FindInvalidReferences(references, schema)

		Expect(invalidRefs).To(BeEmpty(), "Found template references that don't exist in values.yaml")
	})
})
