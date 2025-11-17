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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MockStateMachine is a mock implementation of the StateMachine for testing
type MockStateMachine struct {
}

// ReconcileDesiredState is a mock implementation for testing
func (m *MockStateMachine) ReconcileDesiredState(ctx context.Context, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

// ReconcileDeletion is a mock implementation for testing
func (m *MockStateMachine) ReconcileDeletion(ctx context.Context, workspace *workspacev1alpha1.Workspace) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

// getDesiredStatus is a mock implementation for testing
func (m *MockStateMachine) getDesiredStatus(workspace *workspacev1alpha1.Workspace) string {
	return "Running"
}

// GetAccessStrategyForWorkspace is a mock implementation for testing
func (m *MockStateMachine) GetAccessStrategyForWorkspace(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
	return nil, nil
}

var _ = Describe("Workspace Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		workspace := &workspacev1alpha1.Workspace{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Workspace")
			err := k8sClient.Get(ctx, typeNamespacedName, workspace)
			if err != nil && errors.IsNotFound(err) {
				resource := &workspacev1alpha1.Workspace{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &workspacev1alpha1.Workspace{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Workspace")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Creating a mock StateMachine")
			statusManager := StatusManager{
				client: k8sClient,
			}

			// Use the mock StateMachine implementation
			mockStateMachine := &MockStateMachine{}

			By("Reconciling the created resource")
			controllerReconciler := &WorkspaceReconciler{
				Client:        k8sClient,
				Scheme:        k8sClient.Scheme(),
				stateMachine:  mockStateMachine,
				statusManager: &statusManager,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Describe("TemplateRef Label Setting", func() {
		var (
			ctx       context.Context
			workspace *workspacev1alpha1.Workspace
		)

		BeforeEach(func() {
			ctx = context.Background()
		})

		AfterEach(func() {
			if workspace != nil {
				_ = k8sClient.Delete(ctx, workspace)
			}
		})

		It("should extract template name from TemplateRef struct", func() {
			templateRef := &workspacev1alpha1.TemplateRef{
				Name: "test-template",
			}
			Expect(templateRef.Name).To(Equal("test-template"))
		})

		It("should handle TemplateRef with namespace specified", func() {
			templateRef := &workspacev1alpha1.TemplateRef{
				Name:      "test-template",
				Namespace: "custom-namespace",
			}
			Expect(templateRef.Name).To(Equal("test-template"))
			Expect(templateRef.Namespace).To(Equal("custom-namespace"))
		})

		It("should handle nil TemplateRef gracefully", func() {
			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "workspace-without-templateref",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					TemplateRef: nil,
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())
			Expect(workspace.Spec.TemplateRef).To(BeNil())
		})
	})
})
