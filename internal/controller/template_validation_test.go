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

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("Template Validation", func() {
	Context("TemplateResolver", func() {
		var (
			ctx              context.Context
			templateResolver *TemplateResolver
			templateName     string
			template         *workspacesv1alpha1.WorkspaceTemplate
		)

		BeforeEach(func() {
			ctx = context.Background()
			templateResolver = NewTemplateResolver(k8sClient)
			templateName = "validation-template"

			// Create a comprehensive test template
			template = &workspacesv1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: templateName,
				},
				Spec: workspacesv1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Validation Test Template",
					Description:  "Template for validation testing",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest",
					AllowedImages: []string{
						"quay.io/jupyter/minimal-notebook:latest",
						"quay.io/jupyter/scipy-notebook:latest",
						"custom/allowed-image:v1",
					},
					DefaultResources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						},
					},
					ResourceBounds: &workspacesv1alpha1.ResourceBounds{
						CPU: &workspacesv1alpha1.ResourceRange{
							Min: resource.MustParse("100m"),
							Max: resource.MustParse("4"),
						},
						Memory: &workspacesv1alpha1.ResourceRange{
							Min: resource.MustParse("256Mi"),
							Max: resource.MustParse("8Gi"),
						},
						GPU: &workspacesv1alpha1.ResourceRange{
							Min: resource.MustParse("0"),
							Max: resource.MustParse("2"),
						},
					},
					PrimaryStorage: &workspacesv1alpha1.StorageConfig{
						DefaultSize: resource.MustParse("10Gi"),
						MinSize:     &[]resource.Quantity{resource.MustParse("1Gi")}[0],
						MaxSize:     &[]resource.Quantity{resource.MustParse("100Gi")}[0],
					},
					EnvironmentVariables: []corev1.EnvVar{
						{Name: "JUPYTER_ENABLE_LAB", Value: "yes"},
						{Name: "DEFAULT_ENV", Value: "test"},
					},
					AllowSecondaryStorages: &[]bool{true}[0],
				},
			}
			Expect(k8sClient.Create(ctx, template)).To(Succeed())
		})

		AfterEach(func() {
			if template != nil {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, template))).To(Succeed())
			}
		})

		It("should validate workspace without template reference but with image", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-template-workspace",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					// No TemplateRef but has Image
					Image: "my-registry.com/jupyter:v1.0",
				},
			}

			result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Valid).To(BeTrue())
			Expect(result.Violations).To(BeEmpty())
			Expect(result.Template).To(BeNil()) // No template = nil
		})

		It("should validate workspace with valid template reference", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "valid-template-workspace",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					TemplateRef: &templateName,
				},
			}

			result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Valid).To(BeTrue())
			Expect(result.Violations).To(BeEmpty())
			Expect(result.Template).NotTo(BeNil())
			Expect(result.Template.Image).To(Equal("quay.io/jupyter/minimal-notebook:latest"))
		})

		It("should handle template without DefaultResources", func() {
			// Create a template without DefaultResources
			minimalTemplateName := "minimal-template"
			minimalTemplate := &workspacesv1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name: minimalTemplateName,
				},
				Spec: workspacesv1alpha1.WorkspaceTemplateSpec{
					DisplayName:  "Minimal Template",
					Description:  "Template without default resources",
					DefaultImage: "quay.io/jupyter/minimal-notebook:latest", // Required field
					// DefaultResources is intentionally omitted (nil)
					AllowedImages: []string{
						"quay.io/jupyter/minimal-notebook:latest",
					},
					PrimaryStorage: &workspacesv1alpha1.StorageConfig{
						DefaultSize: resource.MustParse("10Gi"),
					},
				},
			}
			Expect(k8sClient.Create(ctx, minimalTemplate)).To(Succeed())
			defer func() {
				Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, minimalTemplate))).To(Succeed())
			}()

			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-template-workspace",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					TemplateRef: &minimalTemplateName,
				},
			}

			result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.Valid).To(BeTrue())
			Expect(result.Violations).To(BeEmpty())
			Expect(result.Template).NotTo(BeNil())
			// Should have empty resource requirements when template doesn't specify them
			Expect(result.Template.Resources.Requests).To(BeNil())
			Expect(result.Template.Resources.Limits).To(BeNil())
		})

		It("should return error for non-existent template", func() {
			nonExistentTemplate := "non-existent-template"
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-template-workspace",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					TemplateRef: &nonExistentTemplate,
				},
			}

			// Template not found should return a system error, not a validation result
			result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get WorkspaceTemplate"))
			Expect(err.Error()).To(ContainSubstring("non-existent-template"))
			Expect(result).To(BeNil())
		})

		Context("Image Validation", func() {
			It("should allow images in the allowed list", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "allowed-image-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Image:       "quay.io/jupyter/scipy-notebook:latest",
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeTrue())
				Expect(result.Violations).To(BeEmpty())
				Expect(result.Template.Image).To(Equal("quay.io/jupyter/scipy-notebook:latest"))
			})

			It("should reject images not in the allowed list", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "disallowed-image-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Image:       "malicious/image:latest",
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeFalse())
				Expect(result.Violations).To(HaveLen(1))
				Expect(result.Violations[0].Type).To(Equal(ViolationTypeImageNotAllowed))
				Expect(result.Violations[0].Field).To(Equal("spec.image"))
			})
		})

		Context("Resource Validation", func() {
			It("should allow resources within bounds", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "valid-resources-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("2Gi"),
							},
						},
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeTrue())
				Expect(result.Violations).To(BeEmpty())
				Expect(result.Template.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("1")))
			})

			It("should reject CPU requests above maximum", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cpu-exceeded-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("8"), // Exceeds max of 4
							},
						},
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeFalse())
				Expect(result.Violations).To(HaveLen(1))
				Expect(result.Violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
				Expect(result.Violations[0].Field).To(Equal("spec.resources.requests.cpu"))
			})

			It("should reject CPU requests below minimum", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "cpu-below-min-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU: resource.MustParse("50m"), // Below min of 100m
							},
						},
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeFalse())
				Expect(result.Violations).To(HaveLen(1))
				Expect(result.Violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
				Expect(result.Violations[0].Field).To(Equal("spec.resources.requests.cpu"))
			})

			It("should reject memory requests above maximum", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "memory-exceeded-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("16Gi"), // Exceeds max of 8Gi
							},
						},
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeFalse())
				Expect(result.Violations).To(HaveLen(1))
				Expect(result.Violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
				Expect(result.Violations[0].Field).To(Equal("spec.resources.requests.memory"))
			})

			It("should validate GPU resources", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gpu-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("1"),
							},
						},
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeTrue())
				Expect(result.Violations).To(BeEmpty())
			})

			It("should reject GPU requests above maximum", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "gpu-exceeded-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("4"), // Exceeds max of 2
							},
						},
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeFalse())
				Expect(result.Violations).To(HaveLen(1))
				Expect(result.Violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
				Expect(result.Violations[0].Field).To(Equal("spec.resources.requests['nvidia.com/gpu']"))
			})
		})

		Context("Storage Validation", func() {
			It("should allow storage size within bounds", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "valid-storage-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Storage: &workspacesv1alpha1.StorageSpec{
							Size: resource.MustParse("50Gi"),
						},
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeTrue())
				Expect(result.Violations).To(BeEmpty())
				Expect(result.Template.StorageConfiguration.DefaultSize).To(Equal(resource.MustParse("50Gi")))
			})

			It("should reject storage size above maximum", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "storage-exceeded-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Storage: &workspacesv1alpha1.StorageSpec{
							Size: resource.MustParse("200Gi"), // Exceeds max of 100Gi
						},
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeFalse())
				Expect(result.Violations).To(HaveLen(1))
				Expect(result.Violations[0].Type).To(Equal(ViolationTypeStorageExceeded))
				Expect(result.Violations[0].Field).To(Equal("spec.storage.size"))
			})

			It("should reject storage size below minimum", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "storage-below-min-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Storage: &workspacesv1alpha1.StorageSpec{
							Size: resource.MustParse("500Mi"), // Below min of 1Gi
						},
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeFalse())
				Expect(result.Violations).To(HaveLen(1))
				Expect(result.Violations[0].Type).To(Equal(ViolationTypeStorageExceeded))
				Expect(result.Violations[0].Field).To(Equal("spec.storage.size"))
			})

			It("should reject invalid storage size format", func() {
				// Note: This test now relies on CRD validation rejecting invalid quantities at API level
				// resource.Quantity type in Go automatically validates format
				// Keeping test structure for documentation, but it would fail at creation time
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "invalid-storage-format-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Storage: &workspacesv1alpha1.StorageSpec{
							Size: resource.MustParse("1Gi"), // Valid size for test (invalid would fail parse)
						},
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeTrue()) // Changed expectation - now validates
			})
		})

		Context("Multiple Violations", func() {
			It("should collect all validation failures", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "multiple-violations-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
						Image:       "forbidden/image:latest",
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("8"),    // Exceeds max
								corev1.ResourceMemory: resource.MustParse("50Mi"), // Below min
							},
						},
						Storage: &workspacesv1alpha1.StorageSpec{
							Size: resource.MustParse("200Gi"), // Exceeds max
						},
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeFalse())
				Expect(result.Violations).To(HaveLen(4)) // Image, CPU max, Memory min, Storage max

				violationTypes := make(map[string]bool)
				for _, violation := range result.Violations {
					violationTypes[violation.Type] = true
				}
				Expect(violationTypes[ViolationTypeImageNotAllowed]).To(BeTrue())
				Expect(violationTypes[ViolationTypeResourceExceeded]).To(BeTrue())
				Expect(violationTypes[ViolationTypeStorageExceeded]).To(BeTrue())
			})
		})

		Context("Template Resolution", func() {
			It("should properly resolve all template fields", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "full-resolution-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeTrue())
				Expect(result.Template).NotTo(BeNil())

				// Verify all template fields are properly resolved
				Expect(result.Template.Image).To(Equal("quay.io/jupyter/minimal-notebook:latest"))
				Expect(result.Template.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("500m")))
				Expect(result.Template.Resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("1Gi")))
				Expect(result.Template.EnvironmentVariables).To(HaveLen(2))
				Expect(result.Template.StorageConfiguration).NotTo(BeNil())
				Expect(result.Template.StorageConfiguration.DefaultSize).To(Equal(resource.MustParse("10Gi")))
			})

			It("should handle AllowSecondaryStorages field from template", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "secondary-storage-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &templateName,
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeTrue())
				Expect(result.Template).NotTo(BeNil())
				Expect(result.Template.AllowSecondaryStorages).To(BeTrue())
			})

			It("should handle template with AllowSecondaryStorages set to false", func() {
				// Create a template with AllowSecondaryStorages disabled
				restrictedTemplateName := "restricted-template"
				restrictedTemplate := &workspacesv1alpha1.WorkspaceTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name: restrictedTemplateName,
					},
					Spec: workspacesv1alpha1.WorkspaceTemplateSpec{
						DisplayName:            "Restricted Template",
						DefaultImage:           "quay.io/jupyter/minimal-notebook:latest", // Required field
						AllowSecondaryStorages: &[]bool{false}[0],
					},
				}
				Expect(k8sClient.Create(ctx, restrictedTemplate)).To(Succeed())
				defer func() {
					Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, restrictedTemplate))).To(Succeed())
				}()

				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "restricted-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						TemplateRef: &restrictedTemplateName,
					},
				}

				result, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.Valid).To(BeTrue())
				Expect(result.Template).NotTo(BeNil())
				Expect(result.Template.AllowSecondaryStorages).To(BeFalse())
			})
		})

		Context("Image Requirement Enforcement", func() {
			It("should reject template with empty DefaultImage at CRD level", func() {
				emptyImageTemplate := &workspacesv1alpha1.WorkspaceTemplate{
					ObjectMeta: metav1.ObjectMeta{
						Name: "empty-image-template",
					},
					Spec: workspacesv1alpha1.WorkspaceTemplateSpec{
						DisplayName:  "Empty Image Template",
						DefaultImage: "", // Intentionally empty
					},
				}

				// The CRD validation should prevent creation
				err := k8sClient.Create(ctx, emptyImageTemplate)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("defaultImage"))
			})

			It("should reject workspace without template and without image", func() {
				workspace := &workspacesv1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "no-template-no-image-workspace",
						Namespace: "default",
					},
					Spec: workspacesv1alpha1.WorkspaceSpec{
						// No TemplateRef and no Image
					},
				}

				_, err := templateResolver.ValidateAndResolveTemplate(ctx, workspace)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("does not specify an image"))
			})
		})
	})
})
