/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("EnvDefaulter", func() {
	var (
		workspace *workspacev1alpha1.Workspace
		template  *workspacev1alpha1.WorkspaceTemplate
	)

	BeforeEach(func() {
		workspace = &workspacev1alpha1.Workspace{}
		template = &workspacev1alpha1.WorkspaceTemplate{}
	})

	It("should do nothing when template has no BaseEnv", func() {
		workspace.Spec.Env = []corev1.EnvVar{{Name: "EXISTING", Value: "val"}}

		applyEnvDefaults(workspace, template)

		Expect(workspace.Spec.Env).To(HaveLen(1))
	})

	It("should add all template env vars when workspace has none", func() {
		template.Spec.BaseEnv = []corev1.EnvVar{
			{Name: "A", Value: "1"},
			{Name: "B", Value: "2"},
		}

		applyEnvDefaults(workspace, template)

		Expect(workspace.Spec.Env).To(HaveLen(2))
		Expect(workspace.Spec.Env[0].Name).To(Equal("A"))
		Expect(workspace.Spec.Env[1].Name).To(Equal("B"))
	})

	It("should not override existing workspace env var by name", func() {
		workspace.Spec.Env = []corev1.EnvVar{{Name: "A", Value: "workspace-wins"}}
		template.Spec.BaseEnv = []corev1.EnvVar{{Name: "A", Value: "template-loses"}}

		applyEnvDefaults(workspace, template)

		Expect(workspace.Spec.Env).To(HaveLen(1))
		Expect(workspace.Spec.Env[0].Value).To(Equal("workspace-wins"))
	})

	It("should add non-conflicting template vars alongside workspace vars", func() {
		workspace.Spec.Env = []corev1.EnvVar{{Name: "W", Value: "w-val"}}
		template.Spec.BaseEnv = []corev1.EnvVar{
			{Name: "W", Value: "t-val"},
			{Name: "T", Value: "t-only"},
		}

		applyEnvDefaults(workspace, template)

		Expect(workspace.Spec.Env).To(HaveLen(2))
		Expect(workspace.Spec.Env[0].Value).To(Equal("w-val"))
		Expect(workspace.Spec.Env[1].Name).To(Equal("T"))
	})

	It("should deep copy template env vars to prevent mutation", func() {
		template.Spec.BaseEnv = []corev1.EnvVar{{Name: "X", Value: "original"}}

		applyEnvDefaults(workspace, template)
		workspace.Spec.Env[0].Value = "mutated"

		Expect(template.Spec.BaseEnv[0].Value).To(Equal("original"))
	})
})
