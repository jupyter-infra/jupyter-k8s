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

var _ = Describe("reconcileDesiredRunningStatus without access strategy", func() {
	var (
		ctx        context.Context
		mockProber *mockAccessStartupProber
		recorder   *record.FakeRecorder
	)

	newWorkspace := func() *workspacev1alpha1.Workspace {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("sm-simple-%d", time.Now().UnixNano()),
				Namespace: testNamespace,
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Image:         imageBaseNotebook,
				DesiredStatus: DesiredStateRunning,
			},
		}
		Expect(k8sClient.Create(ctx, ws)).To(Succeed())
		return ws
	}

	createNotReadyDeployment := func(ws *workspacev1alpha1.Workspace) *appsv1.Deployment {
		replicas := int32(1)
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateDeploymentName(ws.Name),
				Namespace: ws.Namespace,
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{AppLabel: literalTest},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{AppLabel: literalTest}},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name:  containerNameMain,
							Image: imageBaseNotebook,
						}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, dep)).To(Succeed())
		return dep
	}

	createReadyDeployment := func(ws *workspacev1alpha1.Workspace) *appsv1.Deployment {
		dep := createNotReadyDeployment(ws)
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
				Selector: map[string]string{AppLabel: literalTest},
			},
		}
		Expect(k8sClient.Create(ctx, svc)).To(Succeed())
		return svc
	}

	createLoadBalancerServiceWithoutIngress := func(ws *workspacev1alpha1.Workspace) *corev1.Service {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GenerateServiceName(ws.Name),
				Namespace: ws.Namespace,
			},
			Spec: corev1.ServiceSpec{
				Type: corev1.ServiceTypeLoadBalancer,
				Ports: []corev1.ServicePort{{
					Port:       8888,
					TargetPort: intstr.FromInt32(8888),
				}},
				Selector: map[string]string{AppLabel: literalTest},
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
			NewDeploymentBuilder(scheme.Scheme, WorkspaceControllerOptions{}),
			NewServiceBuilder(scheme.Scheme),
			NewPVCBuilder(scheme.Scheme),
			NewAccessResourcesBuilder(),
			statusManager,
		)
		return &StateMachine{
			resourceManager:     rm,
			statusManager:       statusManager,
			accessStartupProber: mockProber,
			recorder:            recorder,
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

	makeWorkspaceStale := func(ws *workspacev1alpha1.Workspace) {
		fresh := ws.DeepCopy()
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(fresh), fresh)).To(Succeed())
		fresh.Status.AccessURL = staleUpdateValue
		Expect(k8sClient.Status().Update(ctx, fresh)).To(Succeed())
	}

	BeforeEach(func() {
		ctx = context.Background()
		mockProber = &mockAccessStartupProber{}
		recorder = record.NewFakeRecorder(10)
	})

	Context("readiness combinations", func() {
		It("should report ResourcesNotReady when deployment and service are not ready", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createLoadBalancerServiceWithoutIngress(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(PollRequeueDelay))

			progressing := getCondition(workspace, ConditionTypeProgressing)
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionTrue))
			Expect(progressing.Reason).To(Equal(ReasonResourcesNotReady))

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionFalse))
			Expect(available.Reason).To(Equal(ReasonResourcesNotReady))
		})

		It("should report ResourcesNotReady when service ready but deployment not ready", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(PollRequeueDelay))

			progressing := getCondition(workspace, ConditionTypeProgressing)
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionTrue))

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should mark Available when deployment and service are ready", func() {
			workspace := newWorkspace()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0)))

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionTrue))
			Expect(available.Reason).To(Equal(ReasonResourcesReady))

			progressing := getCondition(workspace, ConditionTypeProgressing)
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionFalse))
		})
	})

	Context("stalled rollout", func() {
		const (
			deploymentStalledMessage = "ReplicaSet \"sm-simple\" has timed out progressing."
			schedulingMessage        = "0/1 nodes are available: 1 Insufficient nvidia.com/gpu."
			imagePullMessage         = "ImagePullBackOff: Back-off pulling image \"example.com/missing:1\""
		)

		setDeploymentStatus := func(dep *appsv1.Deployment, status appsv1.DeploymentStatus) {
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(dep), dep)).To(Succeed())
			dep.Status = status
			Expect(k8sClient.Status().Update(ctx, dep)).To(Succeed())
		}
		markDeploymentStalled := func(dep *appsv1.Deployment) {
			setDeploymentStatus(dep, appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1,
				Conditions: []appsv1.DeploymentCondition{{
					Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse,
					Reason: deploymentTimedOutReason, Message: deploymentStalledMessage,
				}}})
		}
		markDeploymentProgressing := func(dep *appsv1.Deployment) {
			setDeploymentStatus(dep, appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1,
				Conditions: []appsv1.DeploymentCondition{{
					Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "ReplicaSetUpdated",
				}}})
		}
		markDeploymentAvailable := func(dep *appsv1.Deployment, progressing appsv1.DeploymentCondition) {
			setDeploymentStatus(dep, appsv1.DeploymentStatus{
				Replicas: 1, UpdatedReplicas: 1, ReadyReplicas: 1, AvailableReplicas: 1,
				Conditions: []appsv1.DeploymentCondition{
					{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
					progressing,
				}})
		}

		// createBuiltDeployment persists the Deployment the operator itself builds for the workspace, so a
		// reconcile of an Available workspace finds no drift to repair; createNotReadyDeployment's fixture
		// template would be replaced, and its differing selector makes that update invalid.
		createBuiltDeployment := func(ws *workspacev1alpha1.Workspace) *appsv1.Deployment {
			dep, err := NewDeploymentBuilder(scheme.Scheme, WorkspaceControllerOptions{}).
				BuildWorkspaceDeployment(ctx, ws, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Create(ctx, dep)).To(Succeed())
			return dep
		}

		// createPendingPod creates the workspace pod and applies the given status to it.
		createPendingPod := func(ws *workspacev1alpha1.Workspace, status corev1.PodStatus) *corev1.Pod {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-pod", ws.Name),
					Namespace: ws.Namespace,
					Labels:    GenerateLabels(ws.Name),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: containerNameMain, Image: imageBaseNotebook}},
				},
			}
			Expect(k8sClient.Create(ctx, pod)).To(Succeed())
			status.Phase = corev1.PodPending
			pod.Status = status
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
			return pod
		}
		unschedulable := func(message string) corev1.PodStatus {
			return corev1.PodStatus{Conditions: []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionFalse,
				Reason: corev1.PodReasonUnschedulable, Message: message,
			}}}
		}
		waitingContainer := func(reason, message string) corev1.PodStatus {
			return corev1.PodStatus{
				Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}},
				ContainerStatuses: []corev1.ContainerStatus{{Name: containerNameMain, State: corev1.ContainerState{
					Waiting: &corev1.ContainerStateWaiting{Reason: reason, Message: message},
				}}},
			}
		}

		expectStalled := func(ws *workspacev1alpha1.Workspace, message string) {
			GinkgoHelper()
			for _, condType := range []string{ConditionTypeDegraded, ConditionTypeAvailable, ConditionTypeProgressing} {
				cond := getCondition(ws, condType)
				Expect(cond).NotTo(BeNil(), condType)
				Expect(cond.Reason).To(Equal(ReasonComputeStalled), condType)
				Expect(cond.Message).To(Equal(message), condType)
			}
			Expect(getCondition(ws, ConditionTypeDegraded).Status).To(Equal(metav1.ConditionTrue))
			Expect(getCondition(ws, ConditionTypeAvailable).Status).To(Equal(metav1.ConditionFalse))
			Expect(getCondition(ws, ConditionTypeProgressing).Status).To(Equal(metav1.ConditionFalse))
			Expect(getCondition(ws, ConditionTypeStopped).Status).To(Equal(metav1.ConditionFalse))
			Expect(getCondition(ws, ConditionTypeDeleting).Status).To(Equal(metav1.ConditionFalse))
		}
		expectNotDegraded := func(ws *workspacev1alpha1.Workspace) {
			GinkgoHelper()
			degraded := getCondition(ws, ConditionTypeDegraded)
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionFalse))
			Expect(degraded.Reason).To(Equal(ReasonNoError))
		}
		stallEvent := func(message string) OmegaMatcher {
			return Equal(fmt.Sprintf("%s %s %s", corev1.EventTypeWarning, EventWorkspaceComputeStalled, message))
		}

		It("should report Degraded=ComputeStalled with the scheduler's verdict once the deadline passes", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			pod := createPendingPod(workspace, unschedulable(schedulingMessage))
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(LongRequeueDelay))

			expectStalled(workspace, schedulingMessage)
			Expect(workspace.Status.DeploymentName).To(Equal(dep.Name))
			Expect(workspace.Status.ServiceName).To(Equal(svc.Name))
			Expect(recorder.Events).To(HaveLen(1))
			Expect(recorder.Events).To(Receive(stallEvent(schedulingMessage)))
		})

		It("should report the waiting container's reason when the pod is scheduled but cannot start", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			pod := createPendingPod(workspace,
				waitingContainer("ImagePullBackOff", "Back-off pulling image \"example.com/missing:1\""))
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())

			expectStalled(workspace, imagePullMessage)
			Expect(recorder.Events).To(Receive(stallEvent(imagePullMessage)))
		})

		It("should fall back to the Deployment's message when no pod reports anything", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())

			expectStalled(workspace, deploymentStalledMessage)
			Expect(recorder.Events).To(Receive(stallEvent(deploymentStalledMessage)))
		})

		It("should ignore the verdict of a pod that is being deleted", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			pod := createPendingPod(workspace, unschedulable(schedulingMessage))
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			// a finalizer keeps the deleted pod around with a deletionTimestamp, as a terminating pod is
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
			pod.Finalizers = []string{"test.jupyter.org/keep"}
			Expect(k8sClient.Update(ctx, pod)).To(Succeed())
			Expect(k8sClient.Delete(ctx, pod)).To(Succeed())
			defer func() {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)
				pod.Finalizers = nil
				_ = k8sClient.Update(ctx, pod)
			}()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())

			expectStalled(workspace, deploymentStalledMessage)
		})

		It("should refresh the message without repeating the event while the stall lasts", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			pod := createPendingPod(workspace, unschedulable(schedulingMessage))
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			expectStalled(workspace, schedulingMessage)

			newMessage := "0/2 nodes are available: 2 Insufficient nvidia.com/gpu."
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
			pod.Status = unschedulable(newMessage)
			Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())

			// getCondition refreshed the workspace, so the second reconcile sees Degraded=True as the real one would
			_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			expectStalled(workspace, newMessage)
			Expect(recorder.Events).To(HaveLen(1))
		})

		It("should clear Degraded and report Available once the pod becomes ready", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			expectStalled(workspace, deploymentStalledMessage)

			markDeploymentAvailable(dep, appsv1.DeploymentCondition{
				Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable",
			})
			_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available.Status).To(Equal(metav1.ConditionTrue))
			Expect(available.Reason).To(Equal(ReasonResourcesReady))
			expectNotDegraded(workspace)
		})

		It("should report a new stall when a later rollout of a recovered workspace stalls", func() {
			workspace := newWorkspace()
			dep := createBuiltDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			sm := buildStateMachine()

			markDeploymentStalled(dep)
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			expectStalled(workspace, deploymentStalledMessage)

			markDeploymentAvailable(dep, appsv1.DeploymentCondition{
				Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: "NewReplicaSetAvailable",
			})
			_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(getCondition(workspace, ConditionTypeAvailable).Status).To(Equal(metav1.ConditionTrue))
			expectNotDegraded(workspace)

			// a new rollout: the Deployment is progressing again and no longer available
			markDeploymentProgressing(dep)
			_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(getCondition(workspace, ConditionTypeProgressing).Status).To(Equal(metav1.ConditionTrue))
			expectNotDegraded(workspace)

			markDeploymentStalled(dep)
			_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			expectStalled(workspace, deploymentStalledMessage)

			Expect(recorder.Events).To(Receive(stallEvent(deploymentStalledMessage)))
			Expect(recorder.Events).To(Receive(ContainSubstring("WorkspaceRunning")))
			Expect(recorder.Events).To(Receive(stallEvent(deploymentStalledMessage)))
			Expect(recorder.Events).To(BeEmpty())
		})

		It("should clear Degraded and report Starting when the rollout resumes before the pod is ready", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			expectStalled(workspace, deploymentStalledMessage)

			markDeploymentProgressing(dep)
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(PollRequeueDelay))

			progressing := getCondition(workspace, ConditionTypeProgressing)
			Expect(progressing.Status).To(Equal(metav1.ConditionTrue))
			Expect(getCondition(workspace, ConditionTypeAvailable).Status).To(Equal(metav1.ConditionFalse))
			expectNotDegraded(workspace)
		})

		It("should let the ready path win over a stale deadline condition", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentAvailable(dep, appsv1.DeploymentCondition{
				Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse,
				Reason: deploymentTimedOutReason, Message: deploymentStalledMessage,
			})

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(getCondition(workspace, ConditionTypeAvailable).Status).To(Equal(metav1.ConditionTrue))
			expectNotDegraded(workspace)
			Expect(recorder.Events).NotTo(Receive(ContainSubstring(EventWorkspaceComputeStalled)))
		})

		It("should not stay Degraded once the workspace is stopped", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			expectStalled(workspace, deploymentStalledMessage)

			// the stop path has already removed the compute; the workspace spec now asks for Stopped
			Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
			Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
			workspace.Spec.DesiredStatus = DesiredStateStopped
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(getCondition(workspace, ConditionTypeStopped).Status).To(Equal(metav1.ConditionTrue))
			expectNotDegraded(workspace)
		})

		It("should propagate a status update failure", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)
			makeWorkspaceStale(workspace)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update Workspace.Status"))
		})
	})

	Context("ensure resource errors", func() {
		It("should propagate EnsureDeploymentExists error", func() {
			// Workspace not persisted to etcd — has no UID, causing SetControllerReference to fail
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("sm-simple-%d", time.Now().UnixNano()),
					Namespace: testNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					Image:         imageBaseNotebook,
					DesiredStatus: DesiredStateRunning,
				},
			}

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to ensure deployment exists"))
		})

		It("should propagate EnsureServiceExists error", func() {
			// Workspace not persisted to etcd — has no UID
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("sm-simple-%d", time.Now().UnixNano()),
					Namespace: testNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					Image:         imageBaseNotebook,
					DesiredStatus: DesiredStateRunning,
				},
			}
			// Pre-create deployment so EnsureDeploymentExists succeeds (finds existing)
			dep := createNotReadyDeployment(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to ensure service exists"))
		})
	})

	Context("status update errors", func() {
		It("should propagate UpdateRunningStatus error", func() {
			workspace := newWorkspace()
			dep := createReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			makeWorkspaceStale(workspace)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update Workspace.Status"))
		})

		It("should propagate UpdateStartingStatus error", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			makeWorkspaceStale(workspace)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update Workspace.Status"))
		})
	})
})
