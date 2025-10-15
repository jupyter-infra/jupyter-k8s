/*
MIT License

Copyright (c) 2025 jupyter-ai-contrib

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
*/

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

	Context("Additional Volumes", func() {
		It("should mount additional PVC volumes", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-volumes",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					Storage: &workspacesv1alpha1.StorageSpec{
						Size: resource.MustParse("1Gi"),
					},
					Volumes: []workspacesv1alpha1.VolumeSpec{
						{
							Name:                      "data-volume",
							PersistentVolumeClaimName: "data-pvc",
							MountPath:                 "/data",
						},
						{
							Name:                      "shared-volume",
							PersistentVolumeClaimName: "shared-pvc",
							MountPath:                 "/shared",
						},
					},
				},
			}

			resolvedTemplate := &ResolvedTemplate{
				Image: "quay.io/jupyter/minimal-notebook:latest",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
				StorageConfiguration: &workspacesv1alpha1.StorageConfig{
					DefaultSize: resource.MustParse("1Gi"),
				},
			}

			deployment, err := deploymentBuilder.BuildDeployment(ctx, workspace, resolvedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment).NotTo(BeNil())

			container := deployment.Spec.Template.Spec.Containers[0]

			// Check volume mounts
			Expect(container.VolumeMounts).To(HaveLen(3)) // workspace-storage + 2 additional

			volumeMountMap := make(map[string]string)
			for _, vm := range container.VolumeMounts {
				volumeMountMap[vm.Name] = vm.MountPath
			}

			Expect(volumeMountMap["data-volume"]).To(Equal("/data"))
			Expect(volumeMountMap["shared-volume"]).To(Equal("/shared"))

			// Check volumes
			Expect(deployment.Spec.Template.Spec.Volumes).To(HaveLen(3)) // workspace-storage + 2 additional

			volumeMap := make(map[string]string)
			for _, v := range deployment.Spec.Template.Spec.Volumes {
				if v.PersistentVolumeClaim != nil {
					volumeMap[v.Name] = v.PersistentVolumeClaim.ClaimName
				}
			}

			Expect(volumeMap["data-volume"]).To(Equal("data-pvc"))
			Expect(volumeMap["shared-volume"]).To(Equal("shared-pvc"))
		})
	})

	Context("Container Configuration", func() {
		It("should set custom command and args", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-container-config",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					ContainerConfig: &workspacesv1alpha1.ContainerConfig{
						Command: []string{"/bin/bash"},
						Args:    []string{"-c", "echo 'test' && sleep 3600"},
					},
				},
			}

			resolvedTemplate := &ResolvedTemplate{
				Image: "quay.io/jupyter/minimal-notebook:latest",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			}

			deployment, err := deploymentBuilder.BuildDeployment(ctx, workspace, resolvedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment).NotTo(BeNil())

			container := deployment.Spec.Template.Spec.Containers[0]

			Expect(container.Command).To(Equal([]string{"/bin/bash"}))
			Expect(container.Args).To(Equal([]string{"-c", "echo 'test' && sleep 3600"}))
		})
	})

	Context("Node Selector", func() {
		It("should set node selector constraints", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-node-selector",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					NodeSelector: map[string]string{
						"node.kubernetes.io/instance-type": "ml.t3.large",
						"kubernetes.io/arch":               "amd64",
					},
				},
			}

			resolvedTemplate := &ResolvedTemplate{
				Image: "quay.io/jupyter/minimal-notebook:latest",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			}

			deployment, err := deploymentBuilder.BuildDeployment(ctx, workspace, resolvedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment).NotTo(BeNil())

			nodeSelector := deployment.Spec.Template.Spec.NodeSelector

			Expect(nodeSelector).To(HaveKeyWithValue("node.kubernetes.io/instance-type", "ml.t3.large"))
			Expect(nodeSelector).To(HaveKeyWithValue("kubernetes.io/arch", "amd64"))
		})
	})

	Context("Lifecycle Hooks", func() {
		It("should set lifecycle hooks", func() {
			workspace := &workspacesv1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-lifecycle",
					Namespace: "default",
				},
				Spec: workspacesv1alpha1.WorkspaceSpec{
					Lifecycle: &corev1.Lifecycle{
						PostStart: &corev1.LifecycleHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"/bin/sh", "-c", "echo 'started' > /tmp/started"},
							},
						},
						PreStop: &corev1.LifecycleHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"/bin/sh", "-c", "echo 'stopping' > /tmp/stopping"},
							},
						},
					},
				},
			}

			resolvedTemplate := &ResolvedTemplate{
				Image: "quay.io/jupyter/minimal-notebook:latest",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			}

			deployment, err := deploymentBuilder.BuildDeployment(ctx, workspace, resolvedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment).NotTo(BeNil())

			container := deployment.Spec.Template.Spec.Containers[0]

			Expect(container.Lifecycle).NotTo(BeNil())
			Expect(container.Lifecycle.PostStart).NotTo(BeNil())
			Expect(container.Lifecycle.PostStart.Exec).NotTo(BeNil())
			Expect(container.Lifecycle.PostStart.Exec.Command).To(Equal([]string{"/bin/sh", "-c", "echo 'started' > /tmp/started"}))

			Expect(container.Lifecycle.PreStop).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop.Exec).NotTo(BeNil())
			Expect(container.Lifecycle.PreStop.Exec.Command).To(Equal([]string{"/bin/sh", "-c", "echo 'stopping' > /tmp/stopping"}))
		})
	})
})
