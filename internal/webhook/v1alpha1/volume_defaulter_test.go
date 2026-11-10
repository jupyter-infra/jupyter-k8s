/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("VolumeDefaulter", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: testTemplateName},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DefaultVolumes: []workspacev1alpha1.VolumeSpec{
					{Name: "shared-data", PersistentVolumeClaimName: "fsx-shared-pvc", MountPath: testDataMountPath},
					{Name: "models", PersistentVolumeClaimName: "ml-models-pvc", MountPath: "/models"},
				},
			},
		}

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName},
			Spec:       workspacev1alpha1.WorkspaceSpec{DisplayName: testDisplayName},
		}
	})

	Context("applyVolumeDefaults", func() {
		It("should apply default volumes when workspace has none", func() {
			applyVolumeDefaults(workspace, template)

			Expect(workspace.Spec.Volumes).To(HaveLen(2))
			Expect(workspace.Spec.Volumes[0].Name).To(Equal("shared-data"))
			Expect(workspace.Spec.Volumes[0].PersistentVolumeClaimName).To(Equal("fsx-shared-pvc"))
			Expect(workspace.Spec.Volumes[0].MountPath).To(Equal(testDataMountPath))
			Expect(workspace.Spec.Volumes[1].Name).To(Equal("models"))
			Expect(workspace.Spec.Volumes[1].PersistentVolumeClaimName).To(Equal("ml-models-pvc"))
			Expect(workspace.Spec.Volumes[1].MountPath).To(Equal("/models"))
		})

		It("should deep copy default emptyDir volumes", func() {
			sizeLimit := resource.MustParse("1Gi")
			template.Spec.DefaultVolumes = []workspacev1alpha1.VolumeSpec{
				{
					Name:      "shm",
					MountPath: "/dev/shm",
					EmptyDir: &corev1.EmptyDirVolumeSource{
						Medium:    corev1.StorageMediumMemory,
						SizeLimit: &sizeLimit,
					},
				},
			}

			applyVolumeDefaults(workspace, template)

			Expect(workspace.Spec.Volumes).To(HaveLen(1))
			Expect(workspace.Spec.Volumes[0].EmptyDir).NotTo(BeNil())
			Expect(workspace.Spec.Volumes[0].EmptyDir).NotTo(BeIdenticalTo(template.Spec.DefaultVolumes[0].EmptyDir))
			Expect(workspace.Spec.Volumes[0].EmptyDir.SizeLimit).NotTo(BeNil())
			Expect(workspace.Spec.Volumes[0].EmptyDir.SizeLimit).NotTo(BeIdenticalTo(template.Spec.DefaultVolumes[0].EmptyDir.SizeLimit))
			Expect(*workspace.Spec.Volumes[0].EmptyDir.SizeLimit).To(Equal(sizeLimit))
		})

		It("should not override existing workspace volumes", func() {
			workspace.Spec.Volumes = []workspacev1alpha1.VolumeSpec{
				{Name: "my-data", PersistentVolumeClaimName: "my-pvc", MountPath: "/my-data"},
			}

			applyVolumeDefaults(workspace, template)

			Expect(workspace.Spec.Volumes).To(HaveLen(1))
			Expect(workspace.Spec.Volumes[0].Name).To(Equal("my-data"))
		})

		It("should do nothing when template has no default volumes", func() {
			template.Spec.DefaultVolumes = nil

			applyVolumeDefaults(workspace, template)

			Expect(workspace.Spec.Volumes).To(BeEmpty())
		})

		It("should not merge default volumes into a workspace's own volumes when the template locks them", func() {
			template.Spec.VolumeOverrides = &workspacev1alpha1.VolumeOverridePolicy{Allow: boolPtr(false)}
			own := workspacev1alpha1.VolumeSpec{Name: "mine", PersistentVolumeClaimName: "mine-pvc", MountPath: "/mine"}
			workspace.Spec.Volumes = []workspacev1alpha1.VolumeSpec{own}

			applyVolumeDefaults(workspace, template)

			Expect(workspace.Spec.Volumes).To(Equal([]workspacev1alpha1.VolumeSpec{own}))
		})

		It("should copy subPath and readOnly from default volumes", func() {
			template.Spec.DefaultVolumes = []workspacev1alpha1.VolumeSpec{
				{Name: "scratch", PersistentVolumeClaimName: "scratch-pvc", MountPath: "/scratch", SubPath: "namespaces/ml", ReadOnly: true},
			}

			applyVolumeDefaults(workspace, template)

			Expect(workspace.Spec.Volumes).To(Equal(template.Spec.DefaultVolumes))
		})
	})
})
