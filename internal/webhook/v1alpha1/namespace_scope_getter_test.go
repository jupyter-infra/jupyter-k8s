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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	webhookconst "github.com/jupyter-infra/jupyter-k8s/internal/webhook"
)

var _ = Describe("GetTemplateScopeStrategy", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should return Cluster when namespace has no template-scope label", func() {
		// "default" namespace exists in envtest and has no template-scope label
		scope, err := GetTemplateScopeStrategy(ctx, k8sClient, "default")
		Expect(err).NotTo(HaveOccurred())
		Expect(scope).To(Equal(webhookconst.TemplateScopeCluster))
	})

	It("should return Namespaced when namespace has Namespaced label", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scope-test-namespaced",
				Labels: map[string]string{
					webhookconst.LabelTemplateScope: string(webhookconst.TemplateScopeNamespaced),
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		defer func() { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) }()

		scope, err := GetTemplateScopeStrategy(ctx, k8sClient, ns.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope).To(Equal(webhookconst.TemplateScopeNamespaced))
	})

	It("should return Cluster when namespace has Cluster label", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scope-test-cluster",
				Labels: map[string]string{
					webhookconst.LabelTemplateScope: string(webhookconst.TemplateScopeCluster),
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		defer func() { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) }()

		scope, err := GetTemplateScopeStrategy(ctx, k8sClient, ns.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope).To(Equal(webhookconst.TemplateScopeCluster))
	})

	It("should return Cluster when namespace has empty label value", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scope-test-empty",
				Labels: map[string]string{
					webhookconst.LabelTemplateScope: "",
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		defer func() { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) }()

		scope, err := GetTemplateScopeStrategy(ctx, k8sClient, ns.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope).To(Equal(webhookconst.TemplateScopeCluster))
	})

	It("should return error for unrecognized label value", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scope-test-invalid",
				Labels: map[string]string{
					webhookconst.LabelTemplateScope: "InvalidValue",
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		defer func() { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) }()

		_, err := GetTemplateScopeStrategy(ctx, k8sClient, ns.Name)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unrecognized template-scope value"))
	})

	It("should return error when namespace does not exist", func() {
		_, err := GetTemplateScopeStrategy(ctx, k8sClient, "nonexistent-namespace")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get namespace"))
	})
})
