/*
MIT License

Copyright (c) 2025 jupyter-ai-contrib

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("Template Immutability", func() {
	Context("Workspace templateRef CEL Immutability", func() {
		var (
			ctx       context.Context
			template1 *workspacev1alpha1.WorkspaceTemplate
			template2 *workspacev1alpha1.WorkspaceTemplate
			workspace *workspacev1alpha1.Workspace
		)

		BeforeEach(func() {
			ctx = context.Background()

			template1 = &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "immutable-template-1",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Template 1",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template1)).To(Succeed())

			template2 = &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "immutable-template-2",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Template 2",
					DefaultImage: "quay.io/jupyter/scipy-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template2)).To(Succeed())

			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "immutable-test-workspace",
					Namespace: "default",
					Labels: map[string]string{
						"workspace.jupyter.org/template": template1.Name,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Immutable Test",
					TemplateRef: &template1.Name,
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
		})

		AfterEach(func() {
			if workspace != nil {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workspace))).To(Succeed())
			}
			if template1 != nil {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template1))).To(Succeed())
			}
			if template2 != nil {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template2))).To(Succeed())
			}
		})

		It("should allow creating workspace with templateRef", func() {
			updatedWorkspace := &workspacev1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())
			Expect(*updatedWorkspace.Spec.TemplateRef).To(Equal(template1.Name))
		})

		It("should reject changing templateRef via CEL validation", func() {
			updatedWorkspace := &workspacev1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())

			// Try to change templateRef to template2
			updatedWorkspace.Spec.TemplateRef = &template2.Name
			err := k8sClient.Update(ctx, updatedWorkspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("templateRef is immutable"))
		})

		It("should allow updating other fields without changing templateRef", func() {
			updatedWorkspace := &workspacev1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())

			// Change displayName but keep same templateRef
			updatedWorkspace.Spec.DisplayName = "Updated Display Name"
			err := k8sClient.Update(ctx, updatedWorkspace)
			Expect(err).NotTo(HaveOccurred())

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())
			Expect(updatedWorkspace.Spec.DisplayName).To(Equal("Updated Display Name"))
			Expect(*updatedWorkspace.Spec.TemplateRef).To(Equal(template1.Name))
		})
	})

	Context("Template Deletion Protection", func() {
		var (
			ctx              context.Context
			templateResolver *TemplateResolver
			template         *workspacev1alpha1.WorkspaceTemplate
			workspace        *workspacev1alpha1.Workspace
		)

		BeforeEach(func() {
			ctx = context.Background()
			templateResolver = NewTemplateResolver(k8sClient)

			template = &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "protected-template",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Protected Template",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "protection-test-workspace",
					Namespace: "default",
					Labels: map[string]string{
						"workspace.jupyter.org/template": template.Name,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Protection Test",
					TemplateRef: &template.Name,
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
		})

		AfterEach(func() {
			if workspace != nil {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workspace))).To(Succeed())
			}
			if template != nil {
				updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate); err == nil {
					controllerutil.RemoveFinalizer(updatedTemplate, templateFinalizerName)
					Expect(client.IgnoreNotFound(k8sClient.Update(ctx, updatedTemplate))).To(Succeed())
				}
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}
		})

		It("should list workspaces using a template", func() {
			workspaces, err := templateResolver.ListWorkspacesUsingTemplate(ctx, template.Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspaces).To(HaveLen(1))
			Expect(workspaces[0].Name).To(Equal(workspace.Name))
		})

		It("should return empty list when no workspaces use template", func() {
			workspaces, err := templateResolver.ListWorkspacesUsingTemplate(ctx, "nonexistent-template")
			Expect(err).NotTo(HaveOccurred())
			Expect(workspaces).To(BeEmpty())
		})

		It("should detect workspaces using template for deletion protection", func() {
			// Add finalizer to simulate what controller would do
			controllerutil.AddFinalizer(template, templateFinalizerName)
			Expect(k8sClient.Update(ctx, template)).To(Succeed())

			// Verify we can detect workspaces using the template
			workspaces, err := templateResolver.ListWorkspacesUsingTemplate(ctx, template.Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspaces).To(HaveLen(1))

			// Controller logic would check this before allowing deletion
			Expect(workspaces).ToNot(BeEmpty())
		})

		It("should detect when no workspaces use template for deletion", func() {
			// Create a template that no workspace uses
			unusedTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "unused-template",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Unused Template",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, unusedTemplate)).To(Succeed())
			defer func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, unusedTemplate))).To(Succeed())
			}()

			// Verify no workspaces use this template
			workspaces, err := templateResolver.ListWorkspacesUsingTemplate(ctx, unusedTemplate.Name)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspaces).To(BeEmpty())

			// Controller would allow deletion since no workspaces reference it
		})
	})

	Context("Template Spec CEL Immutability", func() {
		It("should reject template spec modification via CEL validation", func() {
			ctx := context.Background()
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "cel-immutable-template",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "CEL Immutability Test",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
					ResourceBounds: &workspacev1alpha1.ResourceBounds{
						CPU: &workspacev1alpha1.ResourceRange{
							Min: resource.MustParse("100m"),
							Max: resource.MustParse("2"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())
			defer func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}()

			// Attempt to modify the template spec (should fail due to CEL validation)
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
			updatedTemplate.Spec.DisplayName = "Modified Display Name"
			err := k8sClient.Update(ctx, updatedTemplate)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("template spec is immutable after creation"))
		})

		It("should allow metadata updates while spec remains immutable", func() {
			ctx := context.Background()
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: "metadata-mutable-template",
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Metadata Test",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())
			defer func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}()

			// Metadata changes (labels, annotations) should be allowed
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())
			if updatedTemplate.Labels == nil {
				updatedTemplate.Labels = make(map[string]string)
			}
			updatedTemplate.Labels["new-label"] = "test"
			Expect(k8sClient.Update(ctx, updatedTemplate)).To(Succeed())

			// Verify label was added
			verifyTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), verifyTemplate)).To(Succeed())
			Expect(verifyTemplate.Labels["new-label"]).To(Equal("test"))
		})
	})
})
