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
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("Template Validation Caching", func() {
	Context("Cache Hit Scenarios", func() {
		var (
			ctx              context.Context
			templateResolver *TemplateResolver
			templateName     string
			template         *workspacesv1alpha1.WorkspaceTemplate
			workspace        *workspacesv1alpha1.Workspace
		)

		BeforeEach(func() {
			ctx = context.Background()
			templateResolver = NewTemplateResolver(k8sClient)
			templateName = "cache-test-template"

			template = &workspacesv1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: templateName,
				},
				Spec: workspacesv1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Cache Test Template",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
					AllowedImages: []string{
						"quay.io/jupyter/minimal-notebook:latest",
					},
					ResourceBounds: &workspacesv1alpha1.ResourceBounds{
						CPU: &workspacesv1alpha1.ResourceRange{
							Min: resource.MustParse("100m"),
							Max: resource.MustParse("2"),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())

			workspace = &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cache-test-workspace",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					DisplayName: "Cache Test",
					TemplateRef: &templateName,
				},
			}
		})

		AfterEach(func() {
			if template != nil {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}
		})

		It("should skip validation when resourceVersions match", func() {
			// First validation - no cache
			result1, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(result1.Valid).To(BeTrue())

			// Update cache annotations to simulate successful validation
			workspace.Annotations = map[string]string{
				AnnotationValidatedTemplateRV:          template.ResourceVersion,
				AnnotationValidatedTemplateGeneration:  strconv.FormatInt(template.Generation, 10),
				AnnotationValidatedWorkspaceGeneration: strconv.FormatInt(workspace.Generation, 10),
			}

			// Second validation - should hit cache
			result2, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(result2.Valid).To(BeTrue())
		})

		It("should re-validate when workspace spec changes", func() {
			// Set up cache with old generation
			workspace.Annotations = map[string]string{
				AnnotationValidatedTemplateRV:          template.ResourceVersion,
				AnnotationValidatedTemplateGeneration:  strconv.FormatInt(template.Generation, 10),
				AnnotationValidatedWorkspaceGeneration: "1", // Old generation
			}

			workspace.Generation = 2 // Simulate spec change

			result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Valid).To(BeTrue())
		})

		It("should re-validate when template spec changes", func() {
			// Set up cache with current state
			workspace.Annotations = map[string]string{
				AnnotationValidatedTemplateRV:          template.ResourceVersion,
				AnnotationValidatedTemplateGeneration:  strconv.FormatInt(template.Generation, 10),
				AnnotationValidatedWorkspaceGeneration: strconv.FormatInt(workspace.Generation, 10),
			}

			// Modify template spec to increment generation
			template.Spec.Description = "Updated description"
			Expect(k8sClient.Update(ctx, template)).To(Succeed())

			// Refetch to get updated resourceVersion and generation
			updatedTemplate := &workspacesv1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(template), updatedTemplate)).To(Succeed())

			result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Valid).To(BeTrue())
		})

		It("should handle missing cache annotations gracefully", func() {
			workspace.Annotations = nil

			result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Valid).To(BeTrue())
		})

		It("should handle corrupted cache annotations gracefully", func() {
			workspace.Annotations = map[string]string{
				AnnotationValidatedTemplateRV:          template.ResourceVersion,
				AnnotationValidatedTemplateGeneration:  "not-a-number",
				AnnotationValidatedWorkspaceGeneration: strconv.FormatInt(workspace.Generation, 10),
			}

			result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Valid).To(BeTrue())
		})
	})

	Context("Cache Update", func() {
		It("should set cache annotations correctly", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-workspace",
					Namespace:       "default",
					ResourceVersion: "workspace-rv-123",
					Generation:      3,
				},
			}

			template := &workspacesv1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-template",
					ResourceVersion: "template-rv-456",
					Generation:      5,
				},
			}

			resolver := NewTemplateResolver(k8sClient)
			resolver.UpdateValidationCache(workspace, template)

			Expect(workspace.Annotations).NotTo(BeNil())
			Expect(workspace.Annotations[AnnotationValidatedTemplateRV]).To(Equal("template-rv-456"))
			Expect(workspace.Annotations[AnnotationValidatedTemplateGeneration]).To(Equal("5"))
			Expect(workspace.Annotations[AnnotationValidatedWorkspaceGeneration]).To(Equal("3"))
		})

		It("should not overwrite existing annotations", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-workspace",
					Namespace:       "default",
					ResourceVersion: "workspace-rv-123",
					Annotations: map[string]string{
						"custom.annotation": "preserve-me",
					},
				},
			}

			template := &workspacesv1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-template",
					ResourceVersion: "template-rv-456",
					Generation:      5,
				},
			}

			resolver := NewTemplateResolver(k8sClient)
			resolver.UpdateValidationCache(workspace, template)

			Expect(workspace.Annotations["custom.annotation"]).To(Equal("preserve-me"))
			Expect(workspace.Annotations[AnnotationValidatedTemplateRV]).To(Equal("template-rv-456"))
		})
	})

	Context("Cache Miss Scenarios", func() {
		var (
			ctx              context.Context
			templateResolver *TemplateResolver
			templateName     string
			template         *workspacesv1alpha1.WorkspaceTemplate
		)

		BeforeEach(func() {
			ctx = context.Background()
			templateResolver = NewTemplateResolver(k8sClient)
			templateName = "cache-miss-template"

			template = &workspacesv1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: templateName,
				},
				Spec: workspacesv1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Cache Miss Template",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())
		})

		AfterEach(func() {
			if template != nil {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}
		})

		It("should miss cache when annotations are incomplete", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "incomplete-cache-workspace",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationValidatedTemplateRV: template.ResourceVersion,
						// Missing generation and workspace RV
					},
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					TemplateRef: &templateName,
				},
			}

			cacheResult := templateResolver.checkValidationCache(ctx, workspace, templateName)
			Expect(cacheResult.Hit).To(BeFalse())
			Expect(cacheResult.MissReason).To(Equal("missing cache annotations"))
		})

		It("should miss cache when template deleted", func() {
			nonExistentTemplate := "deleted-template"
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deleted-template-workspace",
					Namespace: "default",
					Annotations: map[string]string{
						AnnotationValidatedTemplateRV:          "old-rv",
						AnnotationValidatedTemplateGeneration:  "1",
						AnnotationValidatedWorkspaceGeneration: "2",
					},
					ResourceVersion: "workspace-rv",
					Generation:      2,
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					TemplateRef: &nonExistentTemplate,
				},
			}

			cacheResult := templateResolver.checkValidationCache(ctx, workspace, nonExistentTemplate)
			Expect(cacheResult.Hit).To(BeFalse())
			Expect(cacheResult.MissReason).To(Equal("failed to fetch template for cache check"))
		})
	})
})
