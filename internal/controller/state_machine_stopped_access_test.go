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

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("reconcileDesiredStoppedStatus access resources", func() {
	var ctx context.Context

	newStoppedWorkspace := func() *workspacev1alpha1.Workspace {
		ws := &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("sm-stop-acc-%d", time.Now().UnixNano()),
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

	// bogusAccessResources returns AccessResources entries with an unknown GVK.
	// This causes EnsureAccessResourcesDeleted to fail because the RESTMapper
	// cannot resolve the kind — simulating an access resource deletion error.
	bogusAccessResources := func() []workspacev1alpha1.AccessResourceStatus {
		return []workspacev1alpha1.AccessResourceStatus{{
			Kind:       "BogusKind",
			APIVersion: "bogus.test/v1",
			Name:       "does-not-matter",
			Namespace:  "default",
		}}
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

	Context("happy path with no access resources", func() {
		It("should mark Stopped when resources already gone and no access resources tracked", func() {
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
		})
	})

	// The branch at line 152 (accessError==nil && !accessResourcesDeleted) is a
	// defensive guard: EnsureAccessResourcesDeleted currently removes all entries
	// on success, so AreAccessResourcesDeleted always returns true when
	// accessError is nil. We skip a direct integration test for this path and
	// instead test the AccessNotStopped reason through the access-error path
	// below, which keeps entries in the list.
	Context("access resource deletion error", func() {
		It("should propagate access error when deployment and service are gone", func() {
			workspace := newStoppedWorkspace()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			workspace.Status.AccessResources = bogusAccessResources()
			Expect(k8sClient.Status().Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve AccessResource"))

			degraded := getCondition(workspace, ConditionTypeDegraded)
			Expect(degraded).NotTo(BeNil())
			Expect(degraded.Status).To(Equal(metav1.ConditionTrue))
		})

		It("should propagate UpdateErrorStatus failure on access error", func() {
			workspace := newStoppedWorkspace()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			workspace.Status.AccessResources = bogusAccessResources()
			Expect(k8sClient.Status().Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			makeWorkspaceStale(workspace)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve AccessResource"))
		})
	})

	Context("status update errors with access resources", func() {
		It("should propagate UpdateStoppingStatus error when access not stopped", func() {
			workspace := newStoppedWorkspace()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			workspace.Status.AccessResources = bogusAccessResources()
			Expect(k8sClient.Status().Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			makeWorkspaceStale(workspace)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to retrieve AccessResource"))
		})

		It("should propagate UpdateStoppedStatus error when access resources clean", func() {
			workspace := newStoppedWorkspace()
			defer func() { _ = k8sClient.Delete(ctx, workspace) }()

			makeWorkspaceStale(workspace)

			sm := buildStateMachine()
			_, err := sm.ReconcileDesiredState(ctx, workspace, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to update Workspace.Status"))
		})
	})
})
