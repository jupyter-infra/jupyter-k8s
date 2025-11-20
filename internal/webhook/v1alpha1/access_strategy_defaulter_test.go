/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

const modifiedValue = "modified"

var _ = Describe("AccessStrategyDefaulter", func() {
	Describe("applyAccessStrategyDefaults", func() {
		var workspace *workspacev1alpha1.Workspace
		var template *workspacev1alpha1.WorkspaceTemplate

		BeforeEach(func() {
			workspace = &workspacev1alpha1.Workspace{
				Spec: workspacev1alpha1.WorkspaceSpec{},
			}
			template = &workspacev1alpha1.WorkspaceTemplate{
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{},
			}
		})

		Context("when workspace has no access strategy", func() {
			Context("and template has default access strategy", func() {
				BeforeEach(func() {
					template.Spec.DefaultAccessStrategy = &workspacev1alpha1.AccessStrategyRef{
						Name:      "default-strategy",
						Namespace: "default",
					}
				})

				It("should apply template's default access strategy", func() {
					applyAccessStrategyDefaults(workspace, template)

					Expect(workspace.Spec.AccessStrategy).ToNot(BeNil())
					Expect(workspace.Spec.AccessStrategy.Name).To(Equal("default-strategy"))
					Expect(workspace.Spec.AccessStrategy.Namespace).To(Equal("default"))
				})

				It("should create a deep copy", func() {
					applyAccessStrategyDefaults(workspace, template)

					// Modify template's access strategy
					template.Spec.DefaultAccessStrategy.Name = modifiedValue

					// Workspace should be unaffected
					Expect(workspace.Spec.AccessStrategy.Name).To(Equal("default-strategy"))
				})
			})

			Context("and template has no default access strategy", func() {
				It("should not modify workspace access strategy", func() {
					applyAccessStrategyDefaults(workspace, template)

					Expect(workspace.Spec.AccessStrategy).To(BeNil())
				})
			})
		})

		Context("when workspace already has access strategy", func() {
			BeforeEach(func() {
				workspace.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
					Name:      "existing-strategy",
					Namespace: "existing",
				}
				template.Spec.DefaultAccessStrategy = &workspacev1alpha1.AccessStrategyRef{
					Name:      "template-strategy",
					Namespace: "template",
				}
			})

			It("should not override existing access strategy", func() {
				applyAccessStrategyDefaults(workspace, template)

				Expect(workspace.Spec.AccessStrategy.Name).To(Equal("existing-strategy"))
				Expect(workspace.Spec.AccessStrategy.Namespace).To(Equal("existing"))
			})
		})
	})
})
