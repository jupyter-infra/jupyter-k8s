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

var _ = Describe("SharedMemoryDefaulter", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: testTemplateName},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DefaultSharedMemory: &workspacev1alpha1.SharedMemorySpec{Enabled: boolPtr(false)},
			},
		}
		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName},
			Spec:       workspacev1alpha1.WorkspaceSpec{DisplayName: testDisplayName},
		}
	})

	It("copies the template default when the workspace sets none", func() {
		applySharedMemoryDefaults(workspace, template)

		Expect(workspace.Spec.SharedMemory).NotTo(BeNil())
		Expect(workspace.Spec.SharedMemory.Enabled).To(HaveValue(BeFalse()))
		Expect(workspace.Spec.SharedMemory).NotTo(BeIdenticalTo(template.Spec.DefaultSharedMemory))
	})

	It("leaves a workspace setting alone, even an empty one", func() {
		for _, own := range []*workspacev1alpha1.SharedMemorySpec{{Enabled: boolPtr(true)}, {}} {
			workspace.Spec.SharedMemory = own.DeepCopy()

			applySharedMemoryDefaults(workspace, template)

			Expect(workspace.Spec.SharedMemory).To(Equal(own))
		}
	})

	It("does nothing when the template sets no default", func() {
		template.Spec.DefaultSharedMemory = nil

		applySharedMemoryDefaults(workspace, template)

		Expect(workspace.Spec.SharedMemory).To(BeNil())
	})
})
