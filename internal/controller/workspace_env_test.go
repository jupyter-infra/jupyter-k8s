package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("Workspace Environment Variables", func() {
	var (
		workspace *workspacev1alpha1.Workspace
		container *corev1.Container
		builder   *DeploymentBuilder
	)

	BeforeEach(func() {
		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "test-namespace",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Image: "jupyter/minimal-notebook:latest",
			},
		}
		container = &corev1.Container{
			Name: "workspace",
			Env:  []corev1.EnvVar{},
		}
		builder = &DeploymentBuilder{}
	})

	Context("addStandardWorkspaceEnvVars", func() {
		It("Should add standard environment variables", func() {
			builder.addStandardWorkspaceEnvVars(container, workspace)

			envMap := make(map[string]string)
			for _, env := range container.Env {
				envMap[env.Name] = env.Value
			}

			Expect(envMap["WORKSPACE_NAMESPACE"]).To(Equal("test-namespace"))
			Expect(envMap["WORKSPACE_NAME"]).To(Equal("test-workspace"))
			// ACCESS_TYPE and APP_TYPE should not be present when empty
			Expect(envMap).NotTo(HaveKey("ACCESS_TYPE"))
			Expect(envMap).NotTo(HaveKey("APP_TYPE"))
		})

		It("Should add AccessType and AppType when specified", func() {
			workspace.Spec.AccessType = "Public"
			workspace.Spec.AppType = "vscode"

			builder.addStandardWorkspaceEnvVars(container, workspace)

			envMap := make(map[string]string)
			for _, env := range container.Env {
				envMap[env.Name] = env.Value
			}

			Expect(envMap["ACCESS_TYPE"]).To(Equal("Public"))
			Expect(envMap["APP_TYPE"]).To(Equal("vscode"))
		})

		It("Should add only AccessType when AppType is empty", func() {
			workspace.Spec.AccessType = "OwnerOnly"

			builder.addStandardWorkspaceEnvVars(container, workspace)

			envMap := make(map[string]string)
			for _, env := range container.Env {
				envMap[env.Name] = env.Value
			}

			Expect(envMap["ACCESS_TYPE"]).To(Equal("OwnerOnly"))
			Expect(envMap).NotTo(HaveKey("APP_TYPE"))
		})
	})
})
