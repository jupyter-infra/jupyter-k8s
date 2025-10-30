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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("Template Resolution Without Validation", func() {
	var (
		ctx              context.Context
		templateResolver *TemplateResolver
		template         *workspacev1alpha1.WorkspaceTemplate
	)

	BeforeEach(func() {
		ctx = context.Background()
		templateResolver = NewTemplateResolver(k8sClient)

		// Create a template with bounds and constraints
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: "resolve-test-template",
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DisplayName:  "Resolve Test Template",
				DefaultImage: "default-image:v1",
				AllowedImages: []string{
					"default-image:v1",
					"allowed-image:v1",
				},
				DefaultResources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
				ResourceBounds: &workspacev1alpha1.ResourceBounds{
					CPU: &workspacev1alpha1.ResourceRange{
						Min: resource.MustParse("50m"),
						Max: resource.MustParse("1"),
					},
					Memory: &workspacev1alpha1.ResourceRange{
						Min: resource.MustParse("64Mi"),
						Max: resource.MustParse("1Gi"),
					},
				},
				PrimaryStorage: &workspacev1alpha1.StorageConfig{
					DefaultSize: resource.MustParse("1Gi"),
					MinSize:     &[]resource.Quantity{resource.MustParse("100Mi")}[0],
					MaxSize:     &[]resource.Quantity{resource.MustParse("10Gi")}[0],
				},
			},
		}
		Expect(k8sClient.Create(ctx, template)).To(Succeed())
	})

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
	})

	Describe("ResolveTemplate", func() {
		It("should return merged template without validation", func() {
			// Create workspace with valid configuration
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Valid Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
					Image:       "allowed-image:v1",
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("500m"),
						},
					},
				},
			}

			resolved, err := templateResolver.ResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).NotTo(BeNil())

			// Verify merged values
			Expect(resolved.Image).To(Equal("allowed-image:v1"))
			Expect(resolved.Resources.Requests.Cpu().String()).To(Equal("500m"))
		})

		It("should resolve even with configuration that would fail validation (no validation happens)", func() {
			// Create workspace that VIOLATES template bounds (CPU exceeds max)
			// ResolveTemplate should succeed because it doesn't validate
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Invalid Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
					Image:       "disallowed-image:v1", // Not in allowedImages
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("5"), // Exceeds max of 1
						},
					},
				},
			}

			// Should succeed because ResolveTemplate doesn't validate
			resolved, err := templateResolver.ResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred(), "ResolveTemplate should not validate")
			Expect(resolved).NotTo(BeNil())

			// Verify it returned the invalid configuration (didn't reject it)
			Expect(resolved.Image).To(Equal("disallowed-image:v1"))
			Expect(resolved.Resources.Requests.Cpu().String()).To(Equal("5"))
		})

		It("should handle missing template gracefully", func() {
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "orphan-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Orphan Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{Name: "non-existent-template"},
				},
			}

			resolved, err := templateResolver.ResolveTemplate(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get WorkspaceTemplate"))
			Expect(resolved).To(BeNil())
		})

		It("should return nil when workspace has no templateRef", func() {
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-template-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "No Template Workspace",
					TemplateRef: nil,
					Image:       "direct-image:v1",
				},
			}

			resolved, err := templateResolver.ResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).To(BeNil(), "should return nil for workspace without template")
		})
	})

	Describe("applyOverridesWithoutValidation", func() {
		It("should apply all workspace overrides", func() {
			// Create a base resolved template
			resolved := &ResolvedTemplate{
				Image: "default-image:v1",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				NodeSelector: map[string]string{"node-type": "default"},
				StorageConfiguration: &workspacev1alpha1.StorageConfig{
					DefaultSize: resource.MustParse("1Gi"),
				},
			}

			workspace := &workspacev1alpha1.Workspace{
				Spec: workspacev1alpha1.WorkspaceSpec{
					Image: "override-image:v1",
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("500m"),
						},
					},
					Storage: &workspacev1alpha1.StorageSpec{
						Size: resource.MustParse("5Gi"),
					},
					NodeSelector: map[string]string{"node-type": "custom"},
				},
			}

			templateResolver.applyOverridesWithoutValidation(resolved, workspace)

			// Verify all overrides applied
			Expect(resolved.Image).To(Equal("override-image:v1"))
			Expect(resolved.Resources.Requests.Cpu().String()).To(Equal("500m"))
			Expect(resolved.StorageConfiguration.DefaultSize.String()).To(Equal("5Gi"))
			Expect(resolved.NodeSelector["node-type"]).To(Equal("custom"))
		})

		It("should handle partial overrides", func() {
			resolved := &ResolvedTemplate{
				Image: "default-image:v1",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("100m"),
					},
				},
			}

			// Workspace only overrides image, not resources
			workspace := &workspacev1alpha1.Workspace{
				Spec: workspacev1alpha1.WorkspaceSpec{
					Image: "partial-override:v1",
				},
			}

			templateResolver.applyOverridesWithoutValidation(resolved, workspace)

			// Image overridden, resources unchanged
			Expect(resolved.Image).To(Equal("partial-override:v1"))
			Expect(resolved.Resources.Requests.Cpu().String()).To(Equal("100m"))
		})

		It("should handle empty overrides gracefully", func() {
			resolved := &ResolvedTemplate{
				Image: "default-image:v1",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU: resource.MustParse("100m"),
					},
				},
			}

			// Workspace with no overrides
			workspace := &workspacev1alpha1.Workspace{
				Spec: workspacev1alpha1.WorkspaceSpec{},
			}

			templateResolver.applyOverridesWithoutValidation(resolved, workspace)

			// Nothing should change
			Expect(resolved.Image).To(Equal("default-image:v1"))
			Expect(resolved.Resources.Requests.Cpu().String()).To(Equal("100m"))
		})
	})

	Describe("Comparison with ValidateAndResolveTemplate", func() {
		It("ValidateAndResolveTemplate should reject invalid configuration while ResolveTemplate accepts it", func() {
			// Create workspace that violates bounds
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "comparison-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Comparison Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{Name: template.Name},
					Image:       "disallowed-image:v999", // Not in allowedImages
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("10"), // Exceeds max
						},
					},
				},
			}

			// ResolveTemplate should succeed (no validation)
			resolved, err := templateResolver.ResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(resolved).NotTo(BeNil())

			// ValidateAndResolveTemplate should return violations
			result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Valid).To(BeFalse(), "should detect violations")
			Expect(result.Violations).NotTo(BeEmpty())
		})
	})
})
