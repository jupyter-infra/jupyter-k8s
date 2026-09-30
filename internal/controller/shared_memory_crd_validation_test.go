/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("Shared memory CRD validation", func() {
	It("should reject a volume named workspace-shm", func() {
		workspace := workspaceWithVolume("reserved-shm-name", workspacev1alpha1.VolumeSpec{
			Name:                      volumeNameWorkspaceSharedMemory,
			MountPath:                 "/scratch",
			PersistentVolumeClaimName: "scratch-pvc",
		})

		err := k8sClient.Create(ctx, workspace)

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("volume name 'workspace-shm' is reserved"))
	})

	It("should store sharedMemory on templates and workspaces and default enabled to true", func() {
		sizeLimit := resource.MustParse("512Mi")
		template := &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: "shm-crd-template", Namespace: testNamespace},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DisplayName:  "Shared memory",
				DefaultImage: "jupyter:latest",
				SharedMemory: &workspacev1alpha1.SharedMemorySpec{SizeLimit: &sizeLimit},
			},
		}
		Expect(k8sClient.Create(ctx, template)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, template) })

		workspace := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "shm-crd-workspace", Namespace: testNamespace},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName:  testWorkspaceDisplayName,
				SharedMemory: &workspacev1alpha1.SharedMemorySpec{SizeLimit: &sizeLimit},
			},
		}
		Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
		DeferCleanup(func() { _ = k8sClient.Delete(ctx, workspace) })

		stored := &workspacev1alpha1.Workspace{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{Name: workspace.Name, Namespace: testNamespace}, stored)).To(Succeed())
		Expect(stored.Spec.SharedMemory).NotTo(BeNil())
		Expect(stored.Spec.SharedMemory.Enabled).To(HaveValue(BeTrue()))
		Expect(stored.Spec.SharedMemory.SizeLimit.Cmp(sizeLimit)).To(BeZero())
	})
})
