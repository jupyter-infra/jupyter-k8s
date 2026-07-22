/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

const (
	testImageA   = "image-a"
	testImgA     = "img-a"
	testImgB     = "img-b"
	testImgDeflt = "img-default"
)

var _ = Describe("WorkspaceTemplate Defaulter", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		defaulter WorkspaceTemplateCustomDefaulter
		template  *workspacev1alpha1.WorkspaceTemplate
	)

	// newDefaulter builds a defaulter backed by a fake client seeded with the given objects.
	newDefaulter := func(objs ...client.Object) WorkspaceTemplateCustomDefaulter {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
		return WorkspaceTemplateCustomDefaulter{
			client:                  fakeClient,
			accessStrategyValidator: NewAccessStrategyValidator("shared-ns"),
		}
	}

	accessStrategyIn := func(name, namespace string) *workspacev1alpha1.WorkspaceAccessStrategy {
		return &workspacev1alpha1.WorkspaceAccessStrategy{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		}
	}

	finalizerPresent := func(c client.Client, name, namespace string) bool {
		as := &workspacev1alpha1.WorkspaceAccessStrategy{}
		Expect(c.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, as)).To(Succeed())
		return controllerutil.ContainsFinalizer(as, workspaceutil.AccessStrategyTemplateFinalizerName)
	}

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
		}
	})

	It("stamps access strategy labels and finalizes the AccessStrategy (explicit ref namespace)", func() {
		template.Spec.DefaultAccessStrategy = &workspacev1alpha1.AccessStrategyRef{Name: testWebAccessStrategy, Namespace: "shared-ns"}
		defaulter = newDefaulter(accessStrategyIn(testWebAccessStrategy, "shared-ns"))

		Expect(defaulter.Default(ctx, template)).To(Succeed())
		Expect(template.Labels[workspaceutil.LabelAccessStrategyName]).To(Equal(testWebAccessStrategy))
		Expect(template.Labels[workspaceutil.LabelAccessStrategyNamespace]).To(Equal("shared-ns"))
		Expect(finalizerPresent(defaulter.client, testWebAccessStrategy, "shared-ns")).To(BeTrue())
	})

	It("defaults the label namespace to the template namespace when the ref namespace is empty", func() {
		template.Spec.DefaultAccessStrategy = &workspacev1alpha1.AccessStrategyRef{Name: testWebAccessStrategy}
		defaulter = newDefaulter(accessStrategyIn(testWebAccessStrategy, testNamespaceTeamA))

		Expect(defaulter.Default(ctx, template)).To(Succeed())
		Expect(template.Labels[workspaceutil.LabelAccessStrategyName]).To(Equal(testWebAccessStrategy))
		Expect(template.Labels[workspaceutil.LabelAccessStrategyNamespace]).To(Equal(testNamespaceTeamA))
		Expect(finalizerPresent(defaulter.client, testWebAccessStrategy, testNamespaceTeamA)).To(BeTrue())
	})

	It("rejects the template when the referenced AccessStrategy does not exist", func() {
		template.Spec.DefaultAccessStrategy = &workspacev1alpha1.AccessStrategyRef{Name: "not-yet", Namespace: "shared-ns"}
		defaulter = newDefaulter() // no AccessStrategy seeded

		err := defaulter.Default(ctx, template)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("does not set labels when no access strategy is referenced", func() {
		defaulter = newDefaulter()

		Expect(defaulter.Default(ctx, template)).To(Succeed())
		Expect(template.Labels).NotTo(HaveKey(workspaceutil.LabelAccessStrategyName))
		Expect(template.Labels).NotTo(HaveKey(workspaceutil.LabelAccessStrategyNamespace))
	})

	It("clears stale access strategy labels when the reference is removed", func() {
		template.Labels = map[string]string{
			workspaceutil.LabelAccessStrategyName:      testWebAccessStrategy,
			workspaceutil.LabelAccessStrategyNamespace: testNamespaceTeamA,
		}
		defaulter = newDefaulter()

		Expect(defaulter.Default(ctx, template)).To(Succeed())
		Expect(template.Labels).NotTo(HaveKey(workspaceutil.LabelAccessStrategyName))
		Expect(template.Labels).NotTo(HaveKey(workspaceutil.LabelAccessStrategyNamespace))
	})

})

