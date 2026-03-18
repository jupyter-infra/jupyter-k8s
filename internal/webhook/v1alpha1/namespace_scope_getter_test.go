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

var _ = Describe("GetTemplateScopeStrategyFromWorkspaceNamespaceLabel", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should return Cluster when namespace has no template-namespace-scope label", func() {
		// "default" namespace exists in envtest and has no template-namespace-scope label
		scope, err := GetTemplateScopeStrategyFromWorkspaceNamespaceLabel(ctx, k8sClient, "default")
		Expect(err).NotTo(HaveOccurred())
		Expect(scope).To(Equal(webhookconst.TemplateScopeCluster))
	})

	It("should return Namespaced when namespace has template-namespace-scope label set to Namespaced", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scope-test-namespaced",
				Labels: map[string]string{
					webhookconst.TemplateScopeNamespaceLabel: string(webhookconst.TemplateScopeNamespaced),
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		defer func() { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) }()

		scope, err := GetTemplateScopeStrategyFromWorkspaceNamespaceLabel(ctx, k8sClient, ns.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope).To(Equal(webhookconst.TemplateScopeNamespaced))
	})

	It("should return Cluster when namespace has template-namespace-scope label set to Cluster", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scope-test-cluster",
				Labels: map[string]string{
					webhookconst.TemplateScopeNamespaceLabel: string(webhookconst.TemplateScopeCluster),
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		defer func() { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) }()

		scope, err := GetTemplateScopeStrategyFromWorkspaceNamespaceLabel(ctx, k8sClient, ns.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope).To(Equal(webhookconst.TemplateScopeCluster))
	})

	It("should return Cluster when namespace has empty template-namespace-scope label value", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scope-test-empty",
				Labels: map[string]string{
					webhookconst.TemplateScopeNamespaceLabel: "",
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		defer func() { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) }()

		scope, err := GetTemplateScopeStrategyFromWorkspaceNamespaceLabel(ctx, k8sClient, ns.Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(scope).To(Equal(webhookconst.TemplateScopeCluster))
	})

	It("should return error for unrecognized template-namespace-scope label value", func() {
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "scope-test-invalid",
				Labels: map[string]string{
					webhookconst.TemplateScopeNamespaceLabel: "InvalidValue",
				},
			},
		}
		Expect(k8sClient.Create(ctx, ns)).To(Succeed())
		defer func() { Expect(k8sClient.Delete(ctx, ns)).To(Succeed()) }()

		_, err := GetTemplateScopeStrategyFromWorkspaceNamespaceLabel(ctx, k8sClient, ns.Name)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("unrecognized template-namespace-scope value"))
	})

	It("should return error when namespace does not exist", func() {
		_, err := GetTemplateScopeStrategyFromWorkspaceNamespaceLabel(ctx, k8sClient, "nonexistent-namespace")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to get namespace"))
	})
})
