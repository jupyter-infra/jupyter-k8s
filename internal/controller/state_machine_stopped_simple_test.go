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

var _ = Describe("reconcileDesiredStoppedStatus without access strategy", func() {
	var ctx context.Context

	const testFinalizer = "test.jupyter.org/block-deletion"

	newStoppedWorkspace := func() *workspacev1alpha1.Workspace {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("sm-stop-%d", time.Now().UnixNano()),
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Image:         "jupyter/base-notebook:latest",
				DesiredStatus: DesiredStateStopped,
			},
		}
		Expect(k8sClient.Create(ctx, ws)).To(Succeed())
		return ws
	}

	// createLiveDeployment creates a deployment without finalizers.
	// EnsureDeploymentDeleted will delete it, but the returned object (pre-delete copy)
	// has no DeletionTimestamp, so IsDeploymentMissingOrDeleting returns false.
	createLiveDeployment := func(ws *workspacev1alpha1.Workspace) *appsv1.Deployment {
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
		return dep
	}

	createLiveService := func(ws *workspacev1alpha1.Workspace) *corev1.Service {
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

	// createDeletingDeployment creates a deployment with a finalizer, then deletes it
	// so it stays in "Deleting" state (DeletionTimestamp set, object still exists).
	createDeletingDeployment := func(ws *workspacev1alpha1.Workspace) *appsv1.Deployment {
		replicas := int32(1)
		dep := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:       GenerateDeploymentName(ws.Name),
				Namespace:  ws.Namespace,
				Finalizers: []string{testFinalizer},
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
		Expect(k8sClient.Delete(ctx, dep)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(dep), dep)).To(Succeed())
		Expect(dep.DeletionTimestamp.IsZero()).To(BeFalse())
		return dep
	}

	removeFinalizer := func(obj client.Object) {
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)).To(Succeed())
		obj.SetFinalizers(nil)
		Expect(k8sClient.Update(ctx, obj)).To(Succeed())
	}

	createDeletingService := func(ws *workspacev1alpha1.Workspace) *corev1.Service {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:       GenerateServiceName(ws.Name),
				Namespace:  ws.Namespace,
				Finalizers: []string{testFinalizer},
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
		Expect(k8sClient.Delete(ctx, svc)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(svc), svc)).To(Succeed())
		Expect(svc.DeletionTimestamp.IsZero()).To(BeFalse())
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
	})

	Context("deletion readiness", func() {
		It("should return error when deployment and service both just deleted", func() {
			workspace := newStoppedWorkspace()
			createLiveDeployment(workspace)
			createLiveService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("unexpected state"))
		})

		It("should mark Stopped when deployment and service are already gone", func() {
			workspace := newStoppedWorkspace()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Duration(0)))

			stopped := getCondition(workspace, ConditionTypeStopped)
			Expect(stopped).NotTo(BeNil())
			Expect(stopped.Status).To(Equal(metav1.ConditionTrue))
			Expect(stopped.Reason).To(Equal(ReasonResourcesStopped))

			available := getCondition(workspace, ConditionTypeAvailable)
			Expect(available).NotTo(BeNil())
			Expect(available.Status).To(Equal(metav1.ConditionFalse))
		})

		It("should report Stopping when service gone but deployment just deleted", func() {
			workspace := newStoppedWorkspace()
			createLiveDeployment(workspace)
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(PollRequeueDelay))

			stopped := getCondition(workspace, ConditionTypeStopped)
			Expect(stopped).NotTo(BeNil())
			Expect(stopped.Status).To(Equal(metav1.ConditionFalse))
			Expect(stopped.Reason).To(Equal(ReasonComputeNotStopped))
		})

		It("should report Stopping when deployment gone but service just deleted", func() {
			workspace := newStoppedWorkspace()
			createLiveService(workspace)
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			sm := buildStateMachine()
			result, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(PollRequeueDelay))

			stopped := getCondition(workspace, ConditionTypeStopped)
			Expect(stopped).NotTo(BeNil())
			Expect(stopped.Status).To(Equal(metav1.ConditionFalse))
			Expect(stopped.Reason).To(Equal(ReasonServiceNotStopped))
		})
	})

	Context("ensure resource deletion errors", func() {
		It("should propagate EnsureDeploymentDeleted error", func() {
			workspace := newStoppedWorkspace()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			// Create a deployment with a finalizer so it persists after delete
			dep := createDeletingDeployment(workspace)
			defer func() { removeFinalizer(dep) }()

			sm := buildStateMachine()
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()

			_, err := sm.ReconcileDesiredState(cancelCtx, workspace, nil)
			Expect(err).To(HaveOccurred())
		})

		It("should propagate EnsureServiceDeleted error", func() {
			workspace := newStoppedWorkspace()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			// Create a service with a finalizer so it persists after delete
			svc := createDeletingService(workspace)
			defer func() { removeFinalizer(svc) }()

			sm := buildStateMachine()
			cancelCtx, cancel := context.WithCancel(ctx)
			cancel()

			_, err := sm.ReconcileDesiredState(cancelCtx, workspace, nil)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("status update errors", func() {
		It("should propagate UpdateStoppedStatus error", func() {
			workspace := newStoppedWorkspace()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			makeWorkspaceStale(workspace)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update Workspace.Status"))
		})

		It("should propagate UpdateStoppingStatus error", func() {
			workspace := newStoppedWorkspace()
			createLiveDeployment(workspace)
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			makeWorkspaceStale(workspace)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update Workspace.Status"))
		})
	})
})
