/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
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
		return WorkspaceTemplateCustomDefaulter{client: fakeClient}
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
			ObjectMeta: metav1.ObjectMeta{Name: "tmpl", Namespace: "team-a"},
		}
	})

	It("stamps access strategy labels and finalizes the AccessStrategy (explicit ref namespace)", func() {
		template.Spec.DefaultAccessStrategy = &workspacev1alpha1.AccessStrategyRef{Name: "web-access", Namespace: "shared-ns"}
		defaulter = newDefaulter(accessStrategyIn("web-access", "shared-ns"))

		Expect(defaulter.Default(ctx, template)).To(Succeed())
		Expect(template.Labels[workspaceutil.LabelAccessStrategyName]).To(Equal("web-access"))
		Expect(template.Labels[workspaceutil.LabelAccessStrategyNamespace]).To(Equal("shared-ns"))
		Expect(finalizerPresent(defaulter.client, "web-access", "shared-ns")).To(BeTrue())
	})

	It("defaults the label namespace to the template namespace when the ref namespace is empty", func() {
		template.Spec.DefaultAccessStrategy = &workspacev1alpha1.AccessStrategyRef{Name: "web-access"}
		defaulter = newDefaulter(accessStrategyIn("web-access", "team-a"))

		Expect(defaulter.Default(ctx, template)).To(Succeed())
		Expect(template.Labels[workspaceutil.LabelAccessStrategyName]).To(Equal("web-access"))
		Expect(template.Labels[workspaceutil.LabelAccessStrategyNamespace]).To(Equal("team-a"))
		Expect(finalizerPresent(defaulter.client, "web-access", "team-a")).To(BeTrue())
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
			workspaceutil.LabelAccessStrategyName:      "web-access",
			workspaceutil.LabelAccessStrategyNamespace: "team-a",
		}
		defaulter = newDefaulter()

		Expect(defaulter.Default(ctx, template)).To(Succeed())
		Expect(template.Labels).NotTo(HaveKey(workspaceutil.LabelAccessStrategyName))
		Expect(template.Labels).NotTo(HaveKey(workspaceutil.LabelAccessStrategyNamespace))
	})

	It("returns an error for a non-template object", func() {
		defaulter = newDefaulter()
		err := defaulter.Default(ctx, &workspacev1alpha1.Workspace{})
		Expect(err).To(HaveOccurred())
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
			ObjectMeta: metav1.ObjectMeta{Name: "tmpl", Namespace: "team-a"},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DefaultAccessStrategy: &workspacev1alpha1.AccessStrategyRef{
					Name:      "some-strategy",
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
			warnings, err := validator.ValidateCreate(ctx, templateWithAS("team-a"))
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("allows a template referencing an access strategy in the shared namespace", func() {
			warnings, err := validator.ValidateCreate(ctx, templateWithAS("shared-ns"))
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("rejects a template referencing an access strategy in another namespace", func() {
			_, err := validator.ValidateCreate(ctx, templateWithAS("team-b"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("team-b"))
			Expect(err.Error()).To(ContainSubstring("template namespace"))
		})

		It("returns an error for a non-template object", func() {
			_, err := validator.ValidateCreate(ctx, &workspacev1alpha1.Workspace{})
			Expect(err).To(HaveOccurred())
		})
	})

	Context("ValidateUpdate", func() {
		It("rejects an update that points the access strategy at another namespace", func() {
			_, err := validator.ValidateUpdate(ctx, templateWithAS("team-a"), templateWithAS("team-b"))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("team-b"))
		})

		It("allows removing the access strategy reference", func() {
			oldTemplate := templateWithAS("team-b")
			newTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl", Namespace: "team-a"},
			}
			warnings, err := validator.ValidateUpdate(ctx, oldTemplate, newTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("returns a warning when constraint fields change", func() {
			oldTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					AllowedImages: []string{"image-a"},
				},
			}
			newTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl", Namespace: "team-a"},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					AllowedImages: []string{"image-a", "image-b"},
				},
			}
			warnings, err := validator.ValidateUpdate(ctx, oldTemplate, newTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(HaveLen(1))
			Expect(warnings[0]).To(ContainSubstring("compliance validation"))
		})

		It("returns no warning when no constraint fields change", func() {
			oldTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl", Namespace: "team-a"},
				Spec:       workspacev1alpha1.WorkspaceTemplateSpec{DisplayName: "before"},
			}
			newTemplate := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{Name: "tmpl", Namespace: "team-a"},
				Spec:       workspacev1alpha1.WorkspaceTemplateSpec{DisplayName: "after"},
			}
			warnings, err := validator.ValidateUpdate(ctx, oldTemplate, newTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("returns an error when the new object is not a template", func() {
			_, err := validator.ValidateUpdate(ctx, templateWithAS("team-a"), &workspacev1alpha1.Workspace{})
			Expect(err).To(HaveOccurred())
		})

		It("returns an error when the old object is not a template", func() {
			_, err := validator.ValidateUpdate(ctx, &workspacev1alpha1.Workspace{}, templateWithAS("team-a"))
			Expect(err).To(HaveOccurred())
		})
	})

	Context("ValidateDelete", func() {
		It("allows deletion and returns no warnings", func() {
			warnings, err := validator.ValidateDelete(ctx, templateWithAS("team-b"))
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
		})

		It("returns an error for a non-template object", func() {
			_, err := validator.ValidateDelete(ctx, &workspacev1alpha1.Workspace{})
			Expect(err).To(HaveOccurred())
		})
	})
})
