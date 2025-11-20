/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
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
			ObjectMeta: metav1.ObjectMeta{Name: "test-template"},
			Spec: workspacev1alpha1.WorkspaceTemplateSpec{
				DefaultImage:         "jupyter/base-notebook:latest",
				DefaultOwnershipType: "OwnerOnly",
				DefaultContainerConfig: &workspacev1alpha1.ContainerConfig{
					Command: []string{"/bin/bash"},
					Args:    []string{"-c", "start-notebook.sh"},
				},
			},
		}

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{Name: "test-workspace"},
			Spec:       workspacev1alpha1.WorkspaceSpec{DisplayName: "Test"},
		}
	})

	Context("applyCoreDefaults", func() {
		It("should apply image default when empty", func() {
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.Image).To(Equal("jupyter/base-notebook:latest"))
		})

		It("should not override existing image", func() {
			workspace.Spec.Image = "custom/image:latest"
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.Image).To(Equal("custom/image:latest"))
		})

		It("should apply ownership type default when empty", func() {
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.OwnershipType).To(Equal("OwnerOnly"))
		})

		It("should not override existing ownership type", func() {
			workspace.Spec.OwnershipType = "Public"
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.OwnershipType).To(Equal("Public"))
		})

		It("should apply container config default when nil", func() {
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.ContainerConfig).NotTo(BeNil())
			Expect(workspace.Spec.ContainerConfig.Command).To(Equal([]string{"/bin/bash"}))
			Expect(workspace.Spec.ContainerConfig.Args).To(Equal([]string{"-c", "start-notebook.sh"}))
		})

		It("should not override existing container config", func() {
			workspace.Spec.ContainerConfig = &workspacev1alpha1.ContainerConfig{
				Command: []string{"/bin/sh"},
			}
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.ContainerConfig.Command).To(Equal([]string{"/bin/sh"}))
		})

		It("should apply access type defaults", func() {
			template.Spec.DefaultAccessType = "OwnerOnly"

			applyCoreDefaults(workspace, template)

			Expect(workspace.Spec.AccessType).To(Equal("OwnerOnly"))
		})

		It("should not override existing access type", func() {
			workspace.Spec.AccessType = "Public"
			template.Spec.DefaultAccessType = "OwnerOnly"

			applyCoreDefaults(workspace, template)

			Expect(workspace.Spec.AccessType).To(Equal("Public"))
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
