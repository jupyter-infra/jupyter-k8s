/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package controller

import (
	"context"
	"fmt"
	"time"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MockStateMachine is a mock implementation of the StateMachine for testing
type MockStateMachine struct {
	reconcileDesiredStateFunc         func(ctx context.Context, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (ctrl.Result, error)
	reconcileDeletionFunc             func(ctx context.Context, workspace *workspacev1alpha1.Workspace) (ctrl.Result, error)
	getDesiredStatusFunc              func(workspace *workspacev1alpha1.Workspace) string
	getAccessStrategyForWorkspaceFunc func(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*workspacev1alpha1.WorkspaceAccessStrategy, error)
}

// ReconcileDesiredState is a mock implementation for testing
func (m *MockStateMachine) ReconcileDesiredState(ctx context.Context, workspace *workspacev1alpha1.Workspace, accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy) (ctrl.Result, error) {
	if m.reconcileDesiredStateFunc != nil {
		return m.reconcileDesiredStateFunc(ctx, workspace, accessStrategy)
	}
	return ctrl.Result{}, nil
}

// ReconcileDeletion is a mock implementation for testing
func (m *MockStateMachine) ReconcileDeletion(ctx context.Context, workspace *workspacev1alpha1.Workspace) (ctrl.Result, error) {
	if m.reconcileDeletionFunc != nil {
		return m.reconcileDeletionFunc(ctx, workspace)
	}
	return ctrl.Result{}, nil
}

// getDesiredStatus is a mock implementation for testing
func (m *MockStateMachine) getDesiredStatus(workspace *workspacev1alpha1.Workspace) string {
	if m.getDesiredStatusFunc != nil {
		return m.getDesiredStatusFunc(workspace)
	}
	return "Running"
}

// GetAccessStrategyForWorkspace is a mock implementation for testing
func (m *MockStateMachine) GetAccessStrategyForWorkspace(ctx context.Context, workspace *workspacev1alpha1.Workspace) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
	if m.getAccessStrategyForWorkspaceFunc != nil {
		return m.getAccessStrategyForWorkspaceFunc(ctx, workspace)
	}
	return nil, nil
}

// generateUniqueName generates a unique resource name for tests
func generateUniqueName(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

var _ = Describe("Workspace Controller", func() {
	Context("When reconciling a non-deleting Workspace", func() {
		// Using default namespace for workspaces to avoid permission issues
		const (
			workspaceNamespace      = "default"
			accessStrategyName      = "test-access-strategy"
			accessStrategyNamespace = "strategy-namespace"
			templateName            = "test-template"
			templateNamespace       = "template-namespace"
		)

		var (
			ctx            context.Context
			workspace      *workspacev1alpha1.Workspace
			accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
			workspaceKey   types.NamespacedName

			// Use variable instead of constant for workspace name to ensure uniqueness
			workspaceName string
		)

		BeforeEach(func() {
			ctx = context.Background()

			// Generate unique names to avoid conflicts between test runs
			workspaceName = generateUniqueName("test-workspace")

			workspaceKey = types.NamespacedName{
				Name:      workspaceName,
				Namespace: workspaceNamespace,
			}

			// Create the workspace
			By("creating the Workspace")
			workspace = &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workspaceName,
					Namespace: workspaceNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
				},
			}
			Expect(k8sClient.Create(ctx, workspace)).To(Succeed())

			// Define the access strategy object but don't create it
			// We'll use the mock StateMachine to return this when GetAccessStrategyForWorkspace is called
			accessStrategy = &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      accessStrategyName,
					Namespace: accessStrategyNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName:             "Test AccessStrategy",
					AccessResourceTemplates: []workspacev1alpha1.AccessResourceTemplate{},
				},
			}
		})

		AfterEach(func() {
			// Clean up in reverse order of creation

			// 1. First clean up the workspace
			if workspace != nil {
				// Get latest version to ensure we're working with current state
				updatedWorkspace := &workspacev1alpha1.Workspace{}
				if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(workspace), updatedWorkspace); err == nil {
					// Remove finalizers first to ensure clean deletion
					if len(updatedWorkspace.GetFinalizers()) > 0 {
						controllerutil.RemoveFinalizer(updatedWorkspace, WorkspaceFinalizerName)
						// Ignore errors as we're best-effort cleaning
						Expect(client.IgnoreNotFound(k8sClient.Update(ctx, updatedWorkspace))).To(Succeed())
					}
					// Then delete the resource (ignore errors)
					Expect(client.IgnoreNotFound(k8sClient.Delete(ctx, updatedWorkspace))).To(Succeed())
				}
			}
		})

		Context("When reconciling a Workspace with missing or incorrect finalizers and labels", func() {
			It("should add the finalizer but no template or access strategy label for a basic workspace", func() {
				By("Creating a mock StateMachine")
				statusManager := StatusManager{
					client: k8sClient,
				}

				// Use the mock StateMachine implementation with tracking for function calls
				reconcileDesiredStateCalled := false
				mockStateMachine := &MockStateMachine{
					// Set a mock function to return nil for any AccessStrategy
					getAccessStrategyForWorkspaceFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
						return nil, nil
					},
					// Track if ReconcileDesiredState was called
					reconcileDesiredStateFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
						accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
					) (ctrl.Result, error) {
						reconcileDesiredStateCalled = true
						return ctrl.Result{}, nil
					},
				}

				By("Reconciling the created resource")
				controllerReconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: &statusManager,
				}

				result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that Reconcile returns a requeue")
				Expect(result.RequeueAfter).NotTo(BeZero(), "Reconcile should return a RequeueAfter")

				By("Verifying that ReconcileDesiredState was not called")
				Expect(reconcileDesiredStateCalled).To(BeFalse(), "ReconcileDesiredState should not be called during label/finalizer updates")

				// Get the updated workspace from the API server
				updatedWorkspace := &workspacev1alpha1.Workspace{}
				err = k8sClient.Get(ctx, types.NamespacedName{Name: workspaceName, Namespace: workspaceNamespace}, updatedWorkspace)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that the finalizer was added")
				Expect(controllerutil.ContainsFinalizer(updatedWorkspace, WorkspaceFinalizerName)).To(BeTrue(), "Finalizer should be added")

				By("Verifying that the template and access strategy labels were not set")
				Expect(updatedWorkspace.Labels).NotTo(HaveKey(workspaceutil.LabelWorkspaceTemplate), "No template label should be set")
				Expect(updatedWorkspace.Labels).NotTo(HaveKey(workspaceutil.LabelWorkspaceTemplateNamespace), "No template namespace label should be set")
				Expect(updatedWorkspace.Labels).NotTo(HaveKey(LabelAccessStrategyName), "No access strategy label should be set")
				Expect(updatedWorkspace.Labels).NotTo(HaveKey(LabelAccessStrategyNamespace), "No access strategy namespace label should be set")
			})

			It("should add finalizer and template labels when Workspace references a template", func() {
				By("updating the existing Workspace to add a templateRef")

				// First get the current workspace
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Now update it to add the templateRef
				existingWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
					Name:      templateName,
					Namespace: templateNamespace,
				}
				existingWorkspace.Spec.DisplayName = "Test Workspace with Template"
				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				By("Setting up the reconciler")
				statusManager := StatusManager{
					client: k8sClient,
				}
				mockStateMachine := &MockStateMachine{
					// Mock to return the accessStrategy when workspace has a matching reference
					getAccessStrategyForWorkspaceFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
						if workspace.Spec.AccessStrategy != nil &&
							workspace.Spec.AccessStrategy.Name == accessStrategyName &&
							workspace.Spec.AccessStrategy.Namespace == accessStrategyNamespace {
							return accessStrategy, nil
						}
						return nil, nil
					},
				}

				controllerReconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: &statusManager,
				}

				By("Reconciling the workspace with template")
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				// Get the updated workspace
				updatedWorkspace := &workspacev1alpha1.Workspace{}
				err = k8sClient.Get(ctx, workspaceKey, updatedWorkspace)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that the finalizer was added")
				Expect(controllerutil.ContainsFinalizer(updatedWorkspace, WorkspaceFinalizerName)).To(BeTrue(), "Finalizer should be added")

				By("Verifying that template labels were set")
				Expect(updatedWorkspace.Labels).To(HaveKey(workspaceutil.LabelWorkspaceTemplate), "Template name label should be set")
				Expect(updatedWorkspace.Labels).To(HaveKey(workspaceutil.LabelWorkspaceTemplateNamespace), "Template namespace label should be set")
				Expect(updatedWorkspace.Labels[workspaceutil.LabelWorkspaceTemplate]).To(Equal(templateName), "Template name label should have correct value")
				Expect(updatedWorkspace.Labels[workspaceutil.LabelWorkspaceTemplateNamespace]).To(Equal(templateNamespace), "Template namespace label should have correct value")

				By("Verifying that access strategy labels were NOT set")
				Expect(updatedWorkspace.Labels).NotTo(HaveKey(LabelAccessStrategyName), "No access strategy label should be set")
				Expect(updatedWorkspace.Labels).NotTo(HaveKey(LabelAccessStrategyNamespace), "No access strategy namespace label should be set")
			})

			It("should add finalizer and access strategy labels when workspace references an AccessStrategy", func() {
				By("updating the existing Workspace to add an accessStrategy reference")

				// First get the current workspace
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Now update it to add the accessStrategy
				existingWorkspace.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
					Name:      accessStrategyName,
					Namespace: accessStrategyNamespace,
				}
				existingWorkspace.Spec.DisplayName = "Test Workspace with AccessStrategy"

				// Clear any template reference if it exists
				existingWorkspace.Spec.TemplateRef = nil

				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				By("Setting up the reconciler")
				statusManager := StatusManager{
					client: k8sClient,
				}
				mockStateMachine := &MockStateMachine{
					// Mock to return the accessStrategy when workspace has a matching reference
					getAccessStrategyForWorkspaceFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
						if workspace.Spec.AccessStrategy != nil &&
							workspace.Spec.AccessStrategy.Name == accessStrategyName &&
							workspace.Spec.AccessStrategy.Namespace == accessStrategyNamespace {
							return accessStrategy, nil
						}
						return nil, nil
					},
				}

				controllerReconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: &statusManager,
				}

				By("Reconciling the workspace with AccessStrategy")
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				// Get the updated workspace
				updatedWorkspace := &workspacev1alpha1.Workspace{}
				err = k8sClient.Get(ctx, workspaceKey, updatedWorkspace)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that the finalizer was added")
				Expect(controllerutil.ContainsFinalizer(updatedWorkspace, WorkspaceFinalizerName)).To(BeTrue(), "Finalizer should be added")

				By("Verifying that access strategy labels were set")
				Expect(updatedWorkspace.Labels).To(HaveKey(LabelAccessStrategyName), "Access strategy name label should be set")
				Expect(updatedWorkspace.Labels).To(HaveKey(LabelAccessStrategyNamespace), "Access strategy namespace label should be set")
				Expect(updatedWorkspace.Labels[LabelAccessStrategyName]).To(Equal(accessStrategyName), "Access strategy name label should have correct value")
				Expect(updatedWorkspace.Labels[LabelAccessStrategyNamespace]).To(Equal(accessStrategyNamespace), "Access strategy namespace label should have correct value")

				By("Verifying that template labels were NOT set")
				Expect(updatedWorkspace.Labels).NotTo(HaveKey(workspaceutil.LabelWorkspaceTemplate), "No template label should be set")
				Expect(updatedWorkspace.Labels).NotTo(HaveKey(workspaceutil.LabelWorkspaceTemplateNamespace), "No template namespace label should be set")
			})

			It("should add finalizer, template and access strategy labels when workspace references a Template and an AccessStrategy", func() {
				By("updating the existing Workspace to add both template and accessStrategy references")

				// First get the current workspace
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Now update it to add both template and accessStrategy
				existingWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
					Name:      templateName,
					Namespace: templateNamespace,
				}
				existingWorkspace.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
					Name:      accessStrategyName,
					Namespace: accessStrategyNamespace,
				}
				existingWorkspace.Spec.DisplayName = "Test Workspace with Template and AccessStrategy"

				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				By("Setting up the reconciler")
				statusManager := StatusManager{
					client: k8sClient,
				}
				mockStateMachine := &MockStateMachine{
					// Mock to return the accessStrategy when workspace has a matching reference
					getAccessStrategyForWorkspaceFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
						if workspace.Spec.AccessStrategy != nil &&
							workspace.Spec.AccessStrategy.Name == accessStrategyName &&
							workspace.Spec.AccessStrategy.Namespace == accessStrategyNamespace {
							return accessStrategy, nil
						}
						return nil, nil
					},
				}

				controllerReconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: &statusManager,
				}

				By("Reconciling the workspace with Template and AccessStrategy")
				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				// Get the updated workspace
				updatedWorkspace := &workspacev1alpha1.Workspace{}
				err = k8sClient.Get(ctx, workspaceKey, updatedWorkspace)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that the finalizer was added")
				Expect(controllerutil.ContainsFinalizer(updatedWorkspace, WorkspaceFinalizerName)).To(BeTrue(), "Finalizer should be added")

				By("Verifying that both template and access strategy labels were set")
				// Check template labels
				Expect(updatedWorkspace.Labels).To(HaveKey(workspaceutil.LabelWorkspaceTemplate), "Template name label should be set")
				Expect(updatedWorkspace.Labels).To(HaveKey(workspaceutil.LabelWorkspaceTemplateNamespace), "Template namespace label should be set")
				Expect(updatedWorkspace.Labels[workspaceutil.LabelWorkspaceTemplate]).To(Equal(templateName), "Template name label should have correct value")
				Expect(updatedWorkspace.Labels[workspaceutil.LabelWorkspaceTemplateNamespace]).To(Equal(templateNamespace), "Template namespace label should have correct value")

				// Check access strategy labels
				Expect(updatedWorkspace.Labels).To(HaveKey(LabelAccessStrategyName), "Access strategy name label should be set")
				Expect(updatedWorkspace.Labels).To(HaveKey(LabelAccessStrategyNamespace), "Access strategy namespace label should be set")
				Expect(updatedWorkspace.Labels[LabelAccessStrategyName]).To(Equal(accessStrategyName), "Access strategy name label should have correct value")
				Expect(updatedWorkspace.Labels[LabelAccessStrategyNamespace]).To(Equal(accessStrategyNamespace), "Access strategy namespace label should have correct value")
			})

			It("should remove template labels on workspace that no longer references a template", func() {
				By("first updating workspace to add template labels")

				// First get the current workspace
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Add a template reference to ensure labels get added
				existingWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
					Name:      templateName,
					Namespace: templateNamespace,
				}
				existingWorkspace.Spec.DisplayName = "Test Workspace with Template"
				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				// Reconcile to add the labels
				statusManager := StatusManager{client: k8sClient}
				mockStateMachine := &MockStateMachine{
					// Mock to return the accessStrategy when workspace has a matching reference
					getAccessStrategyForWorkspaceFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
						if workspace.Spec.AccessStrategy != nil &&
							workspace.Spec.AccessStrategy.Name == accessStrategyName &&
							workspace.Spec.AccessStrategy.Namespace == accessStrategyNamespace {
							return accessStrategy, nil
						}
						return nil, nil
					},
				}
				controllerReconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: &statusManager,
				}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				// Verify the labels were added
				updatedWorkspaceWithLabels := &workspacev1alpha1.Workspace{}
				err = k8sClient.Get(ctx, workspaceKey, updatedWorkspaceWithLabels)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedWorkspaceWithLabels.Labels).To(HaveKey(workspaceutil.LabelWorkspaceTemplate), "Template name label should be set")

				By("removing the template reference")
				updatedWorkspaceWithLabels.Spec.TemplateRef = nil
				Expect(k8sClient.Update(ctx, updatedWorkspaceWithLabels)).To(Succeed())

				By("reconciling the workspace again")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				// Get the workspace again
				finalWorkspace := &workspacev1alpha1.Workspace{}
				err = k8sClient.Get(ctx, workspaceKey, finalWorkspace)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that the template labels were removed")
				Expect(finalWorkspace.Labels).NotTo(HaveKey(workspaceutil.LabelWorkspaceTemplate), "Template name label should be removed")
				Expect(finalWorkspace.Labels).NotTo(HaveKey(workspaceutil.LabelWorkspaceTemplateNamespace), "Template namespace label should be removed")

				By("Verifying that the finalizer is still present")
				Expect(controllerutil.ContainsFinalizer(finalWorkspace, WorkspaceFinalizerName)).To(BeTrue(), "Finalizer should still be present")
			})

			It("should remove access strategy labels on workspace that no longer references an access strategy", func() {
				By("first updating workspace to add access strategy labels")

				// First get the current workspace
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Add an access strategy reference to ensure labels get added
				existingWorkspace.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
					Name:      accessStrategyName,
					Namespace: accessStrategyNamespace,
				}
				existingWorkspace.Spec.DisplayName = "Test Workspace with AccessStrategy"
				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				// Reconcile to add the labels
				statusManager := StatusManager{client: k8sClient}
				mockStateMachine := &MockStateMachine{
					// Mock to return the accessStrategy when workspace has a matching reference
					getAccessStrategyForWorkspaceFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
						if workspace.Spec.AccessStrategy != nil &&
							workspace.Spec.AccessStrategy.Name == accessStrategyName &&
							workspace.Spec.AccessStrategy.Namespace == accessStrategyNamespace {
							return accessStrategy, nil
						}
						return nil, nil
					},
				}
				controllerReconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: &statusManager,
				}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				// Verify the labels were added
				updatedWorkspaceWithLabels := &workspacev1alpha1.Workspace{}
				err = k8sClient.Get(ctx, workspaceKey, updatedWorkspaceWithLabels)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedWorkspaceWithLabels.Labels).To(HaveKey(LabelAccessStrategyName), "Access strategy name label should be set")

				By("removing the access strategy reference")
				updatedWorkspaceWithLabels.Spec.AccessStrategy = nil
				Expect(k8sClient.Update(ctx, updatedWorkspaceWithLabels)).To(Succeed())

				By("reconciling the workspace again")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				// Get the workspace again
				finalWorkspace := &workspacev1alpha1.Workspace{}
				err = k8sClient.Get(ctx, workspaceKey, finalWorkspace)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that the access strategy labels were removed")
				Expect(finalWorkspace.Labels).NotTo(HaveKey(LabelAccessStrategyName), "Access strategy name label should be removed")
				Expect(finalWorkspace.Labels).NotTo(HaveKey(LabelAccessStrategyNamespace), "Access strategy namespace label should be removed")

				By("Verifying that the finalizer is still present")
				Expect(controllerutil.ContainsFinalizer(finalWorkspace, WorkspaceFinalizerName)).To(BeTrue(), "Finalizer should still be present")
			})

			It("should remove template and access strategy labels on workspace that no longer references either", func() {
				By("first updating workspace to add both template and access strategy labels")

				// First get the current workspace
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Add both template and access strategy references
				existingWorkspace.Spec.TemplateRef = &workspacev1alpha1.TemplateRef{
					Name:      templateName,
					Namespace: templateNamespace,
				}
				existingWorkspace.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
					Name:      accessStrategyName,
					Namespace: accessStrategyNamespace,
				}
				existingWorkspace.Spec.DisplayName = "Test Workspace with Template and AccessStrategy"
				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				// Reconcile to add the labels
				statusManager := StatusManager{client: k8sClient}
				mockStateMachine := &MockStateMachine{
					// Mock to return the accessStrategy when workspace has a matching reference
					getAccessStrategyForWorkspaceFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
						if workspace.Spec.AccessStrategy != nil &&
							workspace.Spec.AccessStrategy.Name == accessStrategyName &&
							workspace.Spec.AccessStrategy.Namespace == accessStrategyNamespace {
							return accessStrategy, nil
						}
						return nil, nil
					},
				}
				controllerReconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: &statusManager,
				}

				_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				// Verify the labels were added
				updatedWorkspaceWithLabels := &workspacev1alpha1.Workspace{}
				err = k8sClient.Get(ctx, workspaceKey, updatedWorkspaceWithLabels)
				Expect(err).NotTo(HaveOccurred())
				Expect(updatedWorkspaceWithLabels.Labels).To(HaveKey(workspaceutil.LabelWorkspaceTemplate), "Template name label should be set")
				Expect(updatedWorkspaceWithLabels.Labels).To(HaveKey(LabelAccessStrategyName), "Access strategy name label should be set")

				By("removing both template and access strategy references")
				updatedWorkspaceWithLabels.Spec.TemplateRef = nil
				updatedWorkspaceWithLabels.Spec.AccessStrategy = nil
				Expect(k8sClient.Update(ctx, updatedWorkspaceWithLabels)).To(Succeed())

				By("reconciling the workspace again")
				_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				// Get the workspace again
				finalWorkspace := &workspacev1alpha1.Workspace{}
				err = k8sClient.Get(ctx, workspaceKey, finalWorkspace)
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that both template and access strategy labels were removed")
				// Check template labels are removed
				Expect(finalWorkspace.Labels).NotTo(HaveKey(workspaceutil.LabelWorkspaceTemplate), "Template name label should be removed")
				Expect(finalWorkspace.Labels).NotTo(HaveKey(workspaceutil.LabelWorkspaceTemplateNamespace), "Template namespace label should be removed")

				// Check access strategy labels are removed
				Expect(finalWorkspace.Labels).NotTo(HaveKey(LabelAccessStrategyName), "Access strategy name label should be removed")
				Expect(finalWorkspace.Labels).NotTo(HaveKey(LabelAccessStrategyNamespace), "Access strategy namespace label should be removed")

				By("Verifying that the finalizer is still present")
				Expect(controllerutil.ContainsFinalizer(finalWorkspace, WorkspaceFinalizerName)).To(BeTrue(), "Finalizer should still be present")
			})
		})

		Context("When reconcialing a non-deleting Workspace with correct finalizers and labels", func() {
			It("should call ReconcileDesiredState() and pass the workspace", func() {
				By("Creating a workspace with finalizer already set")
				// Create a workspace and update it to have the finalizer and running state
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Add finalizer and set desired status to Running
				controllerutil.AddFinalizer(existingWorkspace, WorkspaceFinalizerName)
				existingWorkspace.Spec.DesiredStatus = DesiredStateRunning
				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				By("Setting up mock StateMachine to verify ReconcileDesiredState call")
				desiredStatusCalled := false
				reconcileDesiredStateCalled := false
				var calledWithWorkspace *workspacev1alpha1.Workspace

				mockStateMachine := &MockStateMachine{
					// Track calls to getDesiredStatus
					getDesiredStatusFunc: func(workspace *workspacev1alpha1.Workspace) string {
						desiredStatusCalled = true
						return workspace.Spec.DesiredStatus
					},

					// Track calls to ReconcileDesiredState and return a requeue
					reconcileDesiredStateFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
						accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
					) (ctrl.Result, error) {
						reconcileDesiredStateCalled = true
						calledWithWorkspace = workspace
						// No need to track accessStrategy for this test
						// Return a requeue after 30 seconds
						return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
					},
				}

				statusManager := StatusManager{client: k8sClient}
				reconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: &statusManager,
				}

				By("Reconciling the workspace")
				result, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(result.RequeueAfter).To(Equal(30*time.Second), "Requeue time should be passed through from state machine")

				By("Verifying ReconcileDesiredState was called with correct parameters")
				Expect(desiredStatusCalled).To(BeTrue(), "getDesiredStatus should be called")
				Expect(reconcileDesiredStateCalled).To(BeTrue(), "ReconcileDesiredState should be called")
				Expect(calledWithWorkspace).NotTo(BeNil(), "Workspace should not be nil")
				Expect(calledWithWorkspace.Name).To(Equal(workspaceName), "Should be called with correct workspace")
				Expect(calledWithWorkspace.Spec.DesiredStatus).To(Equal(DesiredStateRunning), "Should be called with Running status")
			})

			It("should return an error if ReconcileDesiredState() returns an error", func() {
				By("Creating a workspace with finalizer and labels already set")
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Add finalizer and set desired status to Running
				controllerutil.AddFinalizer(existingWorkspace, WorkspaceFinalizerName)
				existingWorkspace.Spec.DesiredStatus = DesiredStateRunning

				// Initialize labels map if needed
				if existingWorkspace.Labels == nil {
					existingWorkspace.Labels = make(map[string]string)
				}
				// Make sure labels are properly set to avoid early returns in the controller
				existingWorkspace.Labels["test-label"] = "test-value"

				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				By("Setting up mock StateMachine with complete behavior definition")
				mockStateMachine := &MockStateMachine{
					// Default desired status
					getDesiredStatusFunc: func(workspace *workspacev1alpha1.Workspace) string {
						return workspace.Spec.DesiredStatus
					},

					// Default GetAccessStrategyForWorkspace implementation
					getAccessStrategyForWorkspaceFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
						// Return the test accessStrategy for consistency
						return accessStrategy, nil
					},

					// Return an error from ReconcileDesiredState - this is what we're testing
					reconcileDesiredStateFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
						accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
					) (ctrl.Result, error) {
						return ctrl.Result{}, fmt.Errorf("test error from ReconcileDesiredState")
					},
				}

				// Create a reconciler and set our mock on it
				statusManager := &StatusManager{client: k8sClient}
				reconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: statusManager,
				}

				By("Reconciling the workspace")
				result, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})

				By("Verifying that the error was returned")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("test error from ReconcileDesiredState"))

				// Should return empty result on error (controller-runtime handles requeuing)
				Expect(result).To(Equal(ctrl.Result{}), "Should return empty result on error")
			})

			It("should call GetAccessStrategyForWorkspace() and pass it to ReconcileDesiredState() for running Workspace", func() {
				By("Creating a workspace with finalizer, labels, and AccessStrategy reference")
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Add finalizer, set desired status to Running, and add AccessStrategy reference
				controllerutil.AddFinalizer(existingWorkspace, WorkspaceFinalizerName)
				existingWorkspace.Spec.DesiredStatus = DesiredStateRunning
				existingWorkspace.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
					Name:      accessStrategyName,
					Namespace: accessStrategyNamespace,
				}

				// Initialize labels map if needed
				if existingWorkspace.Labels == nil {
					existingWorkspace.Labels = make(map[string]string)
				}

				// Set the access strategy labels directly to avoid early return
				existingWorkspace.Labels[LabelAccessStrategyName] = accessStrategyName
				existingWorkspace.Labels[LabelAccessStrategyNamespace] = accessStrategyNamespace

				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				By("Setting up mock StateMachine to track GetAccessStrategyForWorkspace and ReconcileDesiredState calls")
				getAccessStrategyForWorkspaceCalled := false
				reconcileDesiredStateCalled := false
				var calledWithWorkspace *workspacev1alpha1.Workspace
				var calledWithAccessStrategy *workspacev1alpha1.WorkspaceAccessStrategy

				mockStateMachine := &MockStateMachine{
					// Default desired status from the workspace
					getDesiredStatusFunc: func(workspace *workspacev1alpha1.Workspace) string {
						return workspace.Spec.DesiredStatus
					},

					// Track GetAccessStrategyForWorkspace call
					getAccessStrategyForWorkspaceFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
						getAccessStrategyForWorkspaceCalled = true
						return accessStrategy, nil
					},

					// Track ReconcileDesiredState call and parameters
					reconcileDesiredStateFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
						accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
					) (ctrl.Result, error) {
						reconcileDesiredStateCalled = true
						calledWithWorkspace = workspace
						calledWithAccessStrategy = accessStrategy
						return ctrl.Result{}, nil
					},
				}

				// Create a reconciler and set our mock on it
				statusManager := &StatusManager{client: k8sClient}
				reconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: statusManager,
				}

				By("Reconciling the workspace")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that both GetAccessStrategyForWorkspace and ReconcileDesiredState were called")
				Expect(getAccessStrategyForWorkspaceCalled).To(BeTrue(), "GetAccessStrategyForWorkspace should be called for running workspace")
				Expect(reconcileDesiredStateCalled).To(BeTrue(), "ReconcileDesiredState should be called")
				Expect(calledWithWorkspace).NotTo(BeNil(), "Workspace should be passed to ReconcileDesiredState")
				Expect(calledWithAccessStrategy).NotTo(BeNil(), "AccessStrategy should be passed to ReconcileDesiredState")
				Expect(calledWithAccessStrategy).To(Equal(accessStrategy), "Correct AccessStrategy should be passed")
			})

			It("should not call GetAccessStrategyForWorkspace() for stopped Workspace", func() {
				By("Creating a workspace with finalizer, labels, and Stopped desired state")
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Add finalizer and set desired status to Stopped
				// Even with an AccessStrategy reference, it should not be fetched for Stopped workspaces
				controllerutil.AddFinalizer(existingWorkspace, WorkspaceFinalizerName)
				existingWorkspace.Spec.DesiredStatus = DesiredStateStopped
				existingWorkspace.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
					Name:      accessStrategyName,
					Namespace: accessStrategyNamespace,
				}

				// Initialize labels map if needed
				if existingWorkspace.Labels == nil {
					existingWorkspace.Labels = make(map[string]string)
				}

				// Set the access strategy labels directly to avoid early return
				existingWorkspace.Labels[LabelAccessStrategyName] = accessStrategyName
				existingWorkspace.Labels[LabelAccessStrategyNamespace] = accessStrategyNamespace

				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				By("Setting up mock StateMachine to track GetAccessStrategyForWorkspace and ReconcileDesiredState calls")
				getAccessStrategyForWorkspaceCalled := false
				reconcileDesiredStateCalled := false
				var calledWithAccessStrategy *workspacev1alpha1.WorkspaceAccessStrategy

				mockStateMachine := &MockStateMachine{
					// Get the desired status from the workspace (which is Stopped)
					getDesiredStatusFunc: func(workspace *workspacev1alpha1.Workspace) string {
						return workspace.Spec.DesiredStatus
					},

					// Track GetAccessStrategyForWorkspace call - should NOT be called
					getAccessStrategyForWorkspaceFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
						getAccessStrategyForWorkspaceCalled = true
						return accessStrategy, nil
					},

					// Track ReconcileDesiredState call and parameters
					reconcileDesiredStateFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
						accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
					) (ctrl.Result, error) {
						reconcileDesiredStateCalled = true
						calledWithAccessStrategy = accessStrategy
						return ctrl.Result{}, nil
					},
				}

				// Create a reconciler and set our mock on it
				statusManager := &StatusManager{client: k8sClient}
				reconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: statusManager,
				}

				By("Reconciling the workspace")
				_, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that GetAccessStrategyForWorkspace was not called and ReconcileDesiredState was called with nil access strategy")
				Expect(getAccessStrategyForWorkspaceCalled).To(BeFalse(), "GetAccessStrategyForWorkspace should NOT be called for stopped workspace")
				Expect(reconcileDesiredStateCalled).To(BeTrue(), "ReconcileDesiredState should be called")
				Expect(calledWithAccessStrategy).To(BeNil(), "AccessStrategy should be nil for stopped workspace")
			})

			It("should return an error and requeue when GetAccessStrategyForWorkspace() fails", func() {
				By("Creating a workspace with finalizer, labels, and Running desired state")
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Add finalizer, set desired status to Running, and add AccessStrategy reference
				controllerutil.AddFinalizer(existingWorkspace, WorkspaceFinalizerName)
				existingWorkspace.Spec.DesiredStatus = DesiredStateRunning
				existingWorkspace.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
					Name:      accessStrategyName,
					Namespace: accessStrategyNamespace,
				}

				// Initialize labels map if needed
				if existingWorkspace.Labels == nil {
					existingWorkspace.Labels = make(map[string]string)
				}

				// Set the access strategy labels directly to avoid early return
				existingWorkspace.Labels[LabelAccessStrategyName] = accessStrategyName
				existingWorkspace.Labels[LabelAccessStrategyNamespace] = accessStrategyNamespace

				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				By("Setting up mock StateMachine to return error from GetAccessStrategyForWorkspace")
				reconcileDesiredStateCalled := false

				mockStateMachine := &MockStateMachine{
					// Get desired status from the workspace (which is Running)
					getDesiredStatusFunc: func(workspace *workspacev1alpha1.Workspace) string {
						return workspace.Spec.DesiredStatus
					},

					// Return error from GetAccessStrategyForWorkspace - this is what we're testing
					getAccessStrategyForWorkspaceFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
						return nil, fmt.Errorf("test error from GetAccessStrategyForWorkspace")
					},

					// This should not be called due to the error above
					reconcileDesiredStateFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
						accessStrategy *workspacev1alpha1.WorkspaceAccessStrategy,
					) (ctrl.Result, error) {
						reconcileDesiredStateCalled = true
						return ctrl.Result{}, nil
					},
				}

				// Create a reconciler and set our mock on it
				statusManager := &StatusManager{client: k8sClient}
				reconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: statusManager,
				}

				By("Reconciling the workspace")
				result, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})

				By("Verifying that the error was returned")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("test error from GetAccessStrategyForWorkspace"))
				Expect(reconcileDesiredStateCalled).To(BeFalse(), "ReconcileDesiredState should not be called when GetAccessStrategyForWorkspace fails")

				// Should return empty result on error (controller-runtime handles requeuing)
				Expect(result).To(Equal(ctrl.Result{}), "Should return empty result on error")
			})
		})

		Context("When reconcialing a deleting workspace", func() {
			It("should call ReconcileDeletion() without adding labels, finalizers or fetching the AccessStrategy", func() {
				By("Creating a workspace with AccessStrategy reference and setting DeletionTimestamp")
				// First get the current workspace
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Add finalizer so deletion doesn't complete immediately
				controllerutil.AddFinalizer(existingWorkspace, WorkspaceFinalizerName)

				// Add AccessStrategy reference to make the test more realistic
				existingWorkspace.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
					Name:      accessStrategyName,
					Namespace: accessStrategyNamespace,
				}
				existingWorkspace.Spec.DesiredStatus = DesiredStateRunning

				// Initialize labels map if needed
				if existingWorkspace.Labels == nil {
					existingWorkspace.Labels = make(map[string]string)
				}

				// Set the access strategy labels to match the reference
				existingWorkspace.Labels[LabelAccessStrategyName] = accessStrategyName
				existingWorkspace.Labels[LabelAccessStrategyNamespace] = accessStrategyNamespace

				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				// Mark for deletion
				Expect(k8sClient.Delete(ctx, existingWorkspace)).To(Succeed())

				// Get the updated workspace with deletion timestamp
				deletingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, deletingWorkspace)).To(Succeed())

				// Verify deletion timestamp is set
				Expect(deletingWorkspace.DeletionTimestamp.IsZero()).To(BeFalse(), "DeletionTimestamp should be set")

				By("Setting up mock StateMachine to track ReconcileDeletion call")
				reconcileDeletionCalled := false
				getAccessStrategyForWorkspaceCalled := false
				var calledWithWorkspace *workspacev1alpha1.Workspace
				expectedResult := ctrl.Result{RequeueAfter: 15 * time.Second}

				mockStateMachine := &MockStateMachine{
					// Track if GetAccessStrategyForWorkspace is called - should NOT be called
					getAccessStrategyForWorkspaceFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (*workspacev1alpha1.WorkspaceAccessStrategy, error) {
						getAccessStrategyForWorkspaceCalled = true
						return accessStrategy, nil
					},

					// Track ReconcileDeletion call and parameters
					reconcileDeletionFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (ctrl.Result, error) {
						reconcileDeletionCalled = true
						calledWithWorkspace = workspace
						// Return a specific result to verify it's passed through
						return expectedResult, nil
					},
				}

				statusManager := &StatusManager{client: k8sClient}
				reconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: statusManager,
				}

				By("Reconciling the workspace")
				result, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})
				Expect(err).NotTo(HaveOccurred())

				By("Verifying that ReconcileDeletion was called with correct parameters")
				Expect(reconcileDeletionCalled).To(BeTrue(), "ReconcileDeletion should be called")
				Expect(getAccessStrategyForWorkspaceCalled).To(BeFalse(), "GetAccessStrategyForWorkspace should NOT be called even though workspace has AccessStrategy reference")
				Expect(calledWithWorkspace).NotTo(BeNil(), "Workspace should not be nil")
				Expect(calledWithWorkspace.Name).To(Equal(workspaceName), "Should be called with correct workspace")
				Expect(calledWithWorkspace.DeletionTimestamp.IsZero()).To(BeFalse(), "Should be called with workspace that has DeletionTimestamp")

				// Verify the workspace still has its AccessStrategy reference, but it wasn't used
				Expect(calledWithWorkspace.Spec.AccessStrategy).NotTo(BeNil(), "Workspace should still have AccessStrategy reference")
				Expect(calledWithWorkspace.Spec.AccessStrategy.Name).To(Equal(accessStrategyName), "Workspace should have correct AccessStrategy name")
				Expect(calledWithWorkspace.Spec.AccessStrategy.Namespace).To(Equal(accessStrategyNamespace), "Workspace should have correct AccessStrategy namespace")

				By("Verifying that the result from ReconcileDeletion is returned")
				Expect(result).To(Equal(expectedResult), "Result should be passed through from ReconcileDeletion")
			})

			It("should return an error if ReconcileDeletion() returns an error", func() {
				By("Creating a workspace with AccessStrategy reference and setting DeletionTimestamp")
				// First get the current workspace
				existingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, existingWorkspace)).To(Succeed())

				// Add finalizer so deletion doesn't complete immediately
				controllerutil.AddFinalizer(existingWorkspace, WorkspaceFinalizerName)

				// Add AccessStrategy reference to make the test more realistic
				existingWorkspace.Spec.AccessStrategy = &workspacev1alpha1.AccessStrategyRef{
					Name:      accessStrategyName,
					Namespace: accessStrategyNamespace,
				}
				existingWorkspace.Spec.DesiredStatus = DesiredStateRunning

				// Initialize labels map if needed
				if existingWorkspace.Labels == nil {
					existingWorkspace.Labels = make(map[string]string)
				}

				// Set the access strategy labels to match the reference
				existingWorkspace.Labels[LabelAccessStrategyName] = accessStrategyName
				existingWorkspace.Labels[LabelAccessStrategyNamespace] = accessStrategyNamespace

				Expect(k8sClient.Update(ctx, existingWorkspace)).To(Succeed())

				// Mark for deletion
				Expect(k8sClient.Delete(ctx, existingWorkspace)).To(Succeed())

				// Get the updated workspace with deletion timestamp
				deletingWorkspace := &workspacev1alpha1.Workspace{}
				Expect(k8sClient.Get(ctx, workspaceKey, deletingWorkspace)).To(Succeed())

				By("Setting up mock StateMachine to return an error and a requeue from ReconcileDeletion")
				expectedError := fmt.Errorf("test error from ReconcileDeletion")
				// Create a specific requeue result that should be returned with the error
				expectedResult := ctrl.Result{RequeueAfter: 45 * time.Second}

				mockStateMachine := &MockStateMachine{
					reconcileDeletionFunc: func(
						ctx context.Context,
						workspace *workspacev1alpha1.Workspace,
					) (ctrl.Result, error) {
						// Return both a non-empty Result and an error
						return expectedResult, expectedError
					},
				}

				statusManager := &StatusManager{client: k8sClient}
				reconciler := &WorkspaceReconciler{
					Client:        k8sClient,
					Scheme:        k8sClient.Scheme(),
					stateMachine:  mockStateMachine,
					statusManager: statusManager,
				}

				By("Reconciling the workspace")
				result, err := reconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: workspaceKey,
				})

				By("Verifying that both the error and the requeue result were returned")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("test error from ReconcileDeletion"))

				// We should get the exact requeue result from the StateMachine
				// instead of an empty Result even when there's an error
				Expect(result).To(Equal(expectedResult), "Should return the result from ReconcileDeletion even when there's an error")
			})
		})
	})
})
