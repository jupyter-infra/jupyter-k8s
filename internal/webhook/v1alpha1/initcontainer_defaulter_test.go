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

var _ = Describe("InitContainerDefaulter", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: testTemplateName},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DefaultInitContainers: []corev1.Container{
					{Name: testInitContainerSetup, Image: testImageBusybox, Command: []string{"sh", "-c", "echo setup"}},
				},
			},
		}

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName},
			Spec:       workspacev1alpha1.WorkspaceSpec{DisplayName: testDisplayName},
		}
	})

	Context("applyInitContainerDefaults", func() {
		It("should apply default init containers when workspace has none", func() {
			applyInitContainerDefaults(workspace, template)

			Expect(workspace.Spec.InitContainers).To(HaveLen(1))
			Expect(workspace.Spec.InitContainers[0].Name).To(Equal(testInitContainerSetup))
			Expect(workspace.Spec.InitContainers[0].Image).To(Equal(testImageBusybox))
		})

		It("should not override existing workspace init containers", func() {
			workspace.Spec.InitContainers = []corev1.Container{
				{Name: "my-init", Image: testImageAlpine},
			}

			applyInitContainerDefaults(workspace, template)

			Expect(workspace.Spec.InitContainers).To(HaveLen(1))
			Expect(workspace.Spec.InitContainers[0].Name).To(Equal("my-init"))
		})

		It("should do nothing when template has no default init containers", func() {
			template.Spec.DefaultInitContainers = nil

			applyInitContainerDefaults(workspace, template)

			Expect(workspace.Spec.InitContainers).To(BeEmpty())
		})
	})
})
