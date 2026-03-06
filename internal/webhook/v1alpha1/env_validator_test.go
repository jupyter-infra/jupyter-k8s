/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("EnvValidator", func() {
	var (
		workspace *workspacev1alpha1.Workspace
		template  *workspacev1alpha1.WorkspaceTemplate
	)

	BeforeEach(func() {
		workspace = &workspacev1alpha1.Workspace{}
		template = &workspacev1alpha1.WorkspaceTemplate{}
	})

	Context("when template has no EnvRequirements", func() {
		It("should return nil", func() {
			violations := validateEnvRequirements(workspace, template)
			Expect(violations).To(BeNil())
		})
	})

	Context("required env vars", func() {
		BeforeEach(func() {
			required := true
			template.Spec.EnvRequirements = []workspacev1alpha1.EnvRequirement{
				{Name: "AWS_REGION", Required: &required},
			}
		})

		It("should fail when required env is missing", func() {
			violations := validateEnvRequirements(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeEnvRequired))
		})

		It("should pass when required env is present", func() {
			workspace.Spec.Env = []corev1.EnvVar{{Name: "AWS_REGION", Value: "us-west-2"}}
			violations := validateEnvRequirements(workspace, template)
			Expect(violations).To(BeEmpty())
		})
	})

	Context("regex validation", func() {
		BeforeEach(func() {
			template.Spec.EnvRequirements = []workspacev1alpha1.EnvRequirement{
				{Name: "ENV", Regex: "^(prod|staging)$"},
			}
		})

		It("should pass when value matches regex", func() {
			workspace.Spec.Env = []corev1.EnvVar{{Name: "ENV", Value: "prod"}}
			violations := validateEnvRequirements(workspace, template)
			Expect(violations).To(BeEmpty())
		})

		It("should fail when value doesn't match regex", func() {
			workspace.Spec.Env = []corev1.EnvVar{{Name: "ENV", Value: "dev"}}
			violations := validateEnvRequirements(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeEnvRegexMismatch))
			Expect(violations[0].Actual).To(Equal("dev"))
		})

		It("should skip validation when env var is absent", func() {
			violations := validateEnvRequirements(workspace, template)
			Expect(violations).To(BeEmpty())
		})
	})

	Context("required with regex", func() {
		BeforeEach(func() {
			required := true
			template.Spec.EnvRequirements = []workspacev1alpha1.EnvRequirement{
				{Name: "REGION", Required: &required, Regex: "^us-"},
			}
		})

		It("should fail when missing", func() {
			violations := validateEnvRequirements(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeEnvRequired))
		})

		It("should fail when present but doesn't match regex", func() {
			workspace.Spec.Env = []corev1.EnvVar{{Name: "REGION", Value: "eu-west-1"}}
			violations := validateEnvRequirements(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeEnvRegexMismatch))
		})

		It("should pass when present and matches regex", func() {
			workspace.Spec.Env = []corev1.EnvVar{{Name: "REGION", Value: "us-east-1"}}
			violations := validateEnvRequirements(workspace, template)
			Expect(violations).To(BeEmpty())
		})

		It("should reject value that contains match but isn't exact match", func() {
			workspace.Spec.Env = []corev1.EnvVar{{Name: "REGION", Value: "eu-us-west-2"}}
			violations := validateEnvRequirements(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeEnvRegexMismatch))
		})
	})

	Context("invalid regex in template", func() {
		It("should return a violation for bad regex", func() {
			template.Spec.EnvRequirements = []workspacev1alpha1.EnvRequirement{
				{Name: "X", Regex: "[invalid"},
			}
			workspace.Spec.Env = []corev1.EnvVar{{Name: "X", Value: "anything"}}

			violations := validateEnvRequirements(workspace, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeEnvRegexMismatch))
			Expect(violations[0].Message).To(ContainSubstring("invalid regex"))
		})
	})

	Context("multiple violations", func() {
		It("should return all violations", func() {
			required := true
			template.Spec.EnvRequirements = []workspacev1alpha1.EnvRequirement{
				{Name: "A", Required: &required},
				{Name: "B", Required: &required},
			}

			violations := validateEnvRequirements(workspace, template)
			Expect(violations).To(HaveLen(2))
		})
	})
})
