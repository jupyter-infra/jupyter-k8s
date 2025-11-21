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
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	webhookconst "github.com/jupyter-infra/jupyter-k8s/internal/webhook"
)

var _ = Describe("ServiceAccountDefaulter", func() {
	var (
		defaulter *ServiceAccountDefaulter
		workspace *workspacev1alpha1.Workspace
		ctx       context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName: "Test Workspace",
			},
		}
	})

	Context("ApplyServiceAccountDefaults", func() {
		It("should set default service account when not specified", func() {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			defaulter = NewServiceAccountDefaulter(fakeClient)

			err := defaulter.ApplyServiceAccountDefaults(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Spec.ServiceAccountName).To(Equal("default"))
		})

		It("should not override existing service account name", func() {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)

			workspace.Spec.ServiceAccountName = "existing-sa"

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			defaulter = NewServiceAccountDefaulter(fakeClient)

			err := defaulter.ApplyServiceAccountDefaults(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Spec.ServiceAccountName).To(Equal("existing-sa"))
		})

		It("should set custom default service account when one exists", func() {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)

			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-default-sa",
					Namespace: "default",
					Labels: map[string]string{
						webhookconst.DefaultServiceAccountLabel: "true",
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sa).
				Build()

			defaulter = NewServiceAccountDefaulter(fakeClient)

			err := defaulter.ApplyServiceAccountDefaults(ctx, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(workspace.Spec.ServiceAccountName).To(Equal("custom-default-sa"))
		})
	})

	Context("GetDefaultServiceAccount", func() {
		It("should return 'default' when no service accounts have the label", func() {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			result, err := GetDefaultServiceAccount(ctx, fakeClient, "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("default"))
		})

		It("should return the service account name when exactly one has the label", func() {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)

			sa := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-sa",
					Namespace: "default",
					Labels: map[string]string{
						webhookconst.DefaultServiceAccountLabel: "true",
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sa).
				Build()

			result, err := GetDefaultServiceAccount(ctx, fakeClient, "default")
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("custom-sa"))
		})

		It("should return error when multiple service accounts have the label", func() {
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)

			sa1 := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sa1",
					Namespace: "default",
					Labels: map[string]string{
						webhookconst.DefaultServiceAccountLabel: "true",
					},
				},
			}

			sa2 := &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sa2",
					Namespace: "default",
					Labels: map[string]string{
						webhookconst.DefaultServiceAccountLabel: "true",
					},
				},
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(sa1, sa2).
				Build()

			result, err := GetDefaultServiceAccount(ctx, fakeClient, "default")
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("multiple service accounts found"))
			Expect(result).To(BeEmpty())
		})
	})
})