var _ = Describe("WorkspaceTemplate Validator", func() {
	var (
		ctx       context.Context
		validator WorkspaceTemplateCustomValidator
	)

	// templateWithAS builds a template in "team-a" referencing an access strategy in asNamespace.
	templateWithAS := func(asNamespace string) *workspacev1alpha1.WorkspaceTemplate {
		return &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DefaultAccessStrategy: &workspacev1alpha1.AccessStrategyRef{
					Name:      testSomeStrategy,
					Namespace: asNamespace,
				},
			},
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		validator = WorkspaceTemplateCustomValidator{
			accessStrategyValidator: NewAccessStrategyValidator("shared-ns"),
		}
	})

	Context("ValidateCreate", func() {
		It("allows a template referencing an access strategy in its own namespace", func() {
			warnings, err := validator.ValidateCreate(ctx, templateWithAS(testNamespaceTeamA))
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("allows a template referencing an access strategy in the shared namespace", func() {
			warnings, err := validator.ValidateCreate(ctx, templateWithAS("shared-ns"))
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("rejects a template referencing an access strategy in another namespace", func() {
			_, err := validator.ValidateCreate(ctx, templateWithAS(testNamespaceTeamB))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamB))
			Expect(err.Error()).To(ContainSubstring("template namespace"))
		})

	})

	Context("ValidateUpdate", func() {
		It("rejects an update that points the access strategy at another namespace", func() {
			_, err := validator.ValidateUpdate(ctx, templateWithAS(testNamespaceTeamA), templateWithAS(testNamespaceTeamB))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(testNamespaceTeamB))
		})

		It("allows removing the access strategy reference", func() {
			oldTemplate := templateWithAS(testNamespaceTeamB)
			newTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
			}
			warnings, err := validator.ValidateUpdate(ctx, oldTemplate, newTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("returns a warning when constraint fields change", func() {
			oldTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage:  testImageA,
					AllowedImages: []string{testImageA},
				},
			}
			newTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage:  testImageA,
					AllowedImages: []string{testImageA, "image-b"},
				},
			}
			warnings, err := validator.ValidateUpdate(ctx, oldTemplate, newTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(ContainSubstring("compliance validation"))
		})

		It("returns no warning when no constraint fields change", func() {
			oldTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
				Spec:       workspacev1alpha1.WorkspaceTemplateSpec{DisplayName: "before"},
			}
			newTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
				Spec:       workspacev1alpha1.WorkspaceTemplateSpec{DisplayName: "after"},
			}
			warnings, err := validator.ValidateUpdate(ctx, oldTemplate, newTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

	})

	Context("Idle shutdown policy consistency", func() {
		boolPtr := func(b bool) *bool { return &b }

		lockingPolicy := func() *workspacev1alpha1.IdleShutdownOverridePolicy {
			return &workspacev1alpha1.IdleShutdownOverridePolicy{Allow: boolPtr(false)}
		}
		idleDefault := func() *workspacev1alpha1.IdleShutdownSpec {
			return &workspacev1alpha1.IdleShutdownSpec{Enabled: true, IdleTimeoutInMinutes: 30}
		}
		templateWithIdle := func(
			policy *workspacev1alpha1.IdleShutdownOverridePolicy,
			def *workspacev1alpha1.IdleShutdownSpec,
		) *workspacev1alpha1.WorkspaceTemplate {
			return &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					IdleShutdownOverrides: policy,
					DefaultIdleShutdown:   def,
				},
			}
		}

		It("rejects create when allow is false and defaultIdleShutdown is nil", func() {
			_, err := validator.ValidateCreate(ctx, templateWithIdle(lockingPolicy(), nil))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("defaultIdleShutdown is not set"))
		})

		It("rejects update when allow is false and defaultIdleShutdown is nil", func() {
			oldTemplate := templateWithIdle(lockingPolicy(), idleDefault())
			newTemplate := templateWithIdle(lockingPolicy(), nil)
			_, err := validator.ValidateUpdate(ctx, oldTemplate, newTemplate)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("defaultIdleShutdown is not set"))
		})

		It("allows allow: false when a defaultIdleShutdown is set", func() {
			_, err := validator.ValidateCreate(ctx, templateWithIdle(lockingPolicy(), idleDefault()))
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows allow: true without a defaultIdleShutdown", func() {
			_, err := validator.ValidateCreate(ctx, templateWithIdle(
				&workspacev1alpha1.IdleShutdownOverridePolicy{Allow: boolPtr(true)}, nil))
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows a nil policy without a defaultIdleShutdown", func() {
			_, err := validator.ValidateCreate(ctx, templateWithIdle(nil, nil))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Idle shutdown timeout bounds consistency", func() {
		intPtr := func(i int) *int { return &i }

		templateWithBounds := func(min, max *int) *workspacev1alpha1.WorkspaceTemplate {
			return &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					IdleShutdownOverrides: &workspacev1alpha1.IdleShutdownOverridePolicy{
						MinIdleTimeoutInMinutes: min,
						MaxIdleTimeoutInMinutes: max,
					},
				},
			}
		}

		It("rejects create when min timeout exceeds max timeout", func() {
			_, err := validator.ValidateCreate(ctx, templateWithBounds(intPtr(60), intPtr(30)))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("minIdleTimeoutInMinutes"))
		})

		It("allows min equal to max", func() {
			_, err := validator.ValidateCreate(ctx, templateWithBounds(intPtr(30), intPtr(30)))
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows only one bound set", func() {
			_, err := validator.ValidateCreate(ctx, templateWithBounds(intPtr(30), nil))
			Expect(err).NotTo(HaveOccurred())
			_, err = validator.ValidateCreate(ctx, templateWithBounds(nil, intPtr(30)))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Image policy consistency", func() {
		boolPtr := func(b bool) *bool { return &b }

		templateWithImages := func(defaultImage string, allowed []string, allowCustom *bool) *workspacev1alpha1.WorkspaceTemplate {
			return &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DefaultImage:      defaultImage,
					AllowedImages:     allowed,
					AllowCustomImages: allowCustom,
				},
			}
		}

		It("rejects a defaultImage absent from a non-empty allowedImages", func() {
			_, err := validator.ValidateCreate(ctx, templateWithImages(testImgDeflt, []string{testImgA, testImgB}, nil))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("defaultImage"))
			Expect(err.Error()).To(ContainSubstring("allowedImages"))
		})

		It("allows a defaultImage present in allowedImages", func() {
			_, err := validator.ValidateCreate(ctx, templateWithImages(testImgA, []string{testImgA, testImgB}, nil))
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows an empty allowedImages (falls back to defaultImage)", func() {
			_, err := validator.ValidateCreate(ctx, templateWithImages(testImgDeflt, nil, nil))
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows a mismatch when custom images are permitted", func() {
			_, err := validator.ValidateCreate(ctx, templateWithImages(testImgDeflt, []string{testImgA}, boolPtr(true)))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Storage bounds consistency", func() {
		qtyPtr := func(s string) *resource.Quantity {
			q := resource.MustParse(s)
			return &q
		}

		templateWithStorage := func(min, max *resource.Quantity) *workspacev1alpha1.WorkspaceTemplate {
			return &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					PrimaryStorage: &workspacev1alpha1.StorageConfig{MinSize: min, MaxSize: max},
				},
			}
		}

		It("rejects minSize greater than maxSize", func() {
			_, err := validator.ValidateCreate(ctx, templateWithStorage(qtyPtr("20Gi"), qtyPtr("10Gi")))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("minSize"))
		})

		It("allows minSize equal to maxSize", func() {
			_, err := validator.ValidateCreate(ctx, templateWithStorage(qtyPtr("10Gi"), qtyPtr("10Gi")))
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows only one bound set", func() {
			_, err := validator.ValidateCreate(ctx, templateWithStorage(qtyPtr("10Gi"), nil))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Resource bounds consistency", func() {
		templateWithResourceBounds := func(min, max string) *workspacev1alpha1.WorkspaceTemplate {
			return &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: testTemplateNameTmpl, Namespace: testNamespaceTeamA},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					ResourceBounds: &workspacev1alpha1.ResourceBounds{
						Resources: map[corev1.ResourceName]workspacev1alpha1.ResourceRange{
							corev1.ResourceCPU: {Min: resource.MustParse(min), Max: resource.MustParse(max)},
						},
					},
				},
			}
		}

		It("rejects a resource min greater than max", func() {
			_, err := validator.ValidateCreate(ctx, templateWithResourceBounds("4", "2"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("resourceBounds"))
		})

		It("allows a resource min equal to max", func() {
			_, err := validator.ValidateCreate(ctx, templateWithResourceBounds("2", "2"))
			Expect(err).NotTo(HaveOccurred())
		})

		It("allows a valid resource range", func() {
			_, err := validator.ValidateCreate(ctx, templateWithResourceBounds("1", "4"))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("ValidateDelete", func() {
		It("allows deletion and returns no warnings", func() {
			warnings, err := validator.ValidateDelete(ctx, templateWithAS(testNamespaceTeamB))
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

	})
})
