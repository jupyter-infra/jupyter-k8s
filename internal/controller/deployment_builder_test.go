package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("DeploymentBuilder", func() {
	var (
		ctx               context.Context
		deploymentBuilder *DeploymentBuilder
		scheme            *runtime.Scheme
		options           WorkspaceControllerOptions
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(workspacesv1alpha1.AddToScheme(scheme)).To(Succeed())

		options = WorkspaceControllerOptions{
			ApplicationImagesPullPolicy: corev1.PullIfNotPresent,
			ApplicationImagesRegistry:   "quay.io",
		}

		deploymentBuilder = NewDeploymentBuilder(scheme, options, k8sClient)
	})

	Context("Environment Variables", func() {
		It("should pass environment variables from template to container", func() {
			// Create workspace
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{},
			}

			// Create resolved template with environment variables
			resolvedTemplate := &ResolvedTemplate{
				Image: "quay.io/jupyter/minimal-notebook:latest",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
				EnvironmentVariables: []corev1.EnvVar{
					{Name: "JUPYTER_ENABLE_LAB", Value: "yes"},
					{Name: "CUSTOM_ENV", Value: "test-value"},
					{Name: "DEBUG_MODE", Value: "false"},
				},
			}

			// Build deployment
			deployment, err := deploymentBuilder.BuildDeployment(ctx, workspace, resolvedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment).NotTo(BeNil())

			// Verify deployment has correct structure
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := deployment.Spec.Template.Spec.Containers[0]

			// Verify all environment variables are passed to the container
			Expect(container.Env).To(HaveLen(3))

			// Check each environment variable
			envMap := make(map[string]string)
			for _, env := range container.Env {
				envMap[env.Name] = env.Value
			}

			Expect(envMap["JUPYTER_ENABLE_LAB"]).To(Equal("yes"))
			Expect(envMap["CUSTOM_ENV"]).To(Equal("test-value"))
			Expect(envMap["DEBUG_MODE"]).To(Equal("false"))
		})

		It("should not add environment variables when template has none", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-no-env",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{},
			}

			// Create resolved template without environment variables
			resolvedTemplate := &ResolvedTemplate{
				Image: "quay.io/jupyter/minimal-notebook:latest",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
				EnvironmentVariables: []corev1.EnvVar{}, // Empty
			}

			// Build deployment
			deployment, err := deploymentBuilder.BuildDeployment(ctx, workspace, resolvedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment).NotTo(BeNil())

			// Verify no environment variables are added to container
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Env).To(BeEmpty())
		})

		It("should handle nil resolved template gracefully", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-nil-template",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("500m"),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			}

			// Build deployment with nil template
			deployment, err := deploymentBuilder.BuildDeployment(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment).NotTo(BeNil())

			// Verify container has no environment variables
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.Env).To(BeEmpty())
		})
	})

	Context("Storage Configuration", func() {
		It("should mount volume when storage is configured in template", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-storage",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{},
			}

			resolvedTemplate := &ResolvedTemplate{
				Image: "quay.io/jupyter/minimal-notebook:latest",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
				StorageConfiguration: &workspacesv1alpha1.StorageConfig{
					DefaultSize: resource.MustParse("10Gi"),
				},
			}

			deployment, err := deploymentBuilder.BuildDeployment(ctx, workspace, resolvedTemplate)
			Expect(err).NotTo(HaveOccurred())

			// Verify volume is added
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
			Expect(deployment.Spec.Template.Spec.Volumes[0].Name).To(Equal("workspace-storage"))
			Expect(deployment.Spec.Template.Spec.Volumes[0].VolumeSource.PersistentVolumeClaim.ClaimName).To(Equal(GeneratePVCName(workspace.Name)))

			// Verify volume mount is added to container
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.VolumeMounts[0].Name).To(Equal("workspace-storage"))
			Expect(container.VolumeMounts[0].MountPath).To(Equal(DefaultMountPath))
		})

		It("should handle workspace storage override", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-storage-override",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					Storage: &workspacesv1alpha1.StorageSpec{
						Size: resource.MustParse("20Gi"), // Workspace storage takes precedence
					},
				},
			}

			resolvedTemplate := &ResolvedTemplate{
				Image: "quay.io/jupyter/minimal-notebook:latest",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
				StorageConfiguration: &workspacesv1alpha1.StorageConfig{
					DefaultSize: resource.MustParse("10Gi"),
				},
			}

			deployment, err := deploymentBuilder.BuildDeployment(ctx, workspace, resolvedTemplate)
			Expect(err).NotTo(HaveOccurred())

			// Verify volume is still added (workspace storage takes precedence in getStorageConfig)
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(1))
		})
	})
})
