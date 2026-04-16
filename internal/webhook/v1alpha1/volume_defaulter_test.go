/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
			ObjectMeta: metav1.ObjectMeta{Name: "test-template"},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DefaultVolumes: []workspacev1alpha1.VolumeSpec{
					{Name: "shared-data", PersistentVolumeClaimName: "fsx-shared-pvc", MountPath: "/data"},
					{Name: "models", PersistentVolumeClaimName: "ml-models-pvc", MountPath: "/models"},
				},
			},
		}

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "test-workspace"},
			Spec:       workspacev1alpha1.WorkspaceSpec{DisplayName: "Test"},
		}
	})

	Context("applyVolumeDefaults", func() {
		It("should apply default volumes when workspace has none", func() {
			applyVolumeDefaults(workspace, template)

			Expect(workspace.Spec.Volumes).To(HaveLen(2))
			Expect(workspace.Spec.Volumes[0].Name).To(Equal("shared-data"))
			Expect(workspace.Spec.Volumes[0].PersistentVolumeClaimName).To(Equal("fsx-shared-pvc"))
			Expect(workspace.Spec.Volumes[0].MountPath).To(Equal("/data"))
			Expect(workspace.Spec.Volumes[1].Name).To(Equal("models"))
			Expect(workspace.Spec.Volumes[1].PersistentVolumeClaimName).To(Equal("ml-models-pvc"))
			Expect(workspace.Spec.Volumes[1].MountPath).To(Equal("/models"))
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
	})
})
