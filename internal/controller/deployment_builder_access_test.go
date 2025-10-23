package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	workspacesv1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
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
		testWorkspace      *workspacesv1alpha1.Workspace
		testAccessStrategy *workspacesv1alpha1.WorkspaceAccessStrategy
		testDeployment     *appsv1.Deployment
	)

	BeforeEach(func() {
		// Initialize test objects based on config/samples_routing
		scheme = runtime.NewScheme()
		Expect(workspacesv1alpha1.AddToScheme(scheme)).To(Succeed())

		options = WorkspaceControllerOptions{
			ApplicationImagesPullPolicy: corev1.PullIfNotPresent,
			ApplicationImagesRegistry:   "quay.io",
		}

		deploymentBuilder = NewDeploymentBuilder(scheme, options, nil)

		// Create test workspace
		testWorkspace = &workspacesv1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "test-namespace",
			},
			Spec: workspacesv1alpha1.WorkspaceSpec{
				DisplayName:   "Test Workspace",
				Image:         "jupyter/minimal-notebook:latest",
				DesiredStatus: "Running",
				AccessStrategy: &workspacesv1alpha1.AccessStrategyRef{
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
		testAccessStrategy = &workspacesv1alpha1.WorkspaceAccessStrategy{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-access-strategy",
				Namespace: "default",
			},
			Spec: workspacesv1alpha1.WorkspaceAccessStrategySpec{
				DisplayName: "Test Routing Strategy",
				MergeEnv: []workspacesv1alpha1.AccessEnvTemplate{
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
			resolvedEnvVars, err := deploymentBuilder.resolveAccessStrategyEnv(
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
			strategyWithInvalidTemplate.Spec.MergeEnv[0].ValueTemplate = invalidSyntaxTemplate

			_, err := deploymentBuilder.resolveAccessStrategyEnv(
				strategyWithInvalidTemplate,
				testWorkspace,
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse env template"))
		})

		It("Should return an error if an env var template has invalid substitution", func() {
			// Create a copy with a valid template but invalid field reference
			strategyWithInvalidField := testAccessStrategy.DeepCopy()
			strategyWithInvalidField.Spec.MergeEnv[0].ValueTemplate = nonexistentFieldTemplate

			_, err := deploymentBuilder.resolveAccessStrategyEnv(
				strategyWithInvalidField,
				testWorkspace,
			)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to execute env template"))
		})
	})

	Context("addAccessStrategyEnvToContainer", func() {
		It("Should add resolved env variables from strategy.Spec.MergeEnv", func() {
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

		It("Should not affect env variables not specified in strategy.Spec.MergeEnv", func() {
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
			strategyWithInvalidTemplate.Spec.MergeEnv[0].ValueTemplate = invalidSyntaxTemplate

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
			strategyWithInvalidField.Spec.MergeEnv[0].ValueTemplate = nonexistentFieldTemplate

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
		It("Should inject templated strategy.Spec.MergeEnv into the first container specs", func() {
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

		It("Should be a no-op when strategy.Spec.MergeEnv is nil", func() {
			// Create a copy with nil MergeEnv
			strategyWithNilEnv := testAccessStrategy.DeepCopy()
			strategyWithNilEnv.Spec.MergeEnv = nil

			originalEnvVarCount := len(testDeployment.Spec.Template.Spec.Containers[0].Env)

			err := deploymentBuilder.ApplyAccessStrategyToDeployment(
				testDeployment,
				testWorkspace,
				strategyWithNilEnv,
			)

			Expect(err).NotTo(HaveOccurred())
			Expect(testDeployment.Spec.Template.Spec.Containers[0].Env).To(HaveLen(originalEnvVarCount))
		})

		It("Should be a no-op when strategy.Spec.MergeEnv is empty", func() {
			// Create a copy with empty MergeEnv
			strategyWithEmptyEnv := testAccessStrategy.DeepCopy()
			strategyWithEmptyEnv.Spec.MergeEnv = []workspacesv1alpha1.AccessEnvTemplate{}

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

		It("Should add SSM sidecar container for aws-ssm-remote-access strategy", func() {
			// Create SSM access strategy with required sidecar image
			ssmAccessStrategy := &workspacesv1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name: "aws-ssm-remote-access",
				},
				Spec: workspacesv1alpha1.WorkspaceAccessStrategySpec{
					DisplayName: "AWS SSM Remote Access",
					ControllerConfig: map[string]string{
						"SSM_SIDECAR_IMAGE":     "amazon/aws-ssm-agent:latest",
						"SSM_MANAGED_NODE_ROLE": "arn:aws:iam::123456789012:role/SSMRole",
					},
				},
			}

			// Create test deployment with one container
			testDeployment := &appsv1.Deployment{
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							Containers: []corev1.Container{
								{
									Name:  "main-container",
									Image: "test-image:latest",
								},
							},
							Volumes: []corev1.Volume{}, // Start with no volumes
						},
					},
				},
			}

			// Apply SSM access strategy
			err := deploymentBuilder.ApplyAccessStrategyToDeployment(
				testDeployment,
				testWorkspace,
				ssmAccessStrategy,
			)

			Expect(err).NotTo(HaveOccurred())

			// Validate SSM sidecar container was added
			Expect(testDeployment.Spec.Template.Spec.Containers).To(HaveLen(2))

			sidecarContainer := testDeployment.Spec.Template.Spec.Containers[1]
			Expect(sidecarContainer.Name).To(Equal("ssm-agent-sidecar"))
			Expect(sidecarContainer.Image).To(Equal("amazon/aws-ssm-agent:latest"))
			Expect(sidecarContainer.Command).To(Equal([]string{"/bin/sh"}))
			Expect(sidecarContainer.Args).To(HaveLen(2))
			Expect(sidecarContainer.Args[0]).To(Equal("-c"))
			Expect(sidecarContainer.Args[1]).To(ContainSubstring("cp /usr/local/bin/remote-access-server"))

			// Validate readiness probe
			Expect(sidecarContainer.ReadinessProbe).NotTo(BeNil())
			Expect(sidecarContainer.ReadinessProbe.Exec).NotTo(BeNil())
			Expect(sidecarContainer.ReadinessProbe.Exec.Command).To(Equal([]string{"test", "-f", "/tmp/ssm-registered"}))
			Expect(sidecarContainer.ReadinessProbe.InitialDelaySeconds).To(Equal(int32(2)))
			Expect(sidecarContainer.ReadinessProbe.PeriodSeconds).To(Equal(int32(2)))

			// Validate shared volume was created
			Expect(testDeployment.Spec.Template.Spec.Volumes).To(HaveLen(1))
			sharedVolume := testDeployment.Spec.Template.Spec.Volumes[0]
			Expect(sharedVolume.Name).To(Equal("ssm-remote-access"))
			Expect(sharedVolume.EmptyDir).NotTo(BeNil())
			Expect(sharedVolume.EmptyDir.SizeLimit.String()).To(Equal("1Gi"))

			// Validate volume mounts on both containers
			mainContainer := testDeployment.Spec.Template.Spec.Containers[0]
			Expect(mainContainer.VolumeMounts).To(HaveLen(1))
			Expect(mainContainer.VolumeMounts[0].Name).To(Equal("ssm-remote-access"))
			Expect(mainContainer.VolumeMounts[0].MountPath).To(Equal("/ssm-remote-access"))

			Expect(sidecarContainer.VolumeMounts).To(HaveLen(1))
			Expect(sidecarContainer.VolumeMounts[0].Name).To(Equal("ssm-remote-access"))
			Expect(sidecarContainer.VolumeMounts[0].MountPath).To(Equal("/ssm-remote-access"))

			// Validate that controller config values are not injected as environment variables
			// (they should only be available to the controller, not the workspace containers)
			Expect(mainContainer.Env).To(BeEmpty())
		})
	})
})
