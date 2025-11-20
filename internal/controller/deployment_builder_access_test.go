/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

const (
	invalidSyntaxTemplate    = "{{ .InvalidSyntax }"
	nonexistentFieldTemplate = "{{ .Workspace.NonExistentField }}"
)

var _ = Describe("DeploymentBuilderForAccess", func() {
	var (
		deploymentBuilder  *DeploymentBuilder
		scheme             *runtime.Scheme
		options            WorkspaceControllerOptions
		testWorkspace      *workspacev1alpha1.Workspace
		testAccessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
		testDeployment     *appsv1.Deployment
	)

	BeforeEach(func() {
		// Initialize test objects based on config/samples_routing
		scheme = runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())

		options = WorkspaceControllerOptions{
			ApplicationImagesPullPolicy: corev1.PullIfNotPresent,
			ApplicationImagesRegistry:   "quay.io",
		}

		deploymentBuilder = NewDeploymentBuilder(scheme, options, nil)

		// Create test workspace
		testWorkspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "test-namespace",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName:   "Test Workspace",
				Image:         "jupyter/minimal-notebook:latest",
				DesiredStatus: "Running",
				AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
					Name:      "test-access-strategy",
					Namespace: "default",
				},
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("0.5"),
						corev1.ResourceMemory: resource.MustParse("1Gi"),
					},
				},
			},
		}

		// Create test access strategy with env vars
		testAccessStrategy = &workspacev1alpha1.WorkspaceAccessStrategy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-access-strategy",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
				DisplayName: "Test Routing Strategy",
				DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
					PodModifications: &workspacev1alpha1.PodModifications{
						PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
							MergeEnv: []workspacev1alpha1.AccessEnvTemplate{
								{
									Name:          "JUPYTER_BASE_URL",
									ValueTemplate: "/workspaces/{{ .Workspace.Namespace }}/{{ .Workspace.Name }}/",
								},
								{
									Name:          "WORKSPACE_NAME",
									ValueTemplate: "{{ .Workspace.Name }}",
								},
								{
									Name:          "STRATEGY_NAME",
									ValueTemplate: "{{ .AccessStrategy.Name }}",
								},
							},
						},
					},
				},
				AccessURLTemplate: "https://example.com/workspaces/{{ .Workspace.Namespace }}/{{ .Workspace.Name }}/",
			},
		}

		// Create a test deployment
		testDeployment = &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "workspace-test-workspace",
				Namespace: "test-namespace",
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "workspace-test-workspace",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"app": "workspace-test-workspace",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "workspace",
								Image: "jupyter/minimal-notebook:latest",
								Env: []corev1.EnvVar{
									{
										Name:  "EXISTING_VAR",
										Value: "original-value",
									},
								},
							},
						},
					},
				},
			},
		}
	})

	Context("resolveAccessStrategyEnv", func() {
		It("Should resolve env var values using data from workspace", func() {
			resolvedEnvVars, err := deploymentBuilder.resolveAccessStrategyPrimaryContainerEnv(
				testAccessStrategy,
				testWorkspace,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(resolvedEnvVars).To(HaveLen(3))

			// Check that templates are correctly resolved
			envMap := make(map[string]string)
			for _, env := range resolvedEnvVars {
				envMap[env["name"]] = env["value"]
			}

			Expect(envMap["JUPYTER_BASE_URL"]).To(Equal("/workspaces/test-namespace/test-workspace/"))
			Expect(envMap["WORKSPACE_NAME"]).To(Equal("test-workspace"))
			Expect(envMap["STRATEGY_NAME"]).To(Equal("test-access-strategy"))
		})

		It("Should return an error if an env var template string fails to parse", func() {
			// Create a copy with an invalid template
			strategyWithInvalidTemplate := testAccessStrategy.DeepCopy()
			strategyWithInvalidTemplate.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications.MergeEnv[0].ValueTemplate = invalidSyntaxTemplate

			_, err := deploymentBuilder.resolveAccessStrategyPrimaryContainerEnv(
				strategyWithInvalidTemplate,
				testWorkspace,
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse env template"))
		})

		It("Should return an error if an env var template has invalid substitution", func() {
			// Create a copy with a valid template but invalid field reference
			strategyWithInvalidField := testAccessStrategy.DeepCopy()
			strategyWithInvalidField.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications.MergeEnv[0].ValueTemplate = nonexistentFieldTemplate

			_, err := deploymentBuilder.resolveAccessStrategyPrimaryContainerEnv(
				strategyWithInvalidField,
				testWorkspace,
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to execute env template"))
		})
	})

	Context("addAccessStrategyEnvToContainer", func() {
		It("Should add resolved env variables from PrimaryContainerModifications.MergeEnv", func() {
			container := &corev1.Container{
				Name:  "workspace",
				Image: "jupyter/minimal-notebook:latest",
				Env:   []corev1.EnvVar{},
			}

			err := deploymentBuilder.addAccessStrategyEnvToContainer(
				container,
				testWorkspace,
				testAccessStrategy,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(container.Env).To(HaveLen(3))

			// Check that all env vars from the strategy are added
			envMap := make(map[string]string)
			for _, env := range container.Env {
				envMap[env.Name] = env.Value
			}

			Expect(envMap["JUPYTER_BASE_URL"]).To(Equal("/workspaces/test-namespace/test-workspace/"))
			Expect(envMap["WORKSPACE_NAME"]).To(Equal("test-workspace"))
			Expect(envMap["STRATEGY_NAME"]).To(Equal("test-access-strategy"))
		})

		It("Should override resolved env variables that conflict", func() {
			container := &corev1.Container{
				Name:  "workspace",
				Image: "jupyter/minimal-notebook:latest",
				Env: []corev1.EnvVar{
					{
						Name:  "JUPYTER_BASE_URL",
						Value: "/original/path/",
					},
				},
			}

			err := deploymentBuilder.addAccessStrategyEnvToContainer(
				container,
				testWorkspace,
				testAccessStrategy,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(container.Env).To(HaveLen(3))

			// Check that the existing variable was overridden
			for _, env := range container.Env {
				if env.Name == "JUPYTER_BASE_URL" {
					Expect(env.Value).To(Equal("/workspaces/test-namespace/test-workspace/"))
				}
			}
		})

		It("Should not affect env variables not specified in PrimaryContainerModifications.MergeEnv", func() {
			container := &corev1.Container{
				Name:  "workspace",
				Image: "jupyter/minimal-notebook:latest",
				Env: []corev1.EnvVar{
					{
						Name:  "EXISTING_VAR",
						Value: "original-value",
					},
				},
			}

			err := deploymentBuilder.addAccessStrategyEnvToContainer(
				container,
				testWorkspace,
				testAccessStrategy,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(container.Env).To(HaveLen(4)) // Original + 3 from strategy

			// Check that the existing variable is unchanged
			for _, env := range container.Env {
				if env.Name == "EXISTING_VAR" {
					Expect(env.Value).To(Equal("original-value"))
				}
			}
		})

		It("Should return an error if an env var template string fails to parse", func() {
			container := &corev1.Container{
				Name:  "workspace",
				Image: "jupyter/minimal-notebook:latest",
				Env:   []corev1.EnvVar{},
			}

			// Create a copy with an invalid template
			strategyWithInvalidTemplate := testAccessStrategy.DeepCopy()
			strategyWithInvalidTemplate.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications.MergeEnv[0].ValueTemplate = invalidSyntaxTemplate

			err := deploymentBuilder.addAccessStrategyEnvToContainer(
				container,
				testWorkspace,
				strategyWithInvalidTemplate,
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse env template"))
		})

		It("Should return an error if an env var template has invalid substitution", func() {
			container := &corev1.Container{
				Name:  "workspace",
				Image: "jupyter/minimal-notebook:latest",
				Env:   []corev1.EnvVar{},
			}

			// Create a copy with a valid template but invalid field reference
			strategyWithInvalidField := testAccessStrategy.DeepCopy()
			strategyWithInvalidField.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications.MergeEnv[0].ValueTemplate = nonexistentFieldTemplate

			err := deploymentBuilder.addAccessStrategyEnvToContainer(
				container,
				testWorkspace,
				strategyWithInvalidField,
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to execute env template"))
		})
	})

	Context("ApplyAccessStrategyToDeployment", func() {
		It("Should inject templated PrimaryContainerModifications.MergeEnv into the first container specs", func() {
			err := deploymentBuilder.ApplyAccessStrategyToDeployment(
				testDeployment,
				testWorkspace,
				testAccessStrategy,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(testDeployment.Spec.Template.Spec.Containers).To(HaveLen(1))

			container := testDeployment.Spec.Template.Spec.Containers[0]
			Expect(container.Env).To(HaveLen(4)) // 1 original + 3 from strategy

			// Check that the env vars are correctly added
			envMap := make(map[string]string)
			for _, env := range container.Env {
				envMap[env.Name] = env.Value
			}

			Expect(envMap["JUPYTER_BASE_URL"]).To(Equal("/workspaces/test-namespace/test-workspace/"))
			Expect(envMap["WORKSPACE_NAME"]).To(Equal("test-workspace"))
			Expect(envMap["STRATEGY_NAME"]).To(Equal("test-access-strategy"))
			Expect(envMap["EXISTING_VAR"]).To(Equal("original-value"))
		})

		It("Should be a no-op when PrimaryContainerModifications is nil", func() {
			// Create a copy with nil PrimaryContainerModifications
			strategyWithNilEnv := testAccessStrategy.DeepCopy()
			strategyWithNilEnv.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications = nil

			originalEnvVarCount := len(testDeployment.Spec.Template.Spec.Containers[0].Env)

			err := deploymentBuilder.ApplyAccessStrategyToDeployment(
				testDeployment,
				testWorkspace,
				strategyWithNilEnv,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(testDeployment.Spec.Template.Spec.Containers[0].Env).To(HaveLen(originalEnvVarCount))
		})

		It("Should be a no-op when PrimaryContainerModifications.MergeEnv is empty", func() {
			// Create a copy with empty MergeEnv
			strategyWithEmptyEnv := testAccessStrategy.DeepCopy()
			strategyWithEmptyEnv.Spec.DeploymentModifications.PodModifications.PrimaryContainerModifications.MergeEnv = []workspacev1alpha1.AccessEnvTemplate{}

			originalEnvVarCount := len(testDeployment.Spec.Template.Spec.Containers[0].Env)

			err := deploymentBuilder.ApplyAccessStrategyToDeployment(
				testDeployment,
				testWorkspace,
				strategyWithEmptyEnv,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(testDeployment.Spec.Template.Spec.Containers[0].Env).To(HaveLen(originalEnvVarCount))
		})

		It("Should be a no-op when access strategy is nil", func() {
			originalEnvVarCount := len(testDeployment.Spec.Template.Spec.Containers[0].Env)

			err := deploymentBuilder.ApplyAccessStrategyToDeployment(
				testDeployment,
				testWorkspace,
				nil,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(testDeployment.Spec.Template.Spec.Containers[0].Env).To(HaveLen(originalEnvVarCount))
		})

		It("Should return an error when deployment is nil", func() {
			err := deploymentBuilder.ApplyAccessStrategyToDeployment(
				nil,
				testWorkspace,
				testAccessStrategy,
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("cannot apply AccessStrategy"))
		})

		It("Should apply deployment spec modifications with additional container, resource requirements, and shared volume", func() {
			// Create access strategy with deploymentSpecModifications
			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-deployment-modifications",
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName: "Test Deployment Modifications",
					DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
						PodModifications: &workspacev1alpha1.PodModifications{
							InitContainers: []corev1.Container{
								{
									Name:    "setup-init",
									Image:   "alpine:3.18",
									Command: []string{"/bin/sh"},
									Args: []string{
										"-c",
										"echo 'Initializing shared storage' && mkdir -p /shared-data && touch /shared-data/initialized",
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "shared-storage",
											MountPath: "/shared-data",
										},
									},
								},
							},
							AdditionalContainers: []corev1.Container{
								{
									Name:    "helper-sidecar",
									Image:   "busybox:1.35",
									Command: []string{"/bin/sh"},
									Args: []string{
										"-c",
										"echo 'Setting up shared resources' && cp /usr/bin/helper /shared-data/ && sleep infinity",
									},
									Resources: corev1.ResourceRequirements{
										Requests: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("100m"),
											corev1.ResourceMemory: resource.MustParse("128Mi"),
										},
										Limits: corev1.ResourceList{
											corev1.ResourceCPU:    resource.MustParse("200m"),
											corev1.ResourceMemory: resource.MustParse("256Mi"),
										},
									},
									VolumeMounts: []corev1.VolumeMount{
										{
											Name:      "shared-storage",
											MountPath: "/shared-data",
										},
									},
									ReadinessProbe: &corev1.Probe{
										ProbeHandler: corev1.ProbeHandler{
											Exec: &corev1.ExecAction{
												Command: []string{"test", "-f", "/shared-data/ready"},
											},
										},
										InitialDelaySeconds: 5,
										PeriodSeconds:       10,
									},
								},
							},
							Volumes: []corev1.Volume{
								{
									Name: "shared-storage",
									VolumeSource: corev1.VolumeSource{
										EmptyDir: &corev1.EmptyDirVolumeSource{
											SizeLimit: func() *resource.Quantity {
												limit := resource.MustParse("2Gi")
												return &limit
											}(),
										},
									},
								},
							},
							PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      "shared-storage",
										MountPath: "/shared-data",
									},
								},
							},
						},
					},
				},
			}

			// Apply access strategy through ApplyAccessStrategyToDeployment
			err := deploymentBuilder.ApplyAccessStrategyToDeployment(testDeployment, testWorkspace, accessStrategy)
			Expect(err).NotTo(HaveOccurred())

			// Validate init container was added
			Expect(testDeployment.Spec.Template.Spec.InitContainers).To(HaveLen(1))

			initContainer := testDeployment.Spec.Template.Spec.InitContainers[0]
			Expect(initContainer.Name).To(Equal("setup-init"))
			Expect(initContainer.Image).To(Equal("alpine:3.18"))
			Expect(initContainer.Command).To(Equal([]string{"/bin/sh"}))
			Expect(initContainer.Args).To(HaveLen(2))
			Expect(initContainer.Args[1]).To(ContainSubstring("Initializing shared storage"))
			Expect(initContainer.VolumeMounts).To(HaveLen(1))
			Expect(initContainer.VolumeMounts[0].Name).To(Equal("shared-storage"))

			// Validate additional container was added
			Expect(testDeployment.Spec.Template.Spec.Containers).To(HaveLen(2))

			// Validate primary container remains first
			primaryContainer := testDeployment.Spec.Template.Spec.Containers[0]
			Expect(primaryContainer.Name).To(Equal("workspace"))

			// Validate additional container details
			additionalContainer := testDeployment.Spec.Template.Spec.Containers[1]
			Expect(additionalContainer.Name).To(Equal("helper-sidecar"))
			Expect(additionalContainer.Image).To(Equal("busybox:1.35"))
			Expect(additionalContainer.Command).To(Equal([]string{"/bin/sh"}))
			Expect(additionalContainer.Args).To(HaveLen(2))
			Expect(additionalContainer.Args[0]).To(Equal("-c"))
			Expect(additionalContainer.Args[1]).To(ContainSubstring("Setting up shared resources"))
			Expect(additionalContainer.Args[1]).To(ContainSubstring("sleep infinity"))

			// Validate additional container resource requirements
			Expect(additionalContainer.Resources.Requests).NotTo(BeNil())
			Expect(additionalContainer.Resources.Requests[corev1.ResourceCPU]).To(Equal(resource.MustParse("100m")))
			Expect(additionalContainer.Resources.Requests[corev1.ResourceMemory]).To(Equal(resource.MustParse("128Mi")))
			Expect(additionalContainer.Resources.Limits).NotTo(BeNil())
			Expect(additionalContainer.Resources.Limits[corev1.ResourceCPU]).To(Equal(resource.MustParse("200m")))
			Expect(additionalContainer.Resources.Limits[corev1.ResourceMemory]).To(Equal(resource.MustParse("256Mi")))

			// Validate additional container readiness probe
			Expect(additionalContainer.ReadinessProbe).NotTo(BeNil())
			Expect(additionalContainer.ReadinessProbe.Exec).NotTo(BeNil())
			Expect(additionalContainer.ReadinessProbe.Exec.Command).To(Equal([]string{"test", "-f", "/shared-data/ready"}))
			Expect(additionalContainer.ReadinessProbe.InitialDelaySeconds).To(Equal(int32(5)))
			Expect(additionalContainer.ReadinessProbe.PeriodSeconds).To(Equal(int32(10)))

			// Validate shared volume was created
			Expect(testDeployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
			sharedVolume := testDeployment.Spec.Template.Spec.Volumes[0]
			Expect(sharedVolume.Name).To(Equal("shared-storage"))
			Expect(sharedVolume.EmptyDir).NotTo(BeNil())
			Expect(sharedVolume.EmptyDir.SizeLimit.String()).To(Equal("2Gi"))

			// Validate volume mounts on primary container
			Expect(primaryContainer.VolumeMounts).To(HaveLen(1))
			Expect(primaryContainer.VolumeMounts[0].Name).To(Equal("shared-storage"))
			Expect(primaryContainer.VolumeMounts[0].MountPath).To(Equal("/shared-data"))

			// Validate volume mounts on additional container
			Expect(additionalContainer.VolumeMounts).To(HaveLen(1))
			Expect(additionalContainer.VolumeMounts[0].Name).To(Equal("shared-storage"))
			Expect(additionalContainer.VolumeMounts[0].MountPath).To(Equal("/shared-data"))
		})
	})
})
