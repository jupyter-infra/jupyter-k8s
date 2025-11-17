package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

var _ = Describe("StatusManager", func() {
	var (
		ctx       context.Context
		workspace *workspacev1alpha1.Workspace
	)

	BeforeEach(func() {
		ctx = context.Background()

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
		var statusManager *StatusManager

		BeforeEach(func() {
			// Create the workspace in the test cluster first
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
			statusManager = NewStatusManager(k8sClient)
		})

		AfterEach(func() {
			// Clean up the workspace
			Expect(k8sClient.Delete(ctx, workspace)).To(Succeed())
		})

		Describe("UpdateRunningStatus", func() {
			It("should set correct condition states for running workspace", func() {
				snapshot := workspace.Status.DeepCopy()
				err := statusManager.UpdateRunningStatus(ctx, workspace, snapshot)
				Expect(err).NotTo(HaveOccurred())

				// Reload workspace to get updated status
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

				// Verify exactly 4 conditions
				Expect(workspace.Status.Conditions).To(HaveLen(4))

				// Verify Available=True
				availableCond := findCondition(workspace.Status.Conditions, ConditionTypeAvailable)
				Expect(availableCond).NotTo(BeNil())
				Expect(availableCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(availableCond.Reason).To(Equal(ReasonResourcesReady))

				// Verify Progressing=False
				progressingCond := findCondition(workspace.Status.Conditions, ConditionTypeProgressing)
				Expect(progressingCond).NotTo(BeNil())
				Expect(progressingCond.Status).To(Equal(metav1.ConditionFalse))

				// Verify Degraded=False
				degradedCond := findCondition(workspace.Status.Conditions, ConditionTypeDegraded)
				Expect(degradedCond).NotTo(BeNil())
				Expect(degradedCond.Status).To(Equal(metav1.ConditionFalse))

				// Verify Stopped=False
				stoppedCond := findCondition(workspace.Status.Conditions, ConditionTypeStopped)
				Expect(stoppedCond).NotTo(BeNil())
				Expect(stoppedCond.Status).To(Equal(metav1.ConditionFalse))
			})
		})

		Describe("UpdateStartingStatus", func() {
			It("should set appropriate reason when compute not ready", func() {
				readiness := WorkspaceRunningReadiness{
					computeReady:         false,
					serviceReady:         true,
					accessResourcesReady: true,
				}

				snapshot := workspace.Status.DeepCopy()
				err := statusManager.UpdateStartingStatus(ctx, workspace, readiness, snapshot)
				Expect(err).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

				// Verify exactly 4 conditions
				Expect(workspace.Status.Conditions).To(HaveLen(4))

				// Verify Available=False with ComputeNotReady reason
				availableCond := findCondition(workspace.Status.Conditions, ConditionTypeAvailable)
				Expect(availableCond).NotTo(BeNil())
				Expect(availableCond.Status).To(Equal(metav1.ConditionFalse))
				Expect(availableCond.Reason).To(Equal(ReasonComputeNotReady))

				// Verify Progressing=True
				progressingCond := findCondition(workspace.Status.Conditions, ConditionTypeProgressing)
				Expect(progressingCond).NotTo(BeNil())
				Expect(progressingCond.Status).To(Equal(metav1.ConditionTrue))

				// Verify Degraded=False
				degradedCond := findCondition(workspace.Status.Conditions, ConditionTypeDegraded)
				Expect(degradedCond).NotTo(BeNil())
				Expect(degradedCond.Status).To(Equal(metav1.ConditionFalse))

				// Verify Stopped=False
				stoppedCond := findCondition(workspace.Status.Conditions, ConditionTypeStopped)
				Expect(stoppedCond).NotTo(BeNil())
				Expect(stoppedCond.Status).To(Equal(metav1.ConditionFalse))
			})

			It("should set appropriate reason when service not ready", func() {
				readiness := WorkspaceRunningReadiness{
					computeReady:         true,
					serviceReady:         false,
					accessResourcesReady: true,
				}

				snapshot := workspace.Status.DeepCopy()
				err := statusManager.UpdateStartingStatus(ctx, workspace, readiness, snapshot)
				Expect(err).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

				// Verify exactly 4 conditions
				Expect(workspace.Status.Conditions).To(HaveLen(4))

				// Verify Available=False with ServiceNotReady reason
				availableCond := findCondition(workspace.Status.Conditions, ConditionTypeAvailable)
				Expect(availableCond).NotTo(BeNil())
				Expect(availableCond.Status).To(Equal(metav1.ConditionFalse))
				Expect(availableCond.Reason).To(Equal(ReasonServiceNotReady))

				// Verify Progressing=True
				progressingCond := findCondition(workspace.Status.Conditions, ConditionTypeProgressing)
				Expect(progressingCond).NotTo(BeNil())
				Expect(progressingCond.Status).To(Equal(metav1.ConditionTrue))

				// Verify Degraded=False
				degradedCond := findCondition(workspace.Status.Conditions, ConditionTypeDegraded)
				Expect(degradedCond).NotTo(BeNil())
				Expect(degradedCond.Status).To(Equal(metav1.ConditionFalse))

				// Verify Stopped=False
				stoppedCond := findCondition(workspace.Status.Conditions, ConditionTypeStopped)
				Expect(stoppedCond).NotTo(BeNil())
				Expect(stoppedCond.Status).To(Equal(metav1.ConditionFalse))
			})

			It("should set appropriate reason when access not ready", func() {
				readiness := WorkspaceRunningReadiness{
					computeReady:         true,
					serviceReady:         true,
					accessResourcesReady: false,
				}

				snapshot := workspace.Status.DeepCopy()
				err := statusManager.UpdateStartingStatus(ctx, workspace, readiness, snapshot)
				Expect(err).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

				// Verify exactly 4 conditions
				Expect(workspace.Status.Conditions).To(HaveLen(4))

				// Verify Available=False with AccessNotReady reason
				availableCond := findCondition(workspace.Status.Conditions, ConditionTypeAvailable)
				Expect(availableCond).NotTo(BeNil())
				Expect(availableCond.Status).To(Equal(metav1.ConditionFalse))
				Expect(availableCond.Reason).To(Equal(ReasonAccessNotReady))

				// Verify Progressing=True
				progressingCond := findCondition(workspace.Status.Conditions, ConditionTypeProgressing)
				Expect(progressingCond).NotTo(BeNil())
				Expect(progressingCond.Status).To(Equal(metav1.ConditionTrue))

				// Verify Degraded=False
				degradedCond := findCondition(workspace.Status.Conditions, ConditionTypeDegraded)
				Expect(degradedCond).NotTo(BeNil())
				Expect(degradedCond.Status).To(Equal(metav1.ConditionFalse))

				// Verify Stopped=False
				stoppedCond := findCondition(workspace.Status.Conditions, ConditionTypeStopped)
				Expect(stoppedCond).NotTo(BeNil())
				Expect(stoppedCond.Status).To(Equal(metav1.ConditionFalse))
			})
		})

		Describe("UpdateStoppedStatus", func() {
			It("should set Stopped=True and clear resource names", func() {
				workspace.Status.DeploymentName = "test-deployment"
				workspace.Status.ServiceName = "test-service"

				snapshot := workspace.Status.DeepCopy()
				err := statusManager.UpdateStoppedStatus(ctx, workspace, snapshot)
				Expect(err).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

				// Verify Stopped=True
				stoppedCond := findCondition(workspace.Status.Conditions, ConditionTypeStopped)
				Expect(stoppedCond).NotTo(BeNil())
				Expect(stoppedCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(stoppedCond.Reason).To(Equal(ReasonResourcesStopped))

				// Verify resource names cleared
				Expect(workspace.Status.DeploymentName).To(BeEmpty())
				Expect(workspace.Status.ServiceName).To(BeEmpty())
			})

			It("should include preemption reason when annotation present", func() {
				workspace.Annotations = map[string]string{
					PreemptionReasonAnnotation: PreemptedReason,
				}

				snapshot := workspace.Status.DeepCopy()
				err := statusManager.UpdateStoppedStatus(ctx, workspace, snapshot)
				Expect(err).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

				// Verify Available has Preempted reason
				availableCond := findCondition(workspace.Status.Conditions, ConditionTypeAvailable)
				Expect(availableCond).NotTo(BeNil())
				Expect(availableCond.Reason).To(Equal(ReasonPreempted))
			})
		})

		Describe("UpdateStoppingStatus", func() {
			It("should set Progressing=True while stopping", func() {
				readiness := WorkspaceStoppingReadiness{
					computeStopped:         false,
					serviceStopped:         true,
					accessResourcesStopped: true,
				}

				snapshot := workspace.Status.DeepCopy()
				err := statusManager.UpdateStoppingStatus(ctx, workspace, readiness, snapshot)
				Expect(err).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

				// Verify Progressing=True
				progressingCond := findCondition(workspace.Status.Conditions, ConditionTypeProgressing)
				Expect(progressingCond).NotTo(BeNil())
				Expect(progressingCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(progressingCond.Reason).To(Equal(ReasonDesiredStateStopped))
				Expect(progressingCond.Message).To(Equal("Compute is still running"))

				// Verify Stopped=False with specific reason
				stoppedCond := findCondition(workspace.Status.Conditions, ConditionTypeStopped)
				Expect(stoppedCond).NotTo(BeNil())
				Expect(stoppedCond.Status).To(Equal(metav1.ConditionFalse))
				Expect(stoppedCond.Reason).To(Equal(ReasonComputeNotStopped))
			})
		})

		Describe("UpdateErrorStatus", func() {
			It("should set Degraded=True with provided reason and message", func() {
				// First set workspace to Running state
				runningSnapshot := workspace.Status.DeepCopy()
				err := statusManager.UpdateRunningStatus(ctx, workspace, runningSnapshot)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

				// Now set error status - this should merge with existing conditions
				snapshot := workspace.Status.DeepCopy()
				err = statusManager.UpdateErrorStatus(ctx, workspace, ReasonDeploymentError, "Deployment failed", snapshot)
				Expect(err).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

				// Verify exactly 4 conditions
				Expect(workspace.Status.Conditions).To(HaveLen(4))

				// Verify Degraded=True (changed from Running state)
				degradedCond := findCondition(workspace.Status.Conditions, ConditionTypeDegraded)
				Expect(degradedCond).NotTo(BeNil())
				Expect(degradedCond.Status).To(Equal(metav1.ConditionTrue))
				Expect(degradedCond.Reason).To(Equal(ReasonDeploymentError))
				Expect(degradedCond.Message).To(Equal("Deployment failed"))

				// Verify other 3 conditions remain from Running state
				availableCond := findCondition(workspace.Status.Conditions, ConditionTypeAvailable)
				Expect(availableCond).NotTo(BeNil())
				Expect(availableCond.Status).To(Equal(metav1.ConditionTrue))

				progressingCond := findCondition(workspace.Status.Conditions, ConditionTypeProgressing)
				Expect(progressingCond).NotTo(BeNil())
				Expect(progressingCond.Status).To(Equal(metav1.ConditionFalse))

				stoppedCond := findCondition(workspace.Status.Conditions, ConditionTypeStopped)
				Expect(stoppedCond).NotTo(BeNil())
				Expect(stoppedCond.Status).To(Equal(metav1.ConditionFalse))
			})
		})

		Describe("IsWorkspaceAvailable", func() {
			It("should return true when Available=True", func() {
				snapshot := workspace.Status.DeepCopy()
				err := statusManager.UpdateRunningStatus(ctx, workspace, snapshot)
				Expect(err).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
				Expect(statusManager.IsWorkspaceAvailable(workspace)).To(BeTrue())
			})

			It("should return false when Available=False", func() {
				readiness := WorkspaceRunningReadiness{
					computeReady:         false,
					serviceReady:         false,
					accessResourcesReady: false,
				}

				snapshot := workspace.Status.DeepCopy()
				err := statusManager.UpdateStartingStatus(ctx, workspace, readiness, snapshot)
				Expect(err).NotTo(HaveOccurred())

				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
				Expect(statusManager.IsWorkspaceAvailable(workspace)).To(BeFalse())
			})
		})

		Describe("Condition Ordering Consistency", func() {
			It("should maintain consistent condition order across all status updates", func() {
				expectedOrder := []string{
					ConditionTypeAvailable,
					ConditionTypeProgressing,
					ConditionTypeDegraded,
					ConditionTypeStopped,
				}

				// Test UpdateStartingStatus
				snapshot := workspace.Status.DeepCopy()
				readiness := WorkspaceRunningReadiness{
					computeReady:         false,
					serviceReady:         false,
					accessResourcesReady: false,
				}
				err := statusManager.UpdateStartingStatus(ctx, workspace, readiness, snapshot)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
				verifyConditionOrder(workspace.Status.Conditions, expectedOrder)

				// Test UpdateRunningStatus
				snapshot = workspace.Status.DeepCopy()
				err = statusManager.UpdateRunningStatus(ctx, workspace, snapshot)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
				verifyConditionOrder(workspace.Status.Conditions, expectedOrder)

				// Test UpdateStoppingStatus
				snapshot = workspace.Status.DeepCopy()
				stoppingReadiness := WorkspaceStoppingReadiness{
					computeStopped:         false,
					serviceStopped:         true,
					accessResourcesStopped: true,
				}
				err = statusManager.UpdateStoppingStatus(ctx, workspace, stoppingReadiness, snapshot)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
				verifyConditionOrder(workspace.Status.Conditions, expectedOrder)

				// Test UpdateStoppedStatus
				snapshot = workspace.Status.DeepCopy()
				err = statusManager.UpdateStoppedStatus(ctx, workspace, snapshot)
				Expect(err).NotTo(HaveOccurred())
				Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
				verifyConditionOrder(workspace.Status.Conditions, expectedOrder)
			})
		})
	})
})

// Helper function to find a condition by type
// Returns a pointer to a copy of the condition to avoid pointer aliasing issues
func findCondition(conditions []metav1.Condition, condType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == condType {
			cond := conditions[i]
			return &cond
		}
	}
	return nil
}

// Helper function to verify that conditions appear in the expected order
func verifyConditionOrder(conditions []metav1.Condition, expectedOrder []string) {
	Expect(conditions).To(HaveLen(len(expectedOrder)), "Should have exactly %d conditions", len(expectedOrder))

	for i, expectedType := range expectedOrder {
		Expect(conditions[i].Type).To(Equal(expectedType),
			"Condition at index %d should be %s, got %s", i, expectedType, conditions[i].Type)
	}
}
