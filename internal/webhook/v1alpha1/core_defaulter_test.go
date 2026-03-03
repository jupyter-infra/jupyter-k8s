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

		It("should apply container config with env variables from template", func() {
			template.Spec.DefaultContainerConfig = &workspacev1alpha1.ContainerConfig{
				Command: []string{"/bin/bash"},
				Args:    []string{"-c", "start-notebook.sh"},
				Env: []corev1.EnvVar{
					{
						Name:  "JUPYTER_ENABLE_LAB",
						Value: "yes",
					},
					{
						Name:  "GRANT_SUDO",
						Value: "yes",
					},
				},
			}

			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.ContainerConfig).NotTo(BeNil())
			Expect(workspace.Spec.ContainerConfig.Env).To(HaveLen(2))
			Expect(workspace.Spec.ContainerConfig.Env[0].Name).To(Equal("JUPYTER_ENABLE_LAB"))
			Expect(workspace.Spec.ContainerConfig.Env[0].Value).To(Equal("yes"))
			Expect(workspace.Spec.ContainerConfig.Env[1].Name).To(Equal("GRANT_SUDO"))
			Expect(workspace.Spec.ContainerConfig.Env[1].Value).To(Equal("yes"))
		})

		It("should not override existing container config", func() {
			workspace.Spec.ContainerConfig = &workspacev1alpha1.ContainerConfig{
				Command: []string{"/bin/sh"},
			}
			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.ContainerConfig.Command).To(Equal([]string{"/bin/sh"}))
		})

		It("should not override existing container config with env variables", func() {
			template.Spec.DefaultContainerConfig = &workspacev1alpha1.ContainerConfig{
				Command: []string{"/bin/bash"},
				Env: []corev1.EnvVar{
					{
						Name:  "TEMPLATE_VAR",
						Value: "template-value",
					},
				},
			}

			workspace.Spec.ContainerConfig = &workspacev1alpha1.ContainerConfig{
				Command: []string{"/bin/sh"},
				Env: []corev1.EnvVar{
					{
						Name:  "WORKSPACE_VAR",
						Value: "workspace-value",
					},
				},
			}

			applyCoreDefaults(workspace, template)
			Expect(workspace.Spec.ContainerConfig.Command).To(Equal([]string{"/bin/sh"}))
			Expect(workspace.Spec.ContainerConfig.Env).To(HaveLen(1))
			Expect(workspace.Spec.ContainerConfig.Env[0].Name).To(Equal("WORKSPACE_VAR"))
			Expect(workspace.Spec.ContainerConfig.Env[0].Value).To(Equal("workspace-value"))
		})

		It("should not inherit template command/args when workspace sets only env", func() {
			template.Spec.DefaultContainerConfig = &workspacev1alpha1.ContainerConfig{
				Command: []string{"/bin/bash"},
				Args:    []string{"-c", "start-notebook.sh"},
				Env: []corev1.EnvVar{
					{
						Name:  "TEMPLATE_VAR",
						Value: "template-value",
					},
				},
			}

			workspace.Spec.ContainerConfig = &workspacev1alpha1.ContainerConfig{
				// Only setting Env, not Command or Args
				Env: []corev1.EnvVar{
					{
						Name:  "WORKSPACE_VAR",
						Value: "workspace-value",
					},
				},
			}

			applyCoreDefaults(workspace, template)

			// Template defaults should NOT be applied since workspace has ContainerConfig
			Expect(workspace.Spec.ContainerConfig.Command).To(BeEmpty())
			Expect(workspace.Spec.ContainerConfig.Args).To(BeEmpty())
			Expect(workspace.Spec.ContainerConfig.Env).To(HaveLen(1))
			Expect(workspace.Spec.ContainerConfig.Env[0].Name).To(Equal("WORKSPACE_VAR"))
			Expect(workspace.Spec.ContainerConfig.Env[0].Value).To(Equal("workspace-value"))
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
