/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
)

var _ = Describe("StorageValidator", func() {
	var (
		ctx    context.Context
		scheme *runtime.Scheme
	)

	const (
		wsName      = "storage-ws"
		wsNs        = "default"
		pvcSize     = "2Gi"
		smaller     = "1Gi"
		larger      = "5Gi"
		sameAsPVC   = "2Gi"
		pvcResource = "persistentvolumeclaims"
	)

	// makeWorkspace builds a workspace requesting the given storage size ("" means no storage).
	makeWorkspace := func(size string) *workspacev1alpha1.Workspace {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: wsName, Namespace: wsNs},
		}
		if size != "" {
			ws.Spec.Storage = &workspacev1alpha1.StorageSpec{Size: resource.MustParse(size)}
		}
		return ws
	}

	// existingPVC builds the workspace's PVC with the given requested storage size.
	existingPVC := func(size string) *corev1.PersistentVolumeClaim {
		return &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      controller.GeneratePVCName(wsName),
				Namespace: wsNs,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse(size)},
				},
			},
		}
	}

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
	})

	Context("when no PVC exists", func() {
		It("allows any requested size (fake client returns NotFound)", func() {
			fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
			sv := NewStorageValidator(fakeClient)
			Expect(sv.ValidateStorageSizeNotShrinking(ctx, makeWorkspace(smaller))).To(Succeed())
		})
	})

	Context("when storage is not requested", func() {
		It("skips the lookup entirely and allows", func() {
			// A client whose Get always fails proves the lookup is never reached.
			failingClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(context.Context, client.WithWatch, client.ObjectKey, client.Object, ...client.GetOption) error {
						return errors.New("Get should not be called when storage is unset")
					},
				}).
				Build()
			sv := NewStorageValidator(failingClient)

			Expect(sv.ValidateStorageSizeNotShrinking(ctx, makeWorkspace(""))).To(Succeed())

			zero := makeWorkspace("")
			zero.Spec.Storage = &workspacev1alpha1.StorageSpec{} // size is the zero quantity
			Expect(sv.ValidateStorageSizeNotShrinking(ctx, zero)).To(Succeed())
		})
	})

	Context("when the PVC exists", func() {
		It("rejects shrinking below the provisioned size", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(existingPVC(pvcSize)).
				Build()
			sv := NewStorageValidator(fakeClient)

			err := sv.ValidateStorageSizeNotShrinking(ctx, makeWorkspace(smaller))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("shrinking"))
		})

		It("allows keeping the same size", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(existingPVC(pvcSize)).
				Build()
			sv := NewStorageValidator(fakeClient)
			Expect(sv.ValidateStorageSizeNotShrinking(ctx, makeWorkspace(sameAsPVC))).To(Succeed())
		})

		It("allows growing beyond the provisioned size", func() {
			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(existingPVC(pvcSize)).
				Build()
			sv := NewStorageValidator(fakeClient)
			Expect(sv.ValidateStorageSizeNotShrinking(ctx, makeWorkspace(larger))).To(Succeed())
		})
	})

	Context("when retrieving the PVC fails", func() {
		// getErrorClient returns a fake client whose PVC Get always fails with the given error.
		getErrorClient := func(getErr error) client.Client {
			return fake.NewClientBuilder().
				WithScheme(scheme).
				WithInterceptorFuncs(interceptor.Funcs{
					Get: func(_ context.Context, _ client.WithWatch, _ client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
						if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
							return getErr
						}
						return errors.New("unexpected Get for non-PVC object")
					},
				}).
				Build()
		}

		// The webhook fails closed on lookup errors it cannot classify as NotFound: a blind
		// webhook must not silently wave a shrink through, matching failurePolicy=fail.

		It("fails closed on a Forbidden (RBAC) error", func() {
			forbidden := apierrors.NewForbidden(
				schema.GroupResource{Resource: pvcResource}, controller.GeneratePVCName(wsName),
				errors.New("not allowed"))
			sv := NewStorageValidator(getErrorClient(forbidden))

			err := sv.ValidateStorageSizeNotShrinking(ctx, makeWorkspace(smaller))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to validate storage size"))
		})

		It("fails closed on a ServerTimeout (API outage) error", func() {
			timeout := apierrors.NewServerTimeout(
				schema.GroupResource{Resource: pvcResource}, "get", 1)
			sv := NewStorageValidator(getErrorClient(timeout))

			err := sv.ValidateStorageSizeNotShrinking(ctx, makeWorkspace(smaller))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to validate storage size"))
		})

		It("fails closed on an arbitrary non-status error", func() {
			sv := NewStorageValidator(getErrorClient(errors.New("connection refused")))
			err := sv.ValidateStorageSizeNotShrinking(ctx, makeWorkspace(smaller))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unable to validate storage size"))
		})

		It("still allows the update when the error is NotFound (no PVC yet)", func() {
			notFound := apierrors.NewNotFound(
				schema.GroupResource{Resource: pvcResource}, controller.GeneratePVCName(wsName))
			sv := NewStorageValidator(getErrorClient(notFound))
			Expect(sv.ValidateStorageSizeNotShrinking(ctx, makeWorkspace(smaller))).To(Succeed())
		})
	})
})
