package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("StatusManager", func() {
	var (
		ctx           context.Context
		statusManager *StatusManager
		workspace     *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		ctx = context.Background()
		statusManager = NewStatusManager(k8sClient)

		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workspace",
				Namespace: "default",
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				Image: "jupyter/base-notebook:latest",
			},
		}
	})

	Context("Update Status Methods", func() {
		BeforeEach(func() {
			// Create the workspace in the test cluster first
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up the workspace
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())
		})

		It("should set deployment updating status", func() {
			snapshotStatus := workspace.Status.DeepCopy()

			err := statusManager.UpdateDeploymentUpdatingStatus(ctx, workspace, snapshotStatus)
			Expect(err).NotTo(HaveOccurred())

			// Find the Progressing condition
			var progressingCondition *metav1.Condition
			for i := range workspace.Status.Conditions {
				if workspace.Status.Conditions[i].Type == ConditionTypeProgressing {
					progressingCondition = &workspace.Status.Conditions[i]
					break
				}
			}

			Expect(progressingCondition).NotTo(BeNil())
			Expect(progressingCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(progressingCondition.Reason).To(Equal(ReasonDeploymentUpdating))
			Expect(progressingCondition.Message).To(Equal("Deployment is being updated"))
		})

		It("should set service updating status", func() {
			snapshotStatus := workspace.Status.DeepCopy()

			err := statusManager.UpdateServiceUpdatingStatus(ctx, workspace, snapshotStatus)
			Expect(err).NotTo(HaveOccurred())

			// Find the Progressing condition
			var progressingCondition *metav1.Condition
			for i := range workspace.Status.Conditions {
				if workspace.Status.Conditions[i].Type == ConditionTypeProgressing {
					progressingCondition = &workspace.Status.Conditions[i]
					break
				}
			}

			Expect(progressingCondition).NotTo(BeNil())
			Expect(progressingCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(progressingCondition.Reason).To(Equal(ReasonServiceUpdating))
			Expect(progressingCondition.Message).To(Equal("Service is being updated"))
		})

		It("should set PVC updating status", func() {
			snapshotStatus := workspace.Status.DeepCopy()

			err := statusManager.UpdatePVCUpdatingStatus(ctx, workspace, snapshotStatus)
			Expect(err).NotTo(HaveOccurred())

			// Find the Progressing condition
			var progressingCondition *metav1.Condition
			for i := range workspace.Status.Conditions {
				if workspace.Status.Conditions[i].Type == ConditionTypeProgressing {
					progressingCondition = &workspace.Status.Conditions[i]
					break
				}
			}

			Expect(progressingCondition).NotTo(BeNil())
			Expect(progressingCondition.Status).To(Equal(metav1.ConditionTrue))
			Expect(progressingCondition.Reason).To(Equal(ReasonPVCUpdating))
			Expect(progressingCondition.Message).To(Equal("PVC is being updated"))
		})
	})
})
