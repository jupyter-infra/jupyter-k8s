/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("ServiceBuilder", func() {
	var (
		ctx            context.Context
		serviceBuilder *ServiceBuilder
		scheme         *runtime.Scheme
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())

		serviceBuilder = NewServiceBuilder(scheme)
	})

	Context("Service Updates", func() {
		var (
			workspace       *workspacev1alpha1.Workspace
			existingService *corev1.Service
		)

		BeforeEach(func() {
			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					Image: "jupyter/base-notebook:latest",
				},
			}

			// Create existing service
			var err error
			existingService, err = serviceBuilder.BuildService(workspace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not detect update when nothing changed", func() {
			needsUpdate, err := serviceBuilder.NeedsUpdate(ctx, existingService, workspace)
			Expect(err).NotTo(HaveOccurred())
			Expect(needsUpdate).To(BeFalse())
		})

		It("should update service spec correctly", func() {
			err := serviceBuilder.UpdateServiceSpec(ctx, existingService, workspace)
			Expect(err).NotTo(HaveOccurred())

			// Verify the service spec is still correct
			Expect(existingService.Spec.Type).To(Equal(corev1.ServiceTypeClusterIP))
			Expect(existingService.Spec.Ports).To(HaveLen(1))
			Expect(existingService.Spec.Ports[0].Port).To(Equal(int32(JupyterPort)))
		})
	})
})
