/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
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
