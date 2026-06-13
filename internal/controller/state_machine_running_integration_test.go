/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("reconcileDesiredRunningStatus integration strategy", func() {
	var ctx context.Context

	createReadyDeployment := func(ws *workspacev1alpha1.Workspace) *appsv1.Deployment {
		replicas := int32(1)
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateDeploymentName(ws.Name),
				Namespace: ws.Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "test"},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "test"}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  "main",
							Image: "jupyter/base-notebook:latest",
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, dep)).To(Succeed())

		dep.Status.AvailableReplicas = 1
		dep.Status.ReadyReplicas = 1
		dep.Status.Replicas = 1
		dep.Status.Conditions = []appsv1.DeploymentCondition{{
			Type:   appsv1.DeploymentAvailable,
			Status: corev1.ConditionTrue,
		}}
		Expect(k8sClient.Status().Update(ctx, dep)).To(Succeed())
		return dep
	}

	createService := func(ws *workspacev1alpha1.Workspace) *corev1.Service {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateServiceName(ws.Name),
				Namespace: ws.Namespace,
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{{
					Port:       8888,
					TargetPort: intstr.FromInt32(8888),
				}},
				Selector: map[string]string{"app": "test"},
			},
		}
		Expect(k8sClient.Create(ctx, svc)).To(Succeed())
		return svc
	}

	buildStateMachine := func() *StateMachine {
		statusManager := NewStatusManager(k8sClient)
		rm := NewResourceManager(
			k8sClient,
			scheme.Scheme,
			NewDeploymentBuilder(scheme.Scheme, WorkspaceControllerOptions{}, k8sClient),
			NewServiceBuilder(scheme.Scheme),
			NewPVCBuilder(scheme.Scheme),
			NewAccessResourcesBuilder(),
			statusManager,
		)
		return &StateMachine{
			resourceManager:     rm,
			statusManager:       statusManager,
			accessStartupProber: &mockAccessStartupProber{},
			recorder:            record.NewFakeRecorder(10),
		}
	}

	getDeployment := func(ws *workspacev1alpha1.Workspace) *appsv1.Deployment {
		dep := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, client.ObjectKey{
			Name:      GenerateDeploymentName(ws.Name),
			Namespace: ws.Namespace,
		}, dep)).To(Succeed())
		return dep
	}

	BeforeEach(func() {
		ctx = context.Background()
	})

	Context("workspace references an integration strategy", func() {
		var integrationStrategy *workspacev1alpha1.WorkspaceIntegrationStrategy

		newWorkspaceWithIntegrationStrategy := func() *workspacev1alpha1.Workspace {
			ws := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("sm-int-%d", time.Now().UnixNano()),
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					Image:         "jupyter/base-notebook:latest",
					DesiredStatus: DesiredStateRunning,
					IntegrationStrategy: &workspacev1alpha1.IntegrationStrategyRef{
						Name: "test-integration-strategy",
						Parameters: []workspacev1alpha1.IntegrationParameter{
							{Name: testParamClusterName, Value: "my-cluster"},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, ws)).To(Succeed())
			return ws
		}

		BeforeEach(func() {
			// A strategy with no resourceLookup so resolution relies only on
			// {{ .Workspace.* }} and {{ .Parameters.* }} expressions. This keeps the
			// test focused on the state-machine -> deployment build seam without
			// having to seed an external looked-up resource.
			integrationStrategy = &workspacev1alpha1.WorkspaceIntegrationStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-integration-strategy",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceIntegrationStrategySpec{
					DisplayName: "Test Integration Strategy",
					DeploymentModifications: &workspacev1alpha1.DeploymentModifications{
						PodModifications: &workspacev1alpha1.PodModifications{
							AdditionalContainers: []corev1.Container{
								{Name: "ray-connector", Image: "ray:2.9"},
							},
							Volumes: []corev1.Volume{
								{Name: "ray-tmp"},
							},
							PrimaryContainerModifications: &workspacev1alpha1.PrimaryContainerModifications{
								VolumeMounts: []corev1.VolumeMount{
									{Name: "ray-tmp", MountPath: "/tmp/ray"},
								},
								MergeEnv: []workspacev1alpha1.AccessEnvTemplate{
									{Name: testEnvRayAddress, ValueTemplate: "{{ .Parameters.clusterName }}.{{ .Workspace.Namespace }}:10001"},
								},
							},
						},
					},
				},
			}
		})

		It("applies the integration strategy modifications to a newly created deployment", func() {
			workspace := newWorkspaceWithIntegrationStrategy()
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() {
				dep := &appsv1.Deployment{}
				if k8sClient.Get(ctx, client.ObjectKey{Name: GenerateDeploymentName(workspace.Name), Namespace: workspace.Namespace}, dep) == nil {
					_ = k8sClient.Delete(ctx, dep)
				}
			}()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			sm := buildStateMachine()
			// First reconcile creates the deployment (it will not be ready yet, so
			// the workspace is reported as Starting and a requeue is requested).
			result, err := sm.ReconcileDesiredState(ctx, workspace, integrationStrategy, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(PollRequeueDelay))

			dep := getDeployment(workspace)

			// Sidecar appended after the primary container
			containers := containerNames(dep.Spec.Template.Spec.Containers)
			Expect(containers["ray-connector"]).To(BeTrue(), "sidecar container should be present")

			// Volume appended
			Expect(volumeNames(dep.Spec.Template.Spec.Volumes)["ray-tmp"]).To(BeTrue(), "volume should be present")

			// Volume mount appended to the primary container
			var mountFound bool
			for _, m := range dep.Spec.Template.Spec.Containers[0].VolumeMounts {
				if m.Name == "ray-tmp" && m.MountPath == "/tmp/ray" {
					mountFound = true
				}
			}
			Expect(mountFound).To(BeTrue(), "primary container should have the ray-tmp mount")

			// Templated env resolved against workspace + parameters and merged
			// into the primary container.
			env := envByName(dep.Spec.Template.Spec.Containers[0].Env)
			Expect(env[testEnvRayAddress]).To(Equal("my-cluster.default:10001"))
		})

		It("marks the workspace Available once the deployment and service are ready", func() {
			workspace := newWorkspaceWithIntegrationStrategy()
			// Pre-create a ready deployment and service so the first reconcile sees
			// ready resources and transitions straight to Available.
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, integrationStrategy, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(Equal(PollRequeueDelay))

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
			var available *metav1.Condition
			for i := range workspace.Status.Conditions {
				if workspace.Status.Conditions[i].Type == ConditionTypeAvailable {
					available = &workspace.Status.Conditions[i]
				}
			}
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionTrue))
		})
	})
})
