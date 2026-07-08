/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("Metadata Validator", func() {
	Describe("validateLabelRequirements", func() {
		var (
			workspace *workspacev1alpha1.Workspace
			template  *workspacev1alpha1.WorkspaceTemplate
		)

		BeforeEach(func() {
			workspace = &workspacev1alpha1.Workspace{}
			workspace.Labels = map[string]string{}
			template = &workspacev1alpha1.WorkspaceTemplate{}
		})

		Context("when template has no LabelRequirements", func() {
			It("should return nil", func() {
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(BeNil())
			})
		})

		Context("required labels", func() {
			BeforeEach(func() {
				required := true
				template.Spec.LabelRequirements = []workspacev1alpha1.LabelRequirement{
					{Key: testLabelKeyEnv, Required: &required},
				}
			})

			It("should fail when required label is missing", func() {
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeLabelRequired))
			})

			It("should pass when required label is present", func() {
				workspace.Labels[testLabelKeyEnv] = "anything"
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(BeEmpty())
			})
		})

		Context("regex validation", func() {
			BeforeEach(func() {
				template.Spec.LabelRequirements = []workspacev1alpha1.LabelRequirement{
					{Key: testLabelKeyEnv, Regex: "^(production|staging)$"},
				}
			})

			It("should pass when value matches regex", func() {
				workspace.Labels[testLabelKeyEnv] = testEnvProduction
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(BeEmpty())
			})

			It("should fail when value doesn't match regex", func() {
				workspace.Labels[testLabelKeyEnv] = testEnvDevelopment
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeLabelRegexMismatch))
				Expect(violations[0].Actual).To(Equal(testEnvDevelopment))
			})

			It("should skip validation when label is absent", func() {
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(BeEmpty())
			})
		})

		Context("required with regex", func() {
			BeforeEach(func() {
				required := true
				template.Spec.LabelRequirements = []workspacev1alpha1.LabelRequirement{
					{Key: testLabelKeyEnv, Required: &required, Regex: "^(production|staging)$"},
				}
			})

			It("should fail when missing", func() {
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeLabelRequired))
			})

			It("should fail when present but doesn't match regex", func() {
				workspace.Labels[testLabelKeyEnv] = testEnvDevelopment
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeLabelRegexMismatch))
			})

			It("should pass when present and matches regex", func() {
				workspace.Labels[testLabelKeyEnv] = "staging"
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(BeEmpty())
			})
		})
	})
})
