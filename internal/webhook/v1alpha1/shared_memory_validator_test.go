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
	newTemplate := func(def *workspacev1alpha1.SharedMemorySpec, policy *workspacev1alpha1.SharedMemoryOverridePolicy) *workspacev1alpha1.WorkspaceTemplate {
		return &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: testTemplateName},
			Spec:       workspacev1alpha1.WorkspaceTemplateSpec{DefaultSharedMemory: def, SharedMemoryOverrides: policy},
		}
	}
	noOverrides := func() *workspacev1alpha1.SharedMemoryOverridePolicy {
		return &workspacev1alpha1.SharedMemoryOverridePolicy{Allow: boolPtr(false)}
	}
	maxSize := func(size string) *workspacev1alpha1.SharedMemoryOverridePolicy {
		return &workspacev1alpha1.SharedMemoryOverridePolicy{MaxSizeLimit: qtyPtr(size)}
	}

	newWorkspace := func(sharedMemory *workspacev1alpha1.SharedMemorySpec) *workspacev1alpha1.Workspace {
		return &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName},
			Spec:       workspacev1alpha1.WorkspaceSpec{SharedMemory: sharedMemory},
		}
	}

	Context("validateSharedMemory", func() {
		It("accepts any workspace setting when the template sets no overrides", func() {
			workspace := newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(true), SizeLimit: qtyPtr("64Gi")})
			Expect(validateSharedMemory(workspace, newTemplate(nil, nil))).To(BeEmpty())
			Expect(validateSharedMemory(workspace, newTemplate(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)}, nil))).To(BeEmpty())
		})

		It("accepts a workspace that sets nothing", func() {
			Expect(validateSharedMemory(newWorkspace(nil), newTemplate(nil, noOverrides()))).To(BeEmpty())
			Expect(validateSharedMemory(newWorkspace(nil), newTemplate(nil, maxSize("1Gi")))).To(BeEmpty())
		})

		It("rejects any deviation from the default when overrides are not allowed", func() {
			def := &workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)}
			template := newTemplate(def, noOverrides())
			for _, workspace := range []*workspacev1alpha1.Workspace{
				newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(true)}),
				newWorkspace(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")}),
				newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false), SizeLimit: qtyPtr("1Gi")}),
				newWorkspace(&workspacev1alpha1.SharedMemorySpec{}),
			} {
				violations := validateSharedMemory(workspace, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryOverrideNotAllowed))
				Expect(violations[0].Field).To(Equal("spec.sharedMemory"))
			}
		})

		It("accepts the copied default when overrides are not allowed", func() {
			def := &workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(true), SizeLimit: qtyPtr("1Gi")}
			Expect(validateSharedMemory(newWorkspace(def.DeepCopy()), newTemplate(def, noOverrides()))).To(BeEmpty())
		})

		It("rejects a sizeLimit above maxSizeLimit", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")}, maxSize("1Gi"))
			violations := validateSharedMemory(newWorkspace(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("2Gi")}), template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryExceeded))
			Expect(violations[0].Field).To(Equal("spec.sharedMemory.sizeLimit"))
			Expect(violations[0].Actual).To(Equal("2Gi"))
		})

		It("rejects an enabled volume without sizeLimit under maxSizeLimit", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")}, maxSize("1Gi"))
			for _, workspace := range []*workspacev1alpha1.Workspace{
				newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(true)}),
				newWorkspace(&workspacev1alpha1.SharedMemorySpec{}),
			} {
				violations := validateSharedMemory(workspace, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryExceeded))
				Expect(violations[0].Actual).To(Equal("unset"))
			}
		})

		It("accepts a sizeLimit at or below maxSizeLimit", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")}, maxSize("1Gi"))
			for _, size := range []string{"1Gi", "512Mi", "1G"} {
				Expect(validateSharedMemory(newWorkspace(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr(size)}), template)).To(BeEmpty())
			}
		})

		It("ignores maxSizeLimit for a workspace that disables the volume", func() {
			template := newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")}, maxSize("1Gi"))
			workspace := newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false), SizeLimit: qtyPtr("64Gi")})
			Expect(validateSharedMemory(workspace, template)).To(BeEmpty())
		})

		It("reports both violations when a deviating workspace also exceeds maxSizeLimit", func() {
			def := &workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")}
			policy := &workspacev1alpha1.SharedMemoryOverridePolicy{Allow: boolPtr(false), MaxSizeLimit: qtyPtr("1Gi")}
			violations := validateSharedMemory(newWorkspace(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("2Gi")}), newTemplate(def, policy))
			Expect(violations).To(HaveLen(2))
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

		It("ignores volumes when the template sets no overrides", func() {
			workspace := shmVolume("shm", "/dev/shm", memoryEmptyDir("64Gi"))
			Expect(validateSharedMemoryVolumes(workspace, newTemplate(nil, nil))).To(BeEmpty())
			Expect(validateSharedMemoryVolumes(workspace, newTemplate(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)}, nil))).To(BeEmpty())
		})

		It("ignores volumes mounted elsewhere", func() {
			template := newTemplate(nil, noOverrides())
			Expect(validateSharedMemoryVolumes(shmVolume("data", "/data", memoryEmptyDir("64Gi")), template)).To(BeEmpty())
		})

		It("rejects a volume at /dev/shm when overrides are not allowed", func() {
			template := newTemplate(nil, noOverrides())
			for _, mountPath := range []string{"/dev/shm", "/dev/shm/"} {
				violations := validateSharedMemoryVolumes(shmVolume("shm", mountPath, memoryEmptyDir("64Mi")), template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryOverrideNotAllowed))
				Expect(violations[0].Field).To(Equal("spec.volumes[shm].mountPath"))
			}
		})

		It("accepts any volume at /dev/shm when overrides are allowed without maxSizeLimit", func() {
			template := newTemplate(nil, &workspacev1alpha1.SharedMemoryOverridePolicy{Allow: boolPtr(true)})
			Expect(validateSharedMemoryVolumes(shmVolume("shm", "/dev/shm", workspacev1alpha1.VolumeSpec{PersistentVolumeClaimName: "shm-pvc"}), template)).To(BeEmpty())
		})

		It("bounds a memory-backed volume at /dev/shm by maxSizeLimit", func() {
			template := newTemplate(nil, maxSize("1Gi"))
			Expect(validateSharedMemoryVolumes(shmVolume("shm", "/dev/shm", memoryEmptyDir("1Gi")), template)).To(BeEmpty())
			Expect(validateSharedMemoryVolumes(shmVolume("shm", "/dev/shm", memoryEmptyDir("512Mi")), template)).To(BeEmpty())

			violations := validateSharedMemoryVolumes(shmVolume("shm", "/dev/shm", memoryEmptyDir("2Gi")), template)
			Expect(violations).To(HaveLen(1))
			Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryExceeded))
			Expect(violations[0].Field).To(Equal("spec.volumes[shm].emptyDir.sizeLimit"))
			Expect(violations[0].Actual).To(Equal("2Gi"))
		})

		It("rejects an unbounded volume at /dev/shm under maxSizeLimit", func() {
			template := newTemplate(nil, maxSize("1Gi"))
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
		It("rejects a zero or negative default sizeLimit or maxSizeLimit", func() {
			for _, size := range []string{"0", "-512Mi"} {
				err := validateTemplateSharedMemoryConsistency(newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr(size)}, nil))
				Expect(err).To(MatchError(ContainSubstring("defaultSharedMemory.sizeLimit")))
				Expect(err).To(MatchError(ContainSubstring("must be greater than zero")))

				err = validateTemplateSharedMemoryConsistency(newTemplate(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)}, maxSize(size)))
				Expect(err).To(MatchError(ContainSubstring("sharedMemoryOverrides.maxSizeLimit")))
				Expect(err).To(MatchError(ContainSubstring("must be greater than zero")))
			}
		})

		It("rejects maxSizeLimit without an enabled default sized within it", func() {
			for _, def := range []*workspacev1alpha1.SharedMemorySpec{nil, {}, {Enabled: boolPtr(true)}} {
				err := validateTemplateSharedMemoryConsistency(newTemplate(def, maxSize("1Gi")))
				Expect(err).To(MatchError(ContainSubstring("requires a defaultSharedMemory with a sizeLimit at or below it")))
			}
			err := validateTemplateSharedMemoryConsistency(newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("2Gi")}, maxSize("1Gi")))
			Expect(err).To(MatchError(ContainSubstring("exceeds sharedMemoryOverrides.maxSizeLimit")))
		})

		It("accepts a default within maxSizeLimit, a disabled default, and either field alone", func() {
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("1Gi")}, maxSize("1Gi")))).To(Succeed())
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("512Mi")}, maxSize("1Gi")))).To(Succeed())
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)}, maxSize("1Gi")))).To(Succeed())
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(&workspacev1alpha1.SharedMemorySpec{SizeLimit: qtyPtr("4Gi")}, nil))).To(Succeed())
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(nil, noOverrides()))).To(Succeed())
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(nil, nil))).To(Succeed())
		})
	})
})
