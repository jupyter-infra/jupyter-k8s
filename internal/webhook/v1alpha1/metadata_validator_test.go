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
	Describe("validateDefaultLabels", func() {
		Context("when template has no DefaultLabels", func() {
			It("should return nil", func() {
				workspace := &workspacev1alpha1.Workspace{}
				template := &workspacev1alpha1.WorkspaceTemplate{}

				violation := validateDefaultLabels(workspace, template)
				Expect(violation).To(BeNil())
			})
		})

		Context("when template has DefaultLabels", func() {
			It("should return nil when workspace labels match", func() {
				workspace := &workspacev1alpha1.Workspace{}
				workspace.Labels = map[string]string{
					"env":  "production",
					"team": "data-science",
				}

				template := &workspacev1alpha1.WorkspaceTemplate{}
				template.Spec.DefaultLabels = map[string]string{
					"env":  "production",
					"team": "data-science",
				}

				violation := validateDefaultLabels(workspace, template)
				Expect(violation).To(BeNil())
			})

			It("should return nil when workspace has subset of default labels", func() {
				workspace := &workspacev1alpha1.Workspace{}
				workspace.Labels = map[string]string{
					"env": "production",
				}

				template := &workspacev1alpha1.WorkspaceTemplate{}
				template.Spec.DefaultLabels = map[string]string{
					"env":  "production",
					"team": "data-science",
				}

				violation := validateDefaultLabels(workspace, template)
				Expect(violation).To(BeNil())
			})

			It("should return nil when workspace has additional non-default labels", func() {
				workspace := &workspacev1alpha1.Workspace{}
				workspace.Labels = map[string]string{
					"env":     "production",
					"team":    "data-science",
					"project": "ml-pipeline",
				}

				template := &workspacev1alpha1.WorkspaceTemplate{}
				template.Spec.DefaultLabels = map[string]string{
					"env":  "production",
					"team": "data-science",
				}

				violation := validateDefaultLabels(workspace, template)
				Expect(violation).To(BeNil())
			})

			It("should return violation when workspace label value mismatches", func() {
				workspace := &workspacev1alpha1.Workspace{}
				workspace.Labels = map[string]string{
					"env": "development",
				}

				template := &workspacev1alpha1.WorkspaceTemplate{}
				template.Spec.DefaultLabels = map[string]string{
					"env": "production",
				}

				violation := validateDefaultLabels(workspace, template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(ViolationTypeDefaultLabelMismatch))
				Expect(violation.Field).To(Equal("metadata.labels[env]"))
				Expect(violation.Allowed).To(Equal("production"))
				Expect(violation.Actual).To(Equal("development"))
			})

			It("should return violation for first mismatch when multiple labels mismatch", func() {
				workspace := &workspacev1alpha1.Workspace{}
				workspace.Labels = map[string]string{
					"env":  "development",
					"team": "wrong-team",
				}

				template := &workspacev1alpha1.WorkspaceTemplate{}
				template.Spec.DefaultLabels = map[string]string{
					"env":  "production",
					"team": "data-science",
				}

				violation := validateDefaultLabels(workspace, template)
				Expect(violation).NotTo(BeNil())
				Expect(violation.Type).To(Equal(ViolationTypeDefaultLabelMismatch))
			})
		})

		Context("when workspace has no labels", func() {
			It("should return nil when template has DefaultLabels", func() {
				workspace := &workspacev1alpha1.Workspace{}

				template := &workspacev1alpha1.WorkspaceTemplate{}
				template.Spec.DefaultLabels = map[string]string{
					"env": "production",
				}

				violation := validateDefaultLabels(workspace, template)
				Expect(violation).To(BeNil())
			})
		})
	})
})
