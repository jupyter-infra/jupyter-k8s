/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	webhookconst "github.com/jupyter-ai-contrib/jupyter-k8s/internal/webhook"
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
			templateRef := "existing-template"
			workspace.Spec.TemplateRef = &templateRef

			err := templateGetter.ApplyTemplateName(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(*workspace.Spec.TemplateRef).To(Equal("existing-template"))
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
					Name: "default-template",
					Labels: map[string]string{
						webhookconst.DefaultClusterTemplateLabel: "true",
					},
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName: "Default Template",
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			err := templateGetter.ApplyTemplateName(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Spec.TemplateRef).NotTo(BeNil())
			Expect(*workspace.Spec.TemplateRef).To(Equal("default-template"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, template)).To(Succeed())
		})

		It("should return error when multiple default templates exist", func() {
			// Create two default templates
			template1 := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default-template-1",
					Labels: map[string]string{
						webhookconst.DefaultClusterTemplateLabel: "true",
					},
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName: "Default Template 1",
				},
			}
			template2 := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default-template-2",
					Labels: map[string]string{
						webhookconst.DefaultClusterTemplateLabel: "true",
					},
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName: "Default Template 2",
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
