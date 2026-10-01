/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("SharedMemoryValidator", func() {
	newTemplate := func(sharedMemory *workspacev1alpha1.SharedMemorySpec) *workspacev1alpha1.WorkspaceTemplate {
		return &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: testTemplateName},
			Spec:       workspacev1alpha1.WorkspaceTemplateSpec{SharedMemory: sharedMemory},
		}
	}

	newWorkspace := func(sharedMemory *workspacev1alpha1.SharedMemorySpec) *workspacev1alpha1.Workspace {
		return &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName},
			Spec:       workspacev1alpha1.WorkspaceSpec{SharedMemory: sharedMemory},
		}
	}

	Context("validateSharedMemory", func() {
		It("accepts any workspace setting when the template sets none", func() {
			workspace := newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(true), SizeLimit: qtyPtr("64Gi")})
			Expect(validateSharedMemory(workspace, newTemplate(nil))).To(BeEmpty())
		})

		It("accepts a workspace that sets nothing", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)})
			Expect(validateSharedMemory(newWorkspace(nil), template)).To(BeEmpty())
		})

		It("rejects enabling the volume when the template disables it", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)})
			for _, workspace := range []*workspacev1alpha1.Workspace{
				newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(true)}),
				newWorkspace(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")}),
			} {
				violations := validateSharedMemory(workspace, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryNotAllowed))
				Expect(violations[0].Field).To(Equal("spec.sharedMemory.enabled"))
			}
		})

		It("accepts a disabled workspace under a disabling template", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)})
			workspace := newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false), SizeLimit: qtyPtr("64Gi")})
			Expect(validateSharedMemory(workspace, template)).To(BeEmpty())
		})

		It("rejects a sizeLimit above the template sizeLimit", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")})
			violations := validateSharedMemory(newWorkspace(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("2Gi")}), template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryExceeded))
			Expect(violations[0].Field).To(Equal("spec.sharedMemory.sizeLimit"))
			Expect(violations[0].Actual).To(Equal("2Gi"))
		})

		It("rejects an unset sizeLimit under a template sizeLimit", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")})
			violations := validateSharedMemory(newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(true)}), template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryExceeded))
			Expect(violations[0].Actual).To(Equal("unset"))
		})

		It("accepts a sizeLimit at or below the template sizeLimit", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")})
			for _, size := range []string{"1Gi", "512Mi", "1G"} {
				Expect(validateSharedMemory(newWorkspace(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr(size)}), template)).To(BeEmpty())
			}
		})

		It("ignores the template sizeLimit for a workspace that disables the volume", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")})
			workspace := newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)})
			Expect(validateSharedMemory(workspace, template)).To(BeEmpty())
		})
	})

	Context("validateSharedMemoryVolumes", func() {
		shmVolume := func(name, mountPath string, source workspacev1alpha1.VolumeSpec) *workspacev1alpha1.Workspace {
			source.Name = name
			source.MountPath = mountPath
			return &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName},
				Spec:       workspacev1alpha1.WorkspaceSpec{Volumes: []workspacev1alpha1.VolumeSpec{source}},
			}
		}
		memoryEmptyDir := func(size string) workspacev1alpha1.VolumeSpec {
			return workspacev1alpha1.VolumeSpec{EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory, SizeLimit: qtyPtr(size)}}
		}

		It("ignores volumes when the template sets nothing", func() {
			Expect(validateSharedMemoryVolumes(shmVolume("shm", "/dev/shm", memoryEmptyDir("64Gi")), newTemplate(nil))).To(BeEmpty())
		})

		It("ignores volumes mounted elsewhere", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)})
			Expect(validateSharedMemoryVolumes(shmVolume("data", "/data", memoryEmptyDir("64Gi")), template)).To(BeEmpty())
		})

		It("rejects a volume at /dev/shm when the template disables shared memory", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)})
			for _, mountPath := range []string{"/dev/shm", "/dev/shm/"} {
				violations := validateSharedMemoryVolumes(shmVolume("shm", mountPath, memoryEmptyDir("64Mi")), template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryNotAllowed))
				Expect(violations[0].Field).To(Equal("spec.volumes[shm].mountPath"))
			}
		})

		It("bounds a memory-backed volume at /dev/shm by the template sizeLimit", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")})
			Expect(validateSharedMemoryVolumes(shmVolume("shm", "/dev/shm", memoryEmptyDir("1Gi")), template)).To(BeEmpty())
			Expect(validateSharedMemoryVolumes(shmVolume("shm", "/dev/shm", memoryEmptyDir("512Mi")), template)).To(BeEmpty())

			violations := validateSharedMemoryVolumes(shmVolume("shm", "/dev/shm", memoryEmptyDir("2Gi")), template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryExceeded))
			Expect(violations[0].Field).To(Equal("spec.volumes[shm].emptyDir.sizeLimit"))
			Expect(violations[0].Actual).To(Equal("2Gi"))
		})

		It("rejects an unbounded volume at /dev/shm under a template sizeLimit", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")})
			unbounded := []workspacev1alpha1.VolumeSpec{
				{EmptyDir: &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory}},
				{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: qtyPtr("512Mi")}},
				{PersistentVolumeClaimName: "shm-pvc"},
			}
			for _, source := range unbounded {
				violations := validateSharedMemoryVolumes(shmVolume("shm", "/dev/shm", source), template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryExceeded))
				Expect(violations[0].Actual).To(Equal(actualUnbounded))
			}
		})
	})

	Context("validateSharedMemorySpec", func() {
		It("rejects a zero or negative sizeLimit", func() {
			for _, size := range []string{"0", "-1Gi"} {
				err := validateSharedMemorySpec(newWorkspace(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr(size)}))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be greater than zero"))
			}
		})

		It("accepts a positive or unset sizeLimit", func() {
			Expect(validateSharedMemorySpec(newWorkspace(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")}))).To(Succeed())
			Expect(validateSharedMemorySpec(newWorkspace(&workspacev1alpha1.SharedMemorySpec{}))).To(Succeed())
			Expect(validateSharedMemorySpec(newWorkspace(nil))).To(Succeed())
		})
	})

	Context("validateTemplateSharedMemoryConsistency", func() {
		It("rejects a zero or negative sizeLimit", func() {
			for _, size := range []string{"0", "-512Mi"} {
				err := validateTemplateSharedMemoryConsistency(newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr(size)}))
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("must be greater than zero"))
			}
		})

		It("accepts a positive sizeLimit, an off switch alone, and no setting", func() {
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("4Gi")}))).To(Succeed())
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)}))).To(Succeed())
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(nil))).To(Succeed())
		})
	})
})
