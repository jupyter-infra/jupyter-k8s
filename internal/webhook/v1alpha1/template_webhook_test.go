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
