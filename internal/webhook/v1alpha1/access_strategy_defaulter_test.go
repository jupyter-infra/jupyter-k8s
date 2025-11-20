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

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
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
