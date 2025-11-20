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
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	webhookconst "github.com/jupyter-infra/jupyter-k8s/internal/webhook"
)

var _ = Describe("TemplateGetter", func() {
	var (
		templateGetter *TemplateGetter
		workspace      *workspacev1alpha1.Workspace
		ctx            context.Context
	)

	BeforeEach(func() {
		templateGetter = NewTemplateGetter(k8sClient)
		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Image:         "jupyter/base-notebook:latest",
				DesiredStatus: "Running",
			},
		}
		ctx = context.Background()
	})

	Context("ApplyTemplateName", func() {
		It("should skip if workspace already has templateRef", func() {
			workspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
				Name: "existing-template",
			}

			err := templateGetter.ApplyTemplateName(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Spec.TemplateRef.Name).To(Equal("existing-template"))
		})

		It("should continue without error if no default template exists", func() {
			err := templateGetter.ApplyTemplateName(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Spec.TemplateRef).To(BeNil())
		})

		It("should set templateRef to default template when one exists", func() {
			// Create a default template
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-template",
					Namespace: "default",
					Labels: map[string]string{
						webhookconst.DefaultClusterTemplateLabel: "true",
					},
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Default Template",
					DefaultImage: "jupyter/base-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			err := templateGetter.ApplyTemplateName(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Spec.TemplateRef).NotTo(BeNil())
			Expect(workspace.Spec.TemplateRef.Name).To(Equal("default-template"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, template)).To(Succeed())
		})

		It("should return error when multiple default templates exist", func() {
			// Create two default templates
			template1 := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-template-1",
					Namespace: "default",
					Labels: map[string]string{
						webhookconst.DefaultClusterTemplateLabel: "true",
					},
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Default Template 1",
					DefaultImage: "jupyter/base-notebook:latest",
				},
			}
			template2 := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-template-2",
					Namespace: "default",
					Labels: map[string]string{
						webhookconst.DefaultClusterTemplateLabel: "true",
					},
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Default Template 2",
					DefaultImage: "jupyter/base-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template1)).To(Succeed())
			Expect(k8sClient.Create(ctx, template2)).To(Succeed())

			err := templateGetter.ApplyTemplateName(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("multiple templates found"))
			Expect(err.Error()).To(ContainSubstring("default-template-1"))
			Expect(err.Error()).To(ContainSubstring("default-template-2"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, template1)).To(Succeed())
			Expect(k8sClient.Delete(ctx, template2)).To(Succeed())
		})
	})

	Context("getTemplateNames", func() {
		It("should return empty slice for empty input", func() {
			names := getTemplateNames([]workspacev1alpha1.WorkspaceTemplate{})
			Expect(names).To(BeEmpty())
		})

		It("should return template names", func() {
			templates := []workspacev1alpha1.WorkspaceTemplate{
				{ObjectMeta: metav1.ObjectMeta{Name: "template-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "template-2"}},
			}
			names := getTemplateNames(templates)
			Expect(names).To(Equal([]string{"template-1", "template-2"}))
		})
	})
})
