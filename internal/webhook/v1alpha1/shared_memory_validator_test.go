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
	locked := func() *workspacev1alpha1.SharedMemoryOverridePolicy {
		return &workspacev1alpha1.SharedMemoryOverridePolicy{Allow: boolPtr(false)}
	}
	unlocked := func() *workspacev1alpha1.SharedMemoryOverridePolicy {
		return &workspacev1alpha1.SharedMemoryOverridePolicy{Allow: boolPtr(true)}
	}
	off := func() *workspacev1alpha1.SharedMemorySpec {
		return &workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)}
	}

	newWorkspace := func(sharedMemory *workspacev1alpha1.SharedMemorySpec) *workspacev1alpha1.Workspace {
		return &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName},
			Spec:       workspacev1alpha1.WorkspaceSpec{SharedMemory: sharedMemory},
		}
	}

	Context("validateSharedMemory", func() {
		It("accepts any workspace setting when the template does not lock overrides", func() {
			workspace := newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(true)})
			Expect(validateSharedMemory(workspace, newTemplate(nil, nil))).To(BeEmpty())
			Expect(validateSharedMemory(workspace, newTemplate(off(), nil))).To(BeEmpty())
			Expect(validateSharedMemory(workspace, newTemplate(off(), unlocked()))).To(BeEmpty())
			Expect(validateSharedMemory(workspace, newTemplate(off(), &workspacev1alpha1.SharedMemoryOverridePolicy{}))).To(BeEmpty())
		})

		It("accepts a workspace that sets nothing under a locked template", func() {
			Expect(validateSharedMemory(newWorkspace(nil), newTemplate(off(), locked()))).To(BeEmpty())
		})

		It("accepts a setting that means the same as the locked default, however written", func() {
			Expect(validateSharedMemory(newWorkspace(off()), newTemplate(off(), locked()))).To(BeEmpty())
			on := newTemplate(&workspacev1alpha1.SharedMemorySpec{}, locked())
			Expect(validateSharedMemory(newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(true)}), on)).To(BeEmpty())
			Expect(validateSharedMemory(newWorkspace(&workspacev1alpha1.SharedMemorySpec{}), on)).To(BeEmpty())
		})

		It("rejects a setting that deviates from the locked default", func() {
			template := newTemplate(off(), locked())
			for _, workspace := range []*workspacev1alpha1.Workspace{
				newWorkspace(&workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(true)}),
				newWorkspace(&workspacev1alpha1.SharedMemorySpec{}),
			} {
				violations := validateSharedMemory(workspace, template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryOverrideNotAllowed))
				Expect(violations[0].Field).To(Equal("spec.sharedMemory"))
			}
		})
	})

	Context("validateSharedMemoryVolumes", func() {
		shmVolume := func(name, mountPath string) *workspacev1alpha1.Workspace {
			return &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName},
				Spec: workspacev1alpha1.WorkspaceSpec{Volumes: []workspacev1alpha1.VolumeSpec{{
					Name:      name,
					MountPath: mountPath,
					EmptyDir:  &corev1.EmptyDirVolumeSource{Medium: corev1.StorageMediumMemory},
				}}},
			}
		}

		It("ignores volumes when the template does not lock overrides", func() {
			workspace := shmVolume("shm", "/dev/shm")
			Expect(validateSharedMemoryVolumes(workspace, newTemplate(nil, nil))).To(BeEmpty())
			Expect(validateSharedMemoryVolumes(workspace, newTemplate(off(), nil))).To(BeEmpty())
			Expect(validateSharedMemoryVolumes(workspace, newTemplate(off(), unlocked()))).To(BeEmpty())
		})

		It("ignores volumes mounted elsewhere under a locked template", func() {
			Expect(validateSharedMemoryVolumes(shmVolume("data", "/data"), newTemplate(off(), locked()))).To(BeEmpty())
		})

		It("rejects a volume at /dev/shm under a locked template", func() {
			template := newTemplate(off(), locked())
			for _, mountPath := range []string{"/dev/shm", "/dev/shm/"} {
				violations := validateSharedMemoryVolumes(shmVolume("shm", mountPath), template)
				Expect(violations).To(HaveLen(1))
				Expect(violations[0].Type).To(Equal(ViolationTypeSharedMemoryOverrideNotAllowed))
				Expect(violations[0].Field).To(Equal("spec.volumes[shm].mountPath"))
			}
		})
	})

	Context("validateTemplateSharedMemoryConsistency", func() {
		It("rejects a locked policy without a default", func() {
			err := validateTemplateSharedMemoryConsistency(newTemplate(nil, locked()))
			Expect(err).To(MatchError(ContainSubstring("defaultSharedMemory is not set")))
		})

		It("accepts a locked policy with a default, an unlocked policy, and no policy", func() {
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(off(), locked()))).To(Succeed())
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(&workspacev1alpha1.SharedMemorySpec{}, locked()))).To(Succeed())
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(nil, unlocked()))).To(Succeed())
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(nil, &workspacev1alpha1.SharedMemoryOverridePolicy{}))).To(Succeed())
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(off(), nil))).To(Succeed())
			Expect(validateTemplateSharedMemoryConsistency(newTemplate(nil, nil))).To(Succeed())
		})
	})
})
