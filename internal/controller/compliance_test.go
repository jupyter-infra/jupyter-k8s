/*
MIT License

Copyright (c) 2025 jupyter-ai-contrib

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.
*/

package controller

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
)

// MockTemplateValidator implements TemplateValidatorInterface for testing
type MockTemplateValidator struct {
	validateError error
	validateCalls int
}

func (m *MockTemplateValidator) ValidateCreateWorkspace(ctx context.Context, workspace *workspacev1alpha1.Workspace) error {
	m.validateCalls++
	return m.validateError
}

var _ = Describe("Compliance Checking (GAP-5)", func() {
	var (
		ctx       context.Context
		workspace *workspacev1alpha1.Workspace
		sm        *StateMachine
		validator *MockTemplateValidator
		recorder  *record.FakeRecorder
	)

	BeforeEach(func() {
		ctx = context.Background()
		workspace = &workspacev1alpha1.Workspace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "compliance-test-workspace",
				Namespace: "default",
				Labels: map[string]string{
					LabelWorkspaceTemplate: "test-template",
				},
			},
			Spec: workspacev1alpha1.WorkspaceSpec{
				DisplayName: "Compliance Test",
				Image:       "jupyter/base-notebook:latest",
				TemplateRef: &workspacev1alpha1.TemplateRef{Name: "test-template"},
			},
		}
		Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

		// Setup mocks
		validator = &MockTemplateValidator{}
		recorder = record.NewFakeRecorder(100)

		statusManager := &StatusManager{client: k8sClient}
		resourceManager := &ResourceManager{client: k8sClient, statusManager: statusManager}

		sm = &StateMachine{
			resourceManager:   resourceManager,
			statusManager:     statusManager,
			recorder:          recorder,
			templateValidator: validator,
		}
	})

	AfterEach(func() {
		Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, workspace))).To(Succeed())
	})

	Context("checkComplianceIfNeeded", func() {
		It("should skip validation when compliance label is not set", func() {
			// When workspace has no compliance check label, validation should be skipped entirely
			// This is the normal case - most workspaces don't need compliance checking
			snapshotStatus := workspace.Status.DeepCopy()
			result, err := sm.checkComplianceIfNeeded(ctx, workspace, snapshotStatus)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(validator.validateCalls).To(Equal(0), "validation should not be called")
		})

		It("should skip validation when compliance label is set to false", func() {
			// When label is explicitly set to "false", validation should also be skipped
			// This handles edge cases where label exists but compliance check is not needed
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
			workspace.Labels[LabelComplianceCheckNeeded] = "false"
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())

			snapshotStatus := workspace.Status.DeepCopy()
			result, err := sm.checkComplianceIfNeeded(ctx, workspace, snapshotStatus)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(validator.validateCalls).To(Equal(0), "validation should not be called")
		})

		It("should validate workspace and remove label when compliance check passes", func() {
			// When template webhook marks workspace with compliance label (after template update),
			// controller validates workspace against current template constraints and removes label
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
			workspace.Labels[LabelComplianceCheckNeeded] = LabelValueComplianceNeeded
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			validator.validateError = nil // Workspace complies with template
			snapshotStatus := workspace.Status.DeepCopy()
			result, err := sm.checkComplianceIfNeeded(ctx, workspace, snapshotStatus)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())
			Expect(validator.validateCalls).To(Equal(1), "validation should be called once")

			// Label removal is idempotent - prevents infinite reconciliation loops
			updatedWorkspace := &workspacev1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())
			Expect(updatedWorkspace.Labels[LabelComplianceCheckNeeded]).To(BeEmpty(), "compliance label should be removed")

			// Success event provides audit trail for compliance checks
			Eventually(recorder.Events).Should(Receive(ContainSubstring("ComplianceCheckPassed")))
		})

		It("should mark workspace invalid when compliance check fails and requeue for user remediation", func() {
			// When workspace violates template constraints (e.g., after template becomes more restrictive),
			// controller marks workspace invalid, removes label, and requeues with long delay for user to fix
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
			workspace.Labels[LabelComplianceCheckNeeded] = LabelValueComplianceNeeded
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			validator.validateError = fmt.Errorf("image not allowed by template")
			snapshotStatus := workspace.Status.DeepCopy()
			result, err := sm.checkComplianceIfNeeded(ctx, workspace, snapshotStatus)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("image not allowed"))
			Expect(result.RequeueAfter).To(Equal(LongRequeueDelay))
			Expect(validator.validateCalls).To(Equal(1))

			// Label removal is idempotent even on failure - prevents continuous validation attempts
			updatedWorkspace := &workspacev1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())
			Expect(updatedWorkspace.Labels[LabelComplianceCheckNeeded]).To(BeEmpty(), "compliance label should be removed even on failure")

			// Failure event provides visibility to cluster admins and workspace owners
			Eventually(recorder.Events).Should(Receive(ContainSubstring("ComplianceCheckFailed")))

			// Valid=False condition signals workspace is non-compliant without blocking other operations
			Eventually(func() bool {
				_ = k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)
				for _, cond := range updatedWorkspace.Status.Conditions {
					if cond.Type == ConditionTypeValid && cond.Status == metav1.ConditionFalse {
						return true
					}
				}
				return false
			}).Should(BeTrue(), "Valid condition should be False after compliance check failure")
		})

		It("should handle missing validator gracefully during startup or reconfiguration", func() {
			// During controller startup or webhook unavailability, validator may be nil
			// Controller should handle this gracefully by skipping validation and removing label
			sm.templateValidator = nil

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
			workspace.Labels[LabelComplianceCheckNeeded] = LabelValueComplianceNeeded
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			snapshotStatus := workspace.Status.DeepCopy()
			result, err := sm.checkComplianceIfNeeded(ctx, workspace, snapshotStatus)

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeZero())

			// Label removal prevents infinite loops even when validator is unavailable
			updatedWorkspace := &workspacev1alpha1.Workspace{}
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace)).To(Succeed())
			Expect(updatedWorkspace.Labels[LabelComplianceCheckNeeded]).To(BeEmpty())
		})

		It("should be idempotent - second call skips validation after label removed", func() {
			// Idempotency ensures controller doesn't repeatedly validate same workspace
			// Critical for preventing reconciliation loops and reducing API server load
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())
			workspace.Labels[LabelComplianceCheckNeeded] = LabelValueComplianceNeeded
			Expect(k8sClient.Update(ctx, workspace)).To(Succeed())
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			validator.validateError = nil
			snapshotStatus := workspace.Status.DeepCopy()

			// First call validates and removes label
			result1, err1 := sm.checkComplianceIfNeeded(ctx, workspace, snapshotStatus)
			Expect(err1).NotTo(HaveOccurred())
			Expect(result1.RequeueAfter).To(BeZero())
			Expect(validator.validateCalls).To(Equal(1))

			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), workspace)).To(Succeed())

			// Second call sees no label and skips validation entirely
			result2, err2 := sm.checkComplianceIfNeeded(ctx, workspace, snapshotStatus)
			Expect(err2).NotTo(HaveOccurred())
			Expect(result2.RequeueAfter).To(BeZero())
			Expect(validator.validateCalls).To(Equal(1), "validation should not be called again")
		})
	})

	Context("getTemplateRef helper", func() {
		It("should return template name when set", func() {
			Expect(getTemplateRef(workspace)).To(Equal("test-template"))
		})

		It("should return empty string when templateRef is nil", func() {
			workspace.Spec.TemplateRef = nil
			Expect(getTemplateRef(workspace)).To(BeEmpty())
		})
	})
})
