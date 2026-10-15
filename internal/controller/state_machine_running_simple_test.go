/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"strings"
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

const newReplicaSetAvailableReason = "NewReplicaSetAvailable"
const replicaSetUpdatedReason = "ReplicaSetUpdated"

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
		rm.SetEventReader(k8sClient)
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
			Expect(result.RequeueAfter).To(Equal(NoPodRequeueDelay))

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
			Expect(result.RequeueAfter).To(Equal(NoPodRequeueDelay))

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

	Context("stalled rollout, failed starts and start steps", func() {
		const (
			deploymentStalledMessage = "ReplicaSet \"sm-simple\" has timed out progressing."
			backOffMessage           = "Back-off pulling image \"" + imageBaseNotebook + "\""
			imagePullMessage         = kubeletReasonImagePullBackOff + ": " + backOffMessage
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
		// pastDeadline moves the state machine's clock to where a pod created now has made no progress for the
		// whole progress deadline, the point at which the Deployment's verdict describes it.
		pastDeadline := func(sm *StateMachine) {
			later := time.Now().Add(defaultDeploymentProgressDeadline + time.Minute)
			sm.resourceManager.now = func() time.Time { return later }
		}
		markDeploymentProgressing := func(dep *appsv1.Deployment) {
			setDeploymentStatus(dep, appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1,
				Conditions: []appsv1.DeploymentCondition{{
					Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: replicaSetUpdatedReason,
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
		createPodEvent := func(pod *corev1.Pod, name, component, reason, message string, at time.Time) *corev1.Event {
			event := &corev1.Event{
				ObjectMeta:     metav1.ObjectMeta{Name: pod.Name + "-" + name, Namespace: pod.Namespace},
				InvolvedObject: corev1.ObjectReference{Kind: KindPod, Name: pod.Name, Namespace: pod.Namespace, UID: pod.UID},
				Reason:         reason,
				Message:        message,
				Source:         corev1.EventSource{Component: component},
				Type:           corev1.EventTypeNormal,
				FirstTimestamp: metav1.NewTime(at),
				LastTimestamp:  metav1.NewTime(at),
			}
			Expect(k8sClient.Create(ctx, event)).To(Succeed())
			return event
		}

		// createFailedPulls records the kubelet's Failed event of the main container's pull, repeated count times.
		createFailedPulls := func(pod *corev1.Pod, count int32) *corev1.Event {
			event := createPodEvent(pod, "pull-failed", kubeletComponent, kubeletEventFailed,
				"Failed to pull image \""+imageBaseNotebook+"\": not found", time.Now())
			event.Type = corev1.EventTypeWarning
			event.InvolvedObject.FieldPath = "spec.containers{" + containerNameMain + "}"
			event.Count = count
			Expect(k8sClient.Update(ctx, event)).To(Succeed())
			return event
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
		expectStartStep := func(ws *workspacev1alpha1.Workspace, reason, message string) {
			GinkgoHelper()
			progressing := getCondition(ws, ConditionTypeProgressing)
			Expect(progressing).NotTo(BeNil())
			Expect(progressing.Status).To(Equal(metav1.ConditionTrue))
			Expect(progressing.Reason).To(Equal(reason))
			Expect(progressing.Message).To(Equal(message))
			available := getCondition(ws, ConditionTypeAvailable)
			Expect(available.Status).To(Equal(metav1.ConditionFalse))
			Expect(available.Reason).To(Equal(reason))
			Expect(available.Message).To(Equal(message))
			expectNotDegraded(ws)
		}
		stallEvent := func(message string) OmegaMatcher {
			return Equal(fmt.Sprintf("%s %s %s", corev1.EventTypeWarning, EventWorkspaceComputeStalled, message))
		}

		It("should report Degraded=ComputeStalled with the scheduler's verdict once the deadline passes", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			pod := createPendingPod(workspace, unschedulable(stepTestSchedulingMessage))
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			pastDeadline(sm)
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(LongRequeueDelay))

			expectStalled(workspace, stepTestSchedulingMessage)
			Expect(workspace.Status.DeploymentName).To(Equal(dep.Name))
			Expect(workspace.Status.ServiceName).To(Equal(svc.Name))
			Expect(recorder.Events).To(HaveLen(1))
			Expect(recorder.Events).To(Receive(stallEvent(stepTestSchedulingMessage)))
		})

		It("should keep the kubelet's reason rather than ComputeStalled when a pull failure outlasts the deadline", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			pod := createPendingPod(workspace,
				waitingContainer(kubeletReasonImagePullBackOff, backOffMessage))
			failedPulls := createFailedPulls(pod, 2)
			defer func() { _ = k8sClient.Delete(ctx, failedPulls) }()
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(FailedStartRequeueDelay))

			for _, condType := range []string{ConditionTypeDegraded, ConditionTypeAvailable, ConditionTypeProgressing} {
				Expect(getCondition(workspace, condType).Reason).To(Equal(kubeletReasonImagePullBackOff), condType)
				Expect(getCondition(workspace, condType).Message).To(Equal(backOffMessage), condType)
			}
			Expect(getCondition(workspace, ConditionTypeDegraded).Status).To(Equal(metav1.ConditionTrue))
			Expect(recorder.Events).To(HaveLen(1))
			Expect(recorder.Events).To(Receive(Equal(fmt.Sprintf("%s %s %s", corev1.EventTypeWarning,
				EventWorkspaceStartFailed, imagePullMessage))))
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

		It("should bound the Deployment's message like every copied message", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			longMessage := strings.Repeat("x", 4*maxCopiedMessageBytes)
			setDeploymentStatus(dep, appsv1.DeploymentStatus{Replicas: 1, UpdatedReplicas: 1,
				Conditions: []appsv1.DeploymentCondition{{
					Type: appsv1.DeploymentProgressing, Status: corev1.ConditionFalse,
					Reason: deploymentTimedOutReason, Message: longMessage,
				}}})

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())

			expectStalled(workspace, truncateMessage(longMessage, maxCopiedMessageBytes))
			Expect(recorder.Events).To(Receive(stallEvent(truncateMessage(longMessage, maxCopiedMessageBytes))))
		})

		It("should ignore the verdict of a pod that is being deleted", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			pod := createPendingPod(workspace, unschedulable(stepTestSchedulingMessage))
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			// a finalizer keeps the deleted pod around with a deletionTimestamp, as a terminating pod is
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
			pod.Finalizers = []string{stepTestFinalizer}
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
			pod := createPendingPod(workspace, unschedulable(stepTestSchedulingMessage))
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			pastDeadline(sm)
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			expectStalled(workspace, stepTestSchedulingMessage)

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
				Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: newReplicaSetAvailableReason,
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
				Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: newReplicaSetAvailableReason,
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
			Expect(result.RequeueAfter).To(Equal(NoPodRequeueDelay))

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

		It("should keep PullingImage rather than ComputeStalled while a pull outlasts the deadline", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			pod := createPendingPod(workspace, waitingContainer(kubeletReasonContainerCreating, ""))
			pulling := createPodEvent(pod, "pulling", kubeletComponent, kubeletEventPulling,
				"Pulling image \"jupyter/base-notebook:latest\"", time.Now())
			defer func() { _ = k8sClient.Delete(ctx, pulling) }()
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			pastDeadline(sm)
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(PullingImageRequeueDelay))

			expectStartStep(workspace, ReasonPullingImage, "Pulling image \"jupyter/base-notebook:latest\"")
			Expect(recorder.Events).To(BeEmpty())
		})

		It("should report the step rather than ComputeStalled for a pod that progressed within the deadline", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			// the node arrived ten minutes into the start: the Deployment's verdict stands, the pod is creating
			pod := createPendingPod(workspace, waitingContainer(kubeletReasonContainerCreating, ""))
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(StartingContainerRequeueDelay))

			expectStartStep(workspace, ReasonStartingContainer, kubeletReasonContainerCreating)
			Expect(recorder.Events).To(BeEmpty())
		})

		It("should keep the step through a failed pod list while the Deployment's verdict stands", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			pod := createPendingPod(workspace, waitingContainer(kubeletReasonContainerCreating, ""))
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			expectStartStep(workspace, ReasonStartingContainer, kubeletReasonContainerCreating)

			sm.resourceManager.client = &MockClient{Client: k8sClient,
				listFunc: func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
					if _, ok := list.(*corev1.PodList); ok {
						return errInjected
					}
					return k8sClient.List(ctx, list, opts...)
				}}
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(StartingContainerRequeueDelay))
			expectStartStep(workspace, ReasonStartingContainer, kubeletReasonContainerCreating)
			Expect(recorder.Events).To(BeEmpty())
		})

		It("should fall back to ComputeStalled for a pull back-off whose attempts it cannot count", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			pod := createPendingPod(workspace, waitingContainer(kubeletReasonImagePullBackOff, backOffMessage))
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			pastDeadline(sm)
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(LongRequeueDelay))

			expectStalled(workspace, imagePullMessage)
			Expect(recorder.Events).To(Receive(stallEvent(imagePullMessage)))
		})

		It("should carry the kubelet's newest event in ComputeStalled for a pod with a node", func() {
			workspace := newWorkspace()
			dep := createNotReadyDeployment(workspace)
			svc := createService(workspace)
			pod := createPendingPod(workspace, waitingContainer(kubeletReasonContainerCreating, ""))
			mount := createPodEvent(pod, "mount", kubeletComponent, "FailedMount", stepTestMount, time.Now())
			defer func() { _ = k8sClient.Delete(ctx, mount) }()
			defer func() { _ = k8sClient.Delete(ctx, pod) }()
			defer func() { _ = k8sClient.Delete(ctx, dep) }()
			defer func() { _ = k8sClient.Delete(ctx, svc) }()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()
			markDeploymentStalled(dep)

			sm := buildStateMachine()
			pastDeadline(sm)
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())

			expectStalled(workspace, stepTestMount)
			Expect(recorder.Events).To(Receive(stallEvent(stepTestMount)))
		})

		Context("start steps", func() {
			It("should report WaitingForNode with the scheduler's verdict while the pod has no node", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, unschedulable(stepTestSchedulingMessage))
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(WaitingForNodeRequeueDelay))

				expectStartStep(workspace, ReasonWaitingForNode, stepTestSchedulingMessage)
				Expect(recorder.Events).To(BeEmpty())
			})

			It("should keep the autoscaler's nomination while the scheduler repeats an older verdict", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, unschedulable(stepTestSchedulingMessage))
				now := time.Now()
				nominated := createPodEvent(pod, "nominated", "karpenter", "Nominated", stepTestNominated, now.Add(-time.Minute))
				scheduler := createPodEvent(pod, "sched", "default-scheduler", eventReasonFailedScheduling, stepTestSchedulingMessage,
					now.Add(-2*time.Minute))
				scheduler.Count = 9
				scheduler.LastTimestamp = metav1.NewTime(now)
				Expect(k8sClient.Update(ctx, scheduler)).To(Succeed())
				defer func() { _ = k8sClient.Delete(ctx, nominated) }()
				defer func() { _ = k8sClient.Delete(ctx, scheduler) }()
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())

				expectStartStep(workspace, ReasonWaitingForNode, stepTestNominated)
			})

			It("should report PullingImage while the kubelet pulls the image", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonContainerCreating, ""))
				pulling := createPodEvent(pod, "pulling", kubeletComponent, kubeletEventPulling,
					"Pulling image \"jupyter/base-notebook:latest\"", time.Now())
				defer func() { _ = k8sClient.Delete(ctx, pulling) }()
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())

				expectStartStep(workspace, ReasonPullingImage, "Pulling image \"jupyter/base-notebook:latest\"")
			})

			It("should report StartingContainer with the waiting container's reason", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonContainerCreating, ""))
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())

				expectStartStep(workspace, ReasonStartingContainer, kubeletReasonContainerCreating)
			})

			It("should reuse the events read within the interval and refresh them after it", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, unschedulable(stepTestSchedulingMessage))
				now := time.Now()
				nominated := createPodEvent(pod, "nominated", "karpenter", "Nominated", stepTestNominated, now)
				defer func() { _ = k8sClient.Delete(ctx, nominated) }()
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				clock := now
				sm.resourceManager.now = func() time.Time { return clock }
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartStep(workspace, ReasonWaitingForNode, stepTestNominated)

				limits := createPodEvent(pod, "limits", "karpenter", eventReasonFailedScheduling, stepTestLimits,
					now.Add(time.Second))
				defer func() { _ = k8sClient.Delete(ctx, limits) }()
				clock = now.Add(startEventsInterval / 2)
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartStep(workspace, ReasonWaitingForNode, stepTestNominated)

				clock = now.Add(2 * startEventsInterval)
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartStep(workspace, ReasonWaitingForNode, stepTestLimits)
			})

		})

		Context("failed start", func() {
			const pullErrMessage = "rpc error: code = NotFound desc = failed to pull and unpack image"
			const missingSecretMessage = "secret \"notebook-token\" not found"
			failedEvent := func(reason, message string) OmegaMatcher {
				return Equal(fmt.Sprintf("%s %s %s: %s", corev1.EventTypeWarning, EventWorkspaceStartFailed, reason, message))
			}
			expectStartFailed := func(ws *workspacev1alpha1.Workspace, reason, message string) {
				GinkgoHelper()
				for _, condType := range []string{ConditionTypeDegraded, ConditionTypeAvailable, ConditionTypeProgressing} {
					cond := getCondition(ws, condType)
					Expect(cond).NotTo(BeNil(), condType)
					Expect(cond.Reason).To(Equal(reason), condType)
					Expect(cond.Message).To(Equal(message), condType)
				}
				Expect(getCondition(ws, ConditionTypeDegraded).Status).To(Equal(metav1.ConditionTrue))
				Expect(getCondition(ws, ConditionTypeAvailable).Status).To(Equal(metav1.ConditionFalse))
				Expect(getCondition(ws, ConditionTypeProgressing).Status).To(Equal(metav1.ConditionFalse))
			}

			It("should report the kubelet's reason at once when the container cannot start", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonCreateConfigError, missingSecretMessage))
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(FailedStartRequeueDelay))

				expectStartFailed(workspace, kubeletReasonCreateConfigError, missingSecretMessage)
				Expect(workspace.Status.DeploymentName).To(Equal(dep.Name))
				Expect(workspace.Status.ServiceName).To(Equal(svc.Name))
				Expect(recorder.Events).To(HaveLen(1))
				Expect(recorder.Events).To(Receive(failedEvent(kubeletReasonCreateConfigError, missingSecretMessage)))
			})

			It("should hold a pull back-off as the pull step until the kubelet has failed to pull twice", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonImagePullBackOff, backOffMessage))
				failedPulls := createFailedPulls(pod, 1)
				defer func() { _ = k8sClient.Delete(ctx, failedPulls) }()
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				now := time.Now()
				clock := now
				sm.resourceManager.now = func() time.Time { return clock }
				result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(PullingImageRequeueDelay))
				expectStartStep(workspace, ReasonPullingImage, imagePullMessage)
				Expect(recorder.Events).To(BeEmpty())

				// the kubelet's second failed pull raises its Failed event to a count of two
				failedPulls.Count = 2
				Expect(k8sClient.Update(ctx, failedPulls)).To(Succeed())
				clock = now.Add(2 * startEventsInterval)
				result, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(FailedStartRequeueDelay))
				expectStartFailed(workspace, kubeletReasonImagePullBackOff, backOffMessage)
				Expect(recorder.Events).To(Receive(failedEvent(kubeletReasonImagePullBackOff, backOffMessage)))

				// the event expires an hour later; the reported failure holds while the kubelet keeps backing off
				Expect(k8sClient.Delete(ctx, failedPulls)).To(Succeed())
				clock = now.Add(time.Hour)
				result, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(FailedStartRequeueDelay))
				expectStartFailed(workspace, kubeletReasonImagePullBackOff, backOffMessage)
				Expect(recorder.Events).To(BeEmpty())
			})

			It("should report FailedCreate with the ReplicaSet's message while the pod cannot be created", func() {
				const quotaMessage = `pods "sm-simple-abc" is forbidden: exceeded quota: gpu, ` +
					`requested: requests.nvidia.com/gpu=1, used: requests.nvidia.com/gpu=4, limited: requests.nvidia.com/gpu=4`
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				setDeploymentStatus(dep, appsv1.DeploymentStatus{Replicas: 0, UpdatedReplicas: 0,
					Conditions: []appsv1.DeploymentCondition{
						{Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: replicaSetUpdatedReason},
						{Type: appsv1.DeploymentReplicaFailure, Status: corev1.ConditionTrue,
							Reason: replicaSetReasonFailedCreate, Message: quotaMessage, LastTransitionTime: metav1.Now()},
					}})

				sm := buildStateMachine()
				now := time.Now()
				clock := now
				sm.resourceManager.now = func() time.Time { return clock }
				// a condition that just appeared can be an API server or admission blip the ReplicaSet controller
				// clears on its next retry; it is not reported yet
				result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(NoPodRequeueDelay))
				expectNotDegraded(workspace)
				Expect(recorder.Events).To(BeEmpty())

				clock = now.Add(failedCreateGrace)
				result, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(FailedStartRequeueDelay))
				expectStartFailed(workspace, replicaSetReasonFailedCreate, quotaMessage)
				Expect(recorder.Events).To(HaveLen(1))
				Expect(recorder.Events).To(Receive(failedEvent(replicaSetReasonFailedCreate, quotaMessage)))

				// the failure lasts: no second event
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartFailed(workspace, replicaSetReasonFailedCreate, quotaMessage)
				Expect(recorder.Events).To(BeEmpty())

				// quota freed: the ReplicaSet controller creates the pod and drops the condition
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonContainerCreating, ""))
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				markDeploymentProgressing(dep)
				result, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(StartingContainerRequeueDelay))
				expectStartStep(workspace, ReasonStartingContainer, kubeletReasonContainerCreating)
				Expect(recorder.Events).To(BeEmpty())
			})

			It("should hold a create error as a step until the kubelet has failed twice", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonCreateContainerError, stepTestCreateErrorText))
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				now := time.Now()
				clock := now
				sm.resourceManager.now = func() time.Time { return clock }
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartStep(workspace, ReasonStartingContainer, kubeletReasonCreateContainerError+": "+stepTestCreateErrorText)
				Expect(recorder.Events).To(BeEmpty())

				// the kubelet's second failed attempt raises its Failed event to a count of two
				failed := createPodEvent(pod, "failed", kubeletComponent, kubeletEventFailed, "Error: "+stepTestCreateErrorText, now)
				defer func() { _ = k8sClient.Delete(ctx, failed) }()
				failed.InvolvedObject.FieldPath = "spec.containers{" + containerNameMain + "}"
				failed.Count = 2
				Expect(k8sClient.Update(ctx, failed)).To(Succeed())
				clock = now.Add(2 * startEventsInterval)
				result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(FailedStartRequeueDelay))
				expectStartFailed(workspace, kubeletReasonCreateContainerError, stepTestCreateErrorText)
				Expect(recorder.Events).To(Receive(failedEvent(kubeletReasonCreateContainerError, stepTestCreateErrorText)))

				// the event expires an hour later; the reported failure holds while the kubelet keeps retrying
				Expect(k8sClient.Delete(ctx, failed)).To(Succeed())
				clock = now.Add(time.Hour)
				result, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(FailedStartRequeueDelay))
				expectStartFailed(workspace, kubeletReasonCreateContainerError, stepTestCreateErrorText)
				Expect(recorder.Events).To(BeEmpty())
			})

			It("should record the event only once the status write has succeeded", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonCreateConfigError, missingSecretMessage))
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)
				makeWorkspaceStale(workspace)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).To(MatchError(ContainSubstring("failed to update Workspace.Status")))
				Expect(recorder.Events).To(BeEmpty())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartFailed(workspace, kubeletReasonCreateConfigError, missingSecretMessage)
				Expect(recorder.Events).To(HaveLen(1))
				Expect(recorder.Events).To(Receive(failedEvent(kubeletReasonCreateConfigError, missingSecretMessage)))
			})

			It("should bound the event message like the condition message", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				longMessage := strings.Repeat("x", 4*maxCopiedMessageBytes)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonCreateConfigError, longMessage))
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())

				conditionMessage := truncateMessage(longMessage, maxCopiedMessageBytes)
				expectStartFailed(workspace, kubeletReasonCreateConfigError, conditionMessage)
				Expect(recorder.Events).To(Receive(Equal(fmt.Sprintf("%s %s %s", corev1.EventTypeWarning, EventWorkspaceStartFailed,
					truncateMessage(kubeletReasonCreateConfigError+": "+conditionMessage, maxCopiedMessageBytes)))))
			})

			It("should report a failed pull attempt as a step and turn Degraded once the kubelet backs off", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonErrImagePull, pullErrMessage))
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				now := time.Now()
				clock := now
				sm.resourceManager.now = func() time.Time { return clock }
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartStep(workspace, ReasonPullingImage, kubeletReasonErrImagePull+": "+pullErrMessage)
				Expect(recorder.Events).To(BeEmpty())

				// the second pull fails as well and the kubelet backs off
				failedPulls := createFailedPulls(pod, 2)
				defer func() { _ = k8sClient.Delete(ctx, failedPulls) }()
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
				pod.Status = waitingContainer(kubeletReasonImagePullBackOff, backOffMessage)
				pod.Status.Phase = corev1.PodPending
				Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
				clock = now.Add(2 * startEventsInterval)
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())

				expectStartFailed(workspace, kubeletReasonImagePullBackOff, backOffMessage)
				Expect(recorder.Events).To(HaveLen(1))
				Expect(recorder.Events).To(Receive(failedEvent(kubeletReasonImagePullBackOff, backOffMessage)))

				// the kubelet's next pull attempt shows ErrImagePull again
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
				pod.Status = waitingContainer(kubeletReasonErrImagePull, pullErrMessage)
				pod.Status.Phase = corev1.PodPending
				Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartFailed(workspace, kubeletReasonImagePullBackOff, backOffMessage)
				Expect(recorder.Events).To(BeEmpty())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
				pod.Status = waitingContainer(kubeletReasonImagePullBackOff, backOffMessage)
				pod.Status.Phase = corev1.PodPending
				Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartFailed(workspace, kubeletReasonImagePullBackOff, backOffMessage)
				Expect(recorder.Events).To(BeEmpty())
			})

			It("should report an init container that crash-loops", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				status := waitingContainer("PodInitializing", "")
				status.InitContainerStatuses = []corev1.ContainerStatus{{Name: stepTestInitName, RestartCount: 4,
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
						Reason: kubeletReasonCrashLoopBackOff, Message: "back-off 10s restarting failed container=" + stepTestInitName,
					}},
					LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
						ExitCode: 2, Reason: terminatedReasonError, FinishedAt: metav1.Now(),
					}},
				}}
				pod := createPendingPod(workspace, status)
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())

				expectStartFailed(workspace, kubeletReasonCrashLoopBackOff,
					"container "+stepTestInitName+" exited with code 2 (Error), restart count 4")
			})

			It("should record the event when a start failure follows an operator error", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonImagePullBackOff, backOffMessage))
				failedPulls := createFailedPulls(pod, 2)
				defer func() { _ = k8sClient.Delete(ctx, failedPulls) }()
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				Expect(sm.statusManager.UpdateErrorStatus(ctx, workspace, ReasonDeploymentError, "api error", nil)).To(Succeed())
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())

				expectStartFailed(workspace, kubeletReasonImagePullBackOff, backOffMessage)
				Expect(recorder.Events).To(Receive(failedEvent(kubeletReasonImagePullBackOff, backOffMessage)))
			})

			restartedRunning := func(restartCount int32, finishedAt time.Time) corev1.PodStatus {
				return corev1.PodStatus{
					Conditions: []corev1.PodCondition{{Type: corev1.PodScheduled, Status: corev1.ConditionTrue}},
					ContainerStatuses: []corev1.ContainerStatus{{Name: containerNameMain, RestartCount: restartCount,
						State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
						LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{
							ExitCode: 1, Reason: terminatedReasonError,
							StartedAt: metav1.NewTime(finishedAt.Add(-5 * time.Second)), FinishedAt: metav1.NewTime(finishedAt),
						}},
					}},
				}
			}
			recordedExit := func(restartCount int) string {
				return fmt.Sprintf("container %s exited with code 1 (Error), restart count %d", containerNameMain, restartCount)
			}

			It("should carry the exit code and restart count of a crash loop and refresh the count without a second event", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, restartedRunning(2, time.Now()))
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartFailed(workspace, kubeletReasonCrashLoopBackOff, recordedExit(2))
				Expect(recorder.Events).To(Receive(failedEvent(kubeletReasonCrashLoopBackOff, recordedExit(2))))

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
				pod.Status = restartedRunning(2, time.Now())
				pod.Status.ContainerStatuses[0].State = corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{
					Reason: kubeletReasonCrashLoopBackOff, Message: "back-off 10s restarting failed container=" + containerNameMain,
				}}
				pod.Status.Phase = corev1.PodRunning
				Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartFailed(workspace, kubeletReasonCrashLoopBackOff, recordedExit(2))
				Expect(recorder.Events).To(BeEmpty())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
				pod.Status = restartedRunning(3, time.Now())
				pod.Status.Phase = corev1.PodRunning
				Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartFailed(workspace, kubeletReasonCrashLoopBackOff, recordedExit(3))
				Expect(recorder.Events).To(BeEmpty())
			})

			It("should record a new event when a recovered workspace fails to start again", func() {
				workspace := newWorkspace()
				dep := createBuiltDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonImagePullBackOff, backOffMessage))
				failedPulls := createFailedPulls(pod, 2)
				defer func() { _ = k8sClient.Delete(ctx, failedPulls) }()
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(recorder.Events).To(Receive(failedEvent(kubeletReasonImagePullBackOff, backOffMessage)))

				markDeploymentAvailable(dep, appsv1.DeploymentCondition{
					Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: newReplicaSetAvailableReason,
				})
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(getCondition(workspace, ConditionTypeAvailable).Status).To(Equal(metav1.ConditionTrue))
				expectNotDegraded(workspace)
				Expect(recorder.Events).To(Receive(ContainSubstring("WorkspaceRunning")))

				// a new rollout whose pod fails the same way
				markDeploymentProgressing(dep)
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())

				expectStartFailed(workspace, kubeletReasonImagePullBackOff, backOffMessage)
				Expect(recorder.Events).To(Receive(failedEvent(kubeletReasonImagePullBackOff, backOffMessage)))
			})

			It("should return to the start step as soon as the kubelet stops reporting the failure", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonImagePullBackOff, backOffMessage))
				failedPulls := createFailedPulls(pod, 2)
				defer func() { _ = k8sClient.Delete(ctx, failedPulls) }()
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				now := time.Now()
				clock := now
				sm.resourceManager.now = func() time.Time { return clock }
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartFailed(workspace, kubeletReasonImagePullBackOff, backOffMessage)
				Expect(recorder.Events).To(Receive())

				// the image was pushed: the kubelet pulls it and creates the container
				pulledMessage := "Successfully pulled image \"" + imageBaseNotebook + "\""
				pulled := createPodEvent(pod, "pulled", kubeletComponent, "Pulled", pulledMessage, now.Add(time.Minute))
				defer func() { _ = k8sClient.Delete(ctx, pulled) }()
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), pod)).To(Succeed())
				pod.Status = waitingContainer(kubeletReasonContainerCreating, "")
				pod.Status.Phase = corev1.PodPending
				Expect(k8sClient.Status().Update(ctx, pod)).To(Succeed())
				clock = now.Add(2 * startEventsInterval)
				result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(StartingContainerRequeueDelay))

				expectStartStep(workspace, ReasonStartingContainer, pulledMessage)
				Expect(recorder.Events).To(BeEmpty())
			})

			It("should not take old restarts for a crash loop when the container turns unready later", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, restartedRunning(2, time.Now().Add(-time.Hour)))
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())

				expectStartStep(workspace, ReasonStartingContainer, startingContainerMessage)
				Expect(recorder.Events).To(BeEmpty())
			})

			It("should stay out of Degraded when the container of an Available workspace exits once and comes back", func() {
				workspace := newWorkspace()
				dep := createBuiltDeployment(workspace)
				svc := createService(workspace)
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				progressed := appsv1.DeploymentCondition{
					Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: newReplicaSetAvailableReason,
				}
				markDeploymentAvailable(dep, progressed)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(getCondition(workspace, ConditionTypeAvailable).Status).To(Equal(metav1.ConditionTrue))

				pod := createPendingPod(workspace, restartedRunning(1, time.Now()))
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				markDeploymentProgressing(dep)
				result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(StartingContainerRequeueDelay))
				expectStartStep(workspace, ReasonStartingContainer, startingContainerMessage)

				markDeploymentAvailable(dep, progressed)
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(getCondition(workspace, ConditionTypeAvailable).Status).To(Equal(metav1.ConditionTrue))
				expectNotDegraded(workspace)
				Expect(recorder.Events).To(HaveLen(2))
				for len(recorder.Events) > 0 {
					Expect(<-recorder.Events).NotTo(ContainSubstring(EventWorkspaceStartFailed))
				}
			})

			It("should report a crash loop of an Available workspace once its container exits again", func() {
				workspace := newWorkspace()
				dep := createBuiltDeployment(workspace)
				svc := createService(workspace)
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				progressed := appsv1.DeploymentCondition{
					Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: newReplicaSetAvailableReason,
				}
				markDeploymentAvailable(dep, progressed)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(getCondition(workspace, ConditionTypeAvailable).Status).To(Equal(metav1.ConditionTrue))
				for len(recorder.Events) > 0 {
					<-recorder.Events
				}

				pod := createPendingPod(workspace, restartedRunning(2, time.Now()))
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				markDeploymentProgressing(dep)
				result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(FailedStartRequeueDelay))
				expectStartFailed(workspace, kubeletReasonCrashLoopBackOff, recordedExit(2))
				Expect(recorder.Events).To(Receive(failedEvent(kubeletReasonCrashLoopBackOff, recordedExit(2))))
				Expect(recorder.Events).To(BeEmpty())
			})

			It("should report Deleting and clear Degraded when a failed workspace is deleted", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonImagePullBackOff, backOffMessage))
				failedPulls := createFailedPulls(pod, 2)
				defer func() { _ = k8sClient.Delete(ctx, failedPulls) }()
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() {
					if k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace) == nil {
						workspace.Finalizers = nil
						_ = k8sClient.Update(ctx, workspace)
					}
				}()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartFailed(workspace, kubeletReasonImagePullBackOff, backOffMessage)

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
				workspace.Finalizers = []string{WorkspaceFinalizerName}
				Expect(k8sClient.Update(ctx, workspace)).To(Succeed())
				Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
				_, err = sm.ReconcileDeletion(ctx, workspace)
				Expect(err).NotTo(HaveOccurred())

				// the cleanup finds nothing left to delete and removes the finalizer, so the object itself is gone
				// by now; the conditions it carried are on the object the state machine wrote
				Expect(findCondition(workspace.Status.Conditions, ConditionTypeDeleting).Status).To(Equal(metav1.ConditionTrue))
				Expect(findCondition(workspace.Status.Conditions, ConditionTypeAvailable).Reason).To(Equal(ReasonDeletionInProgress))
				Expect(findCondition(workspace.Status.Conditions, ConditionTypeDegraded).Status).To(Equal(metav1.ConditionFalse))
			})

			It("should clear Degraded and report Available once the container runs", func() {
				workspace := newWorkspace()
				dep := createNotReadyDeployment(workspace)
				svc := createService(workspace)
				pod := createPendingPod(workspace, waitingContainer(kubeletReasonImagePullBackOff, backOffMessage))
				failedPulls := createFailedPulls(pod, 2)
				defer func() { _ = k8sClient.Delete(ctx, failedPulls) }()
				defer func() { _ = k8sClient.Delete(ctx, pod) }()
				defer func() { _ = k8sClient.Delete(ctx, dep) }()
				defer func() { _ = k8sClient.Delete(ctx, svc) }()
				defer func() { _ = k8sClient.Delete(ctx, workspace) }()
				markDeploymentProgressing(dep)

				sm := buildStateMachine()
				_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())
				expectStartFailed(workspace, kubeletReasonImagePullBackOff, backOffMessage)

				markDeploymentAvailable(dep, appsv1.DeploymentCondition{
					Type: appsv1.DeploymentProgressing, Status: corev1.ConditionTrue, Reason: newReplicaSetAvailableReason,
				})
				_, err = sm.ReconcileDesiredState(ctx, workspace, nil)
				Expect(err).NotTo(HaveOccurred())

				Expect(getCondition(workspace, ConditionTypeAvailable).Status).To(Equal(metav1.ConditionTrue))
				expectNotDegraded(workspace)
			})
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
