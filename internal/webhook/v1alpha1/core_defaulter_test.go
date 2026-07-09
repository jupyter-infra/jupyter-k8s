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

var _ = Describe("CoreDefaulter", func() {
	var (
		template  *workspacev1alpha1.WorkspaceTemplate
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		template = &workspacev1alpha1.WorkspaceTemplate{
			ObjectMeta: metav1.ObjectMeta{Name: testTemplateName},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DefaultImage:         testValidBaseNotebook,
				DefaultOwnershipType: testOwnershipOwnerOnly,
				DefaultContainerConfig: &workspacev1alpha1.ContainerConfig{
					Command: []string{testBinBash},
					Args:    []string{"-c", testStartNotebookCmd},
				},
			},
		}

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: testWorkspaceName},
			Spec:       workspacev1alpha1.WorkspaceSpec{DisplayName: testDisplayName},
		}
	})

	Context("applyCoreDefaults", func() {
		It("should apply image default when empty", func() {
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.Image).To(Equal(testValidBaseNotebook))
		})

		It("should not override existing image", func() {
			workspace.Spec.Image = "custom/image:latest"
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.Image).To(Equal("custom/image:latest"))
		})

		It("should apply ownership type default when empty", func() {
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.OwnershipType).To(Equal(testOwnershipOwnerOnly))
		})

		It("should not override existing ownership type", func() {
			workspace.Spec.OwnershipType = testOwnershipPublic
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.OwnershipType).To(Equal(testOwnershipPublic))
		})

		It("should apply container config default when nil", func() {
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.ContainerConfig).NotTo(BeNil())
			Expect(workspace.Spec.ContainerConfig.Command).To(Equal([]string{testBinBash}))
			Expect(workspace.Spec.ContainerConfig.Args).To(Equal([]string{"-c", testStartNotebookCmd}))
		})

		It("should apply container config with command/args from template", func() {
			template.Spec.DefaultContainerConfig = &workspacev1alpha1.ContainerConfig{
				Command: []string{testBinBash},
				Args:    []string{"-c", testStartNotebookCmd},
			}

			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.ContainerConfig).NotTo(BeNil())
			Expect(workspace.Spec.ContainerConfig.Command).To(Equal([]string{testBinBash}))
			Expect(workspace.Spec.ContainerConfig.Args).To(Equal([]string{"-c", testStartNotebookCmd}))
		})

		It("should not override existing container config", func() {
			workspace.Spec.ContainerConfig = &workspacev1alpha1.ContainerConfig{
				Command: []string{testBinSh},
			}
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.ContainerConfig.Command).To(Equal([]string{testBinSh}))
		})

		It("should not override existing container config when workspace has its own", func() {
			template.Spec.DefaultContainerConfig = &workspacev1alpha1.ContainerConfig{
				Command: []string{testBinBash},
			}

			workspace.Spec.ContainerConfig = &workspacev1alpha1.ContainerConfig{
				Command: []string{testBinSh},
			}

			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.ContainerConfig.Command).To(Equal([]string{testBinSh}))
		})

		It("should apply access type defaults", func() {
			template.Spec.DefaultAccessType = testOwnershipOwnerOnly

			applyCoreDefaults(workspace, template)

			Expect(workspace.Spec.AccessType).To(Equal(testOwnershipOwnerOnly))
		})

		It("should not override existing access type", func() {
			workspace.Spec.AccessType = testOwnershipPublic
			template.Spec.DefaultAccessType = testOwnershipOwnerOnly

			applyCoreDefaults(workspace, template)

			Expect(workspace.Spec.AccessType).To(Equal(testOwnershipPublic))
		})

		It("should apply app type defaults", func() {
			template.Spec.AppType = "jupyter-lab"

			applyCoreDefaults(workspace, template)

			Expect(workspace.Spec.AppType).To(Equal("jupyter-lab"))
		})

		It("should not override existing app type", func() {
			workspace.Spec.AppType = "vscode"
			template.Spec.AppType = "jupyter-lab"

			applyCoreDefaults(workspace, template)

			Expect(workspace.Spec.AppType).To(Equal("vscode"))
		})
	})
})
