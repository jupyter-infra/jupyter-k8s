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
					{Key: "env", Required: &required},
				}
			})

			It("should fail when required label is missing", func() {
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeLabelRequired))
			})

			It("should pass when required label is present", func() {
				workspace.Labels["env"] = "anything"
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(BeEmpty())
			})
		})

		Context("regex validation", func() {
			BeforeEach(func() {
				template.Spec.LabelRequirements = []workspacev1alpha1.LabelRequirement{
					{Key: "env", Regex: "^(production|staging)$"},
				}
			})

			It("should pass when value matches regex", func() {
				workspace.Labels["env"] = "production"
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(BeEmpty())
			})

			It("should fail when value doesn't match regex", func() {
				workspace.Labels["env"] = "development"
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeLabelRegexMismatch))
				Expect(violations[0].Actual).To(Equal("development"))
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
					{Key: "env", Required: &required, Regex: "^(production|staging)$"},
				}
			})

			It("should fail when missing", func() {
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeLabelRequired))
			})

			It("should fail when present but doesn't match regex", func() {
				workspace.Labels["env"] = "development"
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeLabelRegexMismatch))
			})

			It("should pass when present and matches regex", func() {
				workspace.Labels["env"] = "staging"
				violations := validateLabelRequirements(workspace, template)
				Expect(violations).To(BeEmpty())
			})
		})
	})

	Describe("validateForbiddenLabels", func() {
		var (
			workspace *workspacev1alpha1.Workspace
			template  *workspacev1alpha1.WorkspaceTemplate
		)

		BeforeEach(func() {
			workspace = &workspacev1alpha1.Workspace{}
			workspace.Labels = map[string]string{}
			template = &workspacev1alpha1.WorkspaceTemplate{}
			template.Spec.ForbiddenLabels = []string{"debug", "bypass-security"}
		})

		It("should pass when no forbidden labels are present", func() {
			workspace.Labels["env"] = "production"
			violations := validateForbiddenLabels(workspace, template)
			Expect(violations).To(BeEmpty())
		})

		It("should fail when a forbidden label is present", func() {
			workspace.Labels["debug"] = "true"
			violations := validateForbiddenLabels(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeForbiddenLabel))
			Expect(violations[0].Field).To(Equal("metadata.labels[debug]"))
		})

		It("should return no violations when template has no forbidden labels", func() {
			template.Spec.ForbiddenLabels = nil
			workspace.Labels["debug"] = "true"
			violations := validateForbiddenLabels(workspace, template)
			Expect(violations).To(BeEmpty())
		})
	})
})
