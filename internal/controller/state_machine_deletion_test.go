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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("ReconcileDeletion", func() {
	var ctx context.Context

	newDeletingWorkspace := func() *workspacev1alpha1.Workspace {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:       fmt.Sprintf("sm-del-%d", time.Now().UnixNano()),
				Namespace:  testNamespace,
				Finalizers: []string{WorkspaceFinalizerName},
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Image:         imageBaseNotebook,
				DesiredStatus: DesiredStateRunning,
			},
		}
		Expect(k8sClient.Create(ctx, ws)).To(Succeed())

		// Set to Running state first so Available=True
		statusManager := NewStatusManager(k8sClient)
		snapshot := ws.Status.DeepCopy()
		Expect(statusManager.UpdateRunningStatus(ctx, ws, snapshot)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ws), ws)).To(Succeed())

		// Issue delete (workspace stays due to finalizer)
		Expect(k8sClient.Delete(ctx, ws)).To(Succeed())
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ws), ws)).To(Succeed())
		Expect(ws.DeletionTimestamp.IsZero()).To(BeFalse())
		return ws
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

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("should set Deleting=True and Available=False on a running workspace", func() {
		ws := newDeletingWorkspace()

		// Verify Available=True before ReconcileDeletion
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ws), ws)).To(Succeed())
		availableCond := findCondition(ws.Status.Conditions, ConditionTypeAvailable)
		Expect(availableCond).NotTo(BeNil())
		Expect(availableCond.Status).To(Equal(metav1.ConditionTrue))

		sm := buildStateMachine()
		_, err := sm.ReconcileDeletion(ctx, ws)
		Expect(err).NotTo(HaveOccurred())

		// After ReconcileDeletion, the workspace is modified in-place with updated conditions.
		// The object may be garbage-collected (finalizer removed + DeletionTimestamp set),
		// so we verify the in-memory object rather than reloading from the API.
		deletingCond := findCondition(ws.Status.Conditions, ConditionTypeDeleting)
		Expect(deletingCond).NotTo(BeNil())
		Expect(deletingCond.Status).To(Equal(metav1.ConditionTrue))
		Expect(deletingCond.Reason).To(Equal(ReasonDeletionInProgress))

		availableCond = findCondition(ws.Status.Conditions, ConditionTypeAvailable)
		Expect(availableCond).NotTo(BeNil())
		Expect(availableCond.Status).To(Equal(metav1.ConditionFalse))

		progressingCond := findCondition(ws.Status.Conditions, ConditionTypeProgressing)
		Expect(progressingCond).NotTo(BeNil())
		Expect(progressingCond.Status).To(Equal(metav1.ConditionFalse))

		degradedCond := findCondition(ws.Status.Conditions, ConditionTypeDegraded)
		Expect(degradedCond).NotTo(BeNil())
		Expect(degradedCond.Status).To(Equal(metav1.ConditionFalse))

		stoppedCond := findCondition(ws.Status.Conditions, ConditionTypeStopped)
		Expect(stoppedCond).NotTo(BeNil())
		Expect(stoppedCond.Status).To(Equal(metav1.ConditionFalse))
	})

	It("should skip when no finalizer is present", func() {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("sm-del-nofin-%d", time.Now().UnixNano()),
				Namespace: testNamespace,
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Image:         imageBaseNotebook,
				DesiredStatus: DesiredStateRunning,
			},
		}
		Expect(k8sClient.Create(ctx, ws)).To(Succeed())
		defer func() { _ = k8sClient.Delete(ctx, ws) }()

		sm := buildStateMachine()
		result, err := sm.ReconcileDeletion(ctx, ws)
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(time.Duration(0)))
	})

	It("should remove finalizer and allow deletion when all resources are cleaned up", func() {
		ws := newDeletingWorkspace()

		sm := buildStateMachine()
		_, err := sm.ReconcileDeletion(ctx, ws)
		Expect(err).NotTo(HaveOccurred())

		// Finalizer removed in-place — object is garbage-collected by the API server
		Expect(controllerutil.ContainsFinalizer(ws, WorkspaceFinalizerName)).To(BeFalse())
	})

	It("should clear DeploymentName and ServiceName", func() {
		ws := newDeletingWorkspace()

		// Set resource names
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(ws), ws)).To(Succeed())
		ws.Status.DeploymentName = "del-test-deploy"
		ws.Status.ServiceName = "del-test-svc"
		Expect(k8sClient.Status().Update(ctx, ws)).To(Succeed())

		sm := buildStateMachine()
		_, err := sm.ReconcileDeletion(ctx, ws)
		Expect(err).NotTo(HaveOccurred())

		// Verify in-memory object (workspace is garbage-collected after finalizer removal)
		Expect(ws.Status.DeploymentName).To(BeEmpty())
		Expect(ws.Status.ServiceName).To(BeEmpty())
	})
})
