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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("reconcileDesiredRunningStatus probe integration", func() {
	var (
		ctx        context.Context
		mockProber *mockAccessStartupProber
	)

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
			accessStartupProber: mockProber,
			recorder:            record.NewFakeRecorder(10),
		}
	}

	getCondition := func(ws *workspacev1alpha1.Workspace, condType string) *metav1.Condition {
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ws), ws)).To(Succeed())
		for i := range ws.Status.Conditions {
			if ws.Status.Conditions[i].Type == condType {
				return &ws.Status.Conditions[i]
			}
		}
		return nil
	}

	BeforeEach(func() {
		ctx = context.Background()
		mockProber = &mockAccessStartupProber{}
	})

	Context("no access strategy and no probe", func() {
		It("should mark workspace as Available", func() {
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("sm-test-%d", time.Now().UnixNano()),
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					Image:         "jupyter/base-notebook:latest",
					DesiredStatus: DesiredStateRunning,
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(Equal(PollRequeueDelay))

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionTrue))
			Expect(available.Reason).To(Equal(ReasonResourcesReady))
		})
	})

	Context("probe configured", func() {
		var accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy

		newWorkspaceWithAccessStrategy := func() *workspacev1alpha1.Workspace {
			ws := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("sm-test-%d", time.Now().UnixNano()),
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					Image:         "jupyter/base-notebook:latest",
					DesiredStatus: DesiredStateRunning,
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name: "test-strategy",
					},
				},
			}
			Expect(k8sClient.Create(ctx, ws)).To(Succeed())
			return ws
		}

		BeforeEach(func() {
			accessStrategy = &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-strategy",
					Namespace:  "default",
					UID:        types.UID("test-uid"),
					Generation: 1,
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName:             "Test Strategy",
					AccessResourceTemplates: []workspacev1alpha1.AccessResourceTemplate{},
					AccessStartupProbe: &workspacev1alpha1.AccessStartupProbe{
						HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
							URLTemplate: "http://example.com/test",
						},
						PeriodSeconds:    2,
						FailureThreshold: 3,
					},
				},
			}
		})

		It("should mark Available when probe succeeds on first try", func() {
			mockProber.ready = true
			workspace := newWorkspaceWithAccessStrategy()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, accessStrategy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(Equal(PollRequeueDelay))

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionTrue))

			progressing := getCondition(workspace, ConditionTypeProgressing)
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should mark Available when probe succeeds after previous failures", func() {
			mockProber.ready = true
			workspace := newWorkspaceWithAccessStrategy()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			failures := int32(2)
			workspace.Status.AccessStartupProbeFailures = &failures
			workspace.Status.ObservedAccessStrategyVersion = fmt.Sprintf("%s.%d", accessStrategy.UID, accessStrategy.Generation)
			Expect(k8sClient.Status().Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, accessStrategy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(Equal(PollRequeueDelay))

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionTrue))

			progressing := getCondition(workspace, ConditionTypeProgressing)
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionFalse))

			degraded := getCondition(workspace, ConditionTypeDegraded)
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should report AccessNotReady and requeue when probe is retrying", func() {
			mockProber.ready = false
			workspace := newWorkspaceWithAccessStrategy()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			zero := int32(0)
			workspace.Status.AccessStartupProbeFailures = &zero
			workspace.Status.ObservedAccessStrategyVersion = fmt.Sprintf("%s.%d", accessStrategy.UID, accessStrategy.Generation)
			Expect(k8sClient.Status().Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, accessStrategy)
			Expect(err).NotTo(HaveOccurred())
			// failureThreshold=3 < ProbeBackoffThreshold, so backoff is active
			// from the start: first failure → periodSeconds*2 = 4s
			Expect(result.RequeueAfter).To(Equal(4 * time.Second))

			progressing := getCondition(workspace, ConditionTypeProgressing)
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionTrue))
			Expect(progressing.Reason).To(Equal(ReasonAccessNotReady))

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionFalse))

			degraded := getCondition(workspace, ConditionTypeDegraded)
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should report AccessNotReady and requeue when probe is pending retry", func() {
			workspace := newWorkspaceWithAccessStrategy()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			one := int32(1)
			future := metav1.NewTime(time.Now().Add(2 * time.Second))
			workspace.Status.AccessStartupProbeFailures = &one
			workspace.Status.EarliestNextProbeTime = &future
			workspace.Status.ObservedAccessStrategyVersion = fmt.Sprintf("%s.%d", accessStrategy.UID, accessStrategy.Generation)
			Expect(k8sClient.Status().Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, accessStrategy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))
			Expect(result.RequeueAfter).To(BeNumerically("<=", 2*time.Second))

			progressing := getCondition(workspace, ConditionTypeProgressing)
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionTrue))
			Expect(progressing.Reason).To(Equal(ReasonAccessNotReady))

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionFalse))

			// Failure counter should not have been incremented
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
			Expect(workspace.Status.AccessStartupProbeFailures).NotTo(BeNil())
			Expect(*workspace.Status.AccessStartupProbeFailures).To(Equal(int32(1)))
		})

		It("should mark Available when access resources are already ready", func() {
			workspace := newWorkspaceWithAccessStrategy()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			workspace.Status.AccessStartupProbeSucceeded = true
			workspace.Status.ObservedAccessStrategyVersion = fmt.Sprintf("%s.%d", accessStrategy.UID, accessStrategy.Generation)
			Expect(k8sClient.Status().Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, accessStrategy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).NotTo(Equal(PollRequeueDelay))

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionTrue))

			progressing := getCondition(workspace, ConditionTypeProgressing)
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionFalse))

			Expect(mockProber.probeCount).To(Equal(0))
		})

		It("should re-probe when AccessStrategy generation changes on an Available workspace", func() {
			mockProber.ready = true
			workspace := newWorkspaceWithAccessStrategy()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			workspace.Status.AccessStartupProbeSucceeded = true
			workspace.Status.ObservedAccessStrategyVersion = fmt.Sprintf("%s.%d", accessStrategy.UID, accessStrategy.Generation)
			Expect(k8sClient.Status().Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			// Simulate AccessStrategy spec update
			accessStrategy.Generation = 2
			mockProber.ready = false

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, accessStrategy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			progressing := getCondition(workspace, ConditionTypeProgressing)
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionTrue))
			Expect(progressing.Reason).To(Equal(ReasonAccessNotReady))

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionFalse))

			Expect(mockProber.probeCount).To(Equal(1))
		})

		It("should mark Degraded when failure threshold is exceeded", func() {
			mockProber.ready = false
			workspace := newWorkspaceWithAccessStrategy()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			failures := int32(2) // next failure (3) >= threshold (3)
			workspace.Status.AccessStartupProbeFailures = &failures
			workspace.Status.ObservedAccessStrategyVersion = fmt.Sprintf("%s.%d", accessStrategy.UID, accessStrategy.Generation)
			Expect(k8sClient.Status().Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, accessStrategy)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0)))

			degraded := getCondition(workspace, ConditionTypeDegraded)
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionTrue))
			Expect(degraded.Reason).To(Equal(ReasonAccessProbeThresholdExceeded))

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionFalse))
			Expect(available.Reason).To(Equal(ReasonAccessNotReady))

			progressing := getCondition(workspace, ConditionTypeProgressing)
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionFalse))
			Expect(progressing.Reason).To(Equal(ReasonAccessProbeThresholdExceeded))

			stopped := getCondition(workspace, ConditionTypeStopped)
			Expect(stopped).NotTo(BeNil())
			Expect(stopped.Status).To(Equal(metav1.ConditionFalse))
			Expect(stopped.Reason).To(Equal(ReasonDesiredStateRunning))
		})

		It("should propagate probe error", func() {
			mockProber.err = fmt.Errorf("template resolution failed")
			workspace := newWorkspaceWithAccessStrategy()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			zero := int32(0)
			workspace.Status.AccessStartupProbeFailures = &zero
			workspace.Status.ObservedAccessStrategyVersion = fmt.Sprintf("%s.%d", accessStrategy.UID, accessStrategy.Generation)
			Expect(k8sClient.Status().Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, accessStrategy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("template resolution failed"))
		})
	})

	Context("error paths", func() {
		var accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy

		newWorkspaceWithAccessStrategy := func() *workspacev1alpha1.Workspace {
			ws := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("sm-err-%d", time.Now().UnixNano()),
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					Image:         "jupyter/base-notebook:latest",
					DesiredStatus: DesiredStateRunning,
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name: "test-strategy",
					},
				},
			}
			Expect(k8sClient.Create(ctx, ws)).To(Succeed())
			return ws
		}

		makeWorkspaceStale := func(ws *workspacev1alpha1.Workspace) {
			fresh := ws.DeepCopy()
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(fresh), fresh)).To(Succeed())
			fresh.Status.AccessURL = staleUpdateValue
			Expect(k8sClient.Status().Update(ctx, fresh)).To(Succeed())
		}

		BeforeEach(func() {
			accessStrategy = &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-strategy",
					Namespace:  "default",
					UID:        types.UID("test-uid"),
					Generation: 1,
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName:             "Test Strategy",
					AccessResourceTemplates: []workspacev1alpha1.AccessResourceTemplate{},
					AccessStartupProbe: &workspacev1alpha1.AccessStartupProbe{
						HTTPGet: &workspacev1alpha1.AccessHTTPGetProbe{
							URLTemplate: "http://example.com/test",
						},
						PeriodSeconds:    2,
						FailureThreshold: 3,
					},
				},
			}
		})

		It("should propagate ReconcileAccessForDesiredRunningStatus error", func() {
			accessStrategy.Spec.AccessURLTemplate = "{{ .Invalid"
			workspace := newWorkspaceWithAccessStrategy()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, accessStrategy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse URL template"))
		})

		It("should propagate UpdatePermanentDegradedRunningStatus failure on threshold exceeded", func() {
			mockProber.ready = false
			workspace := newWorkspaceWithAccessStrategy()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			failures := int32(2)
			workspace.Status.AccessStartupProbeFailures = &failures
			workspace.Status.ObservedAccessStrategyVersion = fmt.Sprintf("%s.%d", accessStrategy.UID, accessStrategy.Generation)
			Expect(k8sClient.Status().Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			makeWorkspaceStale(workspace)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, accessStrategy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update Workspace.Status"))
		})

		It("should propagate UpdateRunningStatus failure", func() {
			mockProber.ready = true
			workspace := newWorkspaceWithAccessStrategy()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			makeWorkspaceStale(workspace)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, accessStrategy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update Workspace.Status"))
		})

		It("should propagate UpdateStartingStatus failure", func() {
			mockProber.ready = false
			workspace := newWorkspaceWithAccessStrategy()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			zero := int32(0)
			workspace.Status.AccessStartupProbeFailures = &zero
			workspace.Status.ObservedAccessStrategyVersion = fmt.Sprintf("%s.%d", accessStrategy.UID, accessStrategy.Generation)
			Expect(k8sClient.Status().Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			makeWorkspaceStale(workspace)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, accessStrategy)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update Workspace.Status"))
		})
	})
})
