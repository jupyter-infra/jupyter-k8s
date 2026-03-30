/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("Resource Validator", func() {
	var template *workspacev1alpha1.WorkspaceTemplate

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-template",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				ResourceBounds: &workspacev1alpha1.ResourceBounds{
					Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
						corev1.ResourceCPU: {
							Min: resource.MustParse("100m"),
							Max: resource.MustParse("2"),
						},
						corev1.ResourceMemory: {
							Min: resource.MustParse("128Mi"),
							Max: resource.MustParse("4Gi"),
						},
						corev1.ResourceName("nvidia.com/gpu"): {
							Min: resource.MustParse("0"),
							Max: resource.MustParse("4"),
						},
					},
				},
			},
		}
	})

	Context("request cpu and memory bounds validation", func() {
		It("should allow all resources (cpu, memory, gpu) within bounds", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:                    resource.MustParse("500m"),
					corev1.ResourceMemory:                 resource.MustParse("1Gi"),
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(BeEmpty())
		})

		It("should reject CPU below minimum", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("50m"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
			Expect(violations[0].Field).To(Equal("spec.resources.requests.cpu"))
			Expect(violations[0].Message).To(ContainSubstring("below minimum"))
			Expect(violations[0].Message).To(ContainSubstring("test-template"))
		})

		It("should reject CPU above maximum", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("4"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
			Expect(violations[0].Field).To(Equal("spec.resources.requests.cpu"))
			Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
			Expect(violations[0].Message).To(ContainSubstring("test-template"))
		})

		It("should reject memory below minimum", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
			Expect(violations[0].Message).To(ContainSubstring("below minimum"))
		})

		It("should reject memory above maximum", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("8Gi"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
			Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
		})

		It("should allow resources when no bounds defined", func() {
			template.Spec.ResourceBounds = nil
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("10"),
					corev1.ResourceMemory: resource.MustParse("100Gi"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(BeEmpty())
		})
	})

	Context("limit cpu and memory bounds validation", func() {
		It("should allow limits within bounds", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("1"),
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(BeEmpty())
		})

		It("should reject CPU limit above maximum", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("16"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
			Expect(violations[0].Field).To(Equal("spec.resources.limits.cpu"))
			Expect(violations[0].Message).To(ContainSubstring("limit"))
			Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
			Expect(violations[0].Message).To(ContainSubstring("test-template"))
		})

		It("should reject memory limit below minimum", func() {
			resources := corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("64Mi"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
			Expect(violations[0].Field).To(Equal("spec.resources.limits.memory"))
			Expect(violations[0].Message).To(ContainSubstring("limit"))
			Expect(violations[0].Message).To(ContainSubstring("below minimum"))
		})

		It("should reject limits above bounds even when requests are within bounds", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("8"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Field).To(Equal("spec.resources.limits.cpu"))
			Expect(violations[0].Message).To(ContainSubstring("limit"))
		})

		It("should reject limits without requests when above bounds", func() {
			resources := corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("10"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Field).To(Equal("spec.resources.limits.cpu"))
		})

		It("should reject GPU limit above maximum", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("8"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Field).To(Equal("spec.resources.limits.nvidia.com/gpu"))
			Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
		})

		It("should report violations for both requests and limits independently", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("10"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("16"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(2))

			fields := []string{violations[0].Field, violations[1].Field}
			Expect(fields).To(ContainElement("spec.resources.requests.cpu"))
			Expect(fields).To(ContainElement("spec.resources.limits.cpu"))
		})

		It("should pass when bounds defined but neither requests nor limits set", func() {
			resources := corev1.ResourceRequirements{}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(BeEmpty())
		})
	})

	Context("limits >= requests sanity check", func() {
		It("should reject CPU limit less than request", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("1"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU: resource.MustParse("500m"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Message).To(ContainSubstring("CPU limit must be greater than or equal to CPU request"))
		})

		It("should reject memory limit less than request", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("2Gi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1Gi"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Message).To(ContainSubstring("Memory limit must be greater than or equal to memory request"))
		})
	})

	Context("GPU bounds validation", func() {
		It("should allow GPU within bounds", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("2"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(BeEmpty())
		})

		It("should reject GPU below minimum", func() {
			template.Spec.ResourceBounds.Resources[corev1.ResourceName("nvidia.com/gpu")] = workspacev1alpha1.ResourceRange{
				Min: resource.MustParse("1"),
				Max: resource.MustParse("4"),
			}
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("0"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
			Expect(violations[0].Message).To(ContainSubstring("below minimum"))
			Expect(violations[0].Message).To(ContainSubstring("test-template"))
		})

		It("should reject GPU above maximum", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("8"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeResourceExceeded))
			Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
			Expect(violations[0].Message).To(ContainSubstring("test-template"))
		})

		It("should allow GPU when no GPU bounds specified", func() {
			delete(template.Spec.ResourceBounds.Resources, corev1.ResourceName("nvidia.com/gpu"))
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("100"),
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(BeEmpty())
		})

		It("should validate GPU bounds independently of CPU/Memory", func() {
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:                    resource.MustParse("500m"), // Valid
					corev1.ResourceMemory:                 resource.MustParse("1Gi"),  // Valid
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("10"),   // Invalid - exceeds max
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Field).To(Equal("spec.resources.requests.nvidia.com/gpu"))
			Expect(violations[0].Message).To(ContainSubstring("exceeds maximum"))
		})
	})

	Context("edge cases", func() {
		It("should allow unbounded resources (not in template bounds)", func() {
			template.Spec.ResourceBounds = &workspacev1alpha1.ResourceBounds{
				Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
					corev1.ResourceCPU: {
						Min: resource.MustParse("100m"),
						Max: resource.MustParse("2"),
					},
				},
			}
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:                    resource.MustParse("1"),     // Bounded - valid
					corev1.ResourceMemory:                 resource.MustParse("100Gi"), // Unbounded - allowed
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("100"),   // Unbounded - allowed
					corev1.ResourceName("custom.io/tpu"):  resource.MustParse("1000"),  // Unbounded - allowed
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(BeEmpty())
		})

		It("should report multiple violations for multiple resources", func() {
			template.Spec.ResourceBounds = &workspacev1alpha1.ResourceBounds{
				Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
					corev1.ResourceCPU: {
						Min: resource.MustParse("100m"),
						Max: resource.MustParse("2"),
					},
					corev1.ResourceMemory: {
						Min: resource.MustParse("128Mi"),
						Max: resource.MustParse("4Gi"),
					},
					corev1.ResourceName("nvidia.com/gpu"): {
						Min: resource.MustParse("0"),
						Max: resource.MustParse("2"),
					},
				},
			}
			resources := corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:                    resource.MustParse("10"),   // Exceeds max
					corev1.ResourceMemory:                 resource.MustParse("50Mi"), // Below min
					corev1.ResourceName("nvidia.com/gpu"): resource.MustParse("5"),    // Exceeds max
				},
			}
			violations := validateResourceBounds(resources, template)
			Expect(violations).To(HaveLen(3))
		})
	})

	Context("resourcesEqual", func() {
		It("should return true for nil resources", func() {
			Expect(resourcesEqual(nil, nil)).To(BeTrue())
		})

		It("should return false when one is nil", func() {
			resources := &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
			}
			Expect(resourcesEqual(nil, resources)).To(BeFalse())
			Expect(resourcesEqual(resources, nil)).To(BeFalse())
		})

		It("should return true for equal resources", func() {
			resources1 := &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
				Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
			}
			resources2 := &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
				Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
			}
			Expect(resourcesEqual(resources1, resources2)).To(BeTrue())
		})

		It("should return false for different requests", func() {
			resources1 := &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
			}
			resources2 := &corev1.ResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
			}
			Expect(resourcesEqual(resources1, resources2)).To(BeFalse())
		})

		It("should return false for different limits", func() {
			resources1 := &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("1")},
			}
			resources2 := &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
			}
			Expect(resourcesEqual(resources1, resources2)).To(BeFalse())
		})
	})
})
