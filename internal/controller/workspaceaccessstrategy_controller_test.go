package controller

import (
	"context"
	"fmt"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/workspace"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("AccessStrategy controller", func() {
	Context("Finalizer management", func() {
		var (
			ctx               context.Context
			mockClient        *MockClient
			reconciler        *WorkspaceAccessStrategyReconciler
			accessStrategy    *workspacev1alpha1.WorkspaceAccessStrategy
			accessStrategyKey types.NamespacedName
			fakeRecorder      *FakeEventRecorder
			scheme            *runtime.Scheme
		)

		BeforeEach(func() {
			ctx = context.Background()

			scheme = runtime.NewScheme()
			Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())

			// Add Status to the scheme - no need to check error as AddKnownTypes doesn't return one
			gv := schema.GroupVersion{Group: "", Version: "v1"}
			scheme.AddKnownTypes(gv, &metav1.Status{})

			mockClient = &MockClient{
				scheme: scheme,
			}

			fakeRecorder = &FakeEventRecorder{Events: []string{}}

			reconciler = &WorkspaceAccessStrategyReconciler{
				Client:        mockClient,
				Scheme:        scheme,
				EventRecorder: fakeRecorder,
			}

			accessStrategy = &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-accessstrategy",
					Namespace: "test-namespace",
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName: "Test AccessStrategy",
				},
			}

			accessStrategyKey = types.NamespacedName{
				Name:      accessStrategy.Name,
				Namespace: accessStrategy.Namespace,
			}
		})

		It("Should fetch the access strategy", func() {
			getCalled := false
			listCalled := false

			// Set up mock client behavior
			mockClient.getFunc = func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				getCalled = true
				if key.Name == accessStrategy.Name && key.Namespace == accessStrategy.Namespace {
					accessStrategy.DeepCopyInto(obj.(*workspacev1alpha1.WorkspaceAccessStrategy))
					return nil
				}
				return fmt.Errorf("unexpected key: %v", key)
			}

			// Set up mock client for HasActiveWorkspacesWithAccessStrategy
			mockClient.listFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				// Do nothing - just return empty list
				listCalled = true
				return nil
			}

			// Call Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: accessStrategyKey})

			// Verify results
			Expect(getCalled).To(BeTrue())
			Expect(listCalled).To(BeTrue())
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("Should return empty result when AccessStrategy is not found", func() {
			getCalled := false
			listCalled := false

			// Set up mock client behavior
			mockClient.getFunc = func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				getCalled = true
				return errors.NewNotFound(schema.GroupResource{Group: "workspace.jupyter.org", Resource: "workspaceaccessstrategies"}, key.Name)
			}

			// Set up mock client for HasActiveWorkspacesWithAccessStrategy
			mockClient.listFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				// Do nothing - just return empty list
				listCalled = true
				return nil
			}

			// Call Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: accessStrategyKey})

			// Verify results
			Expect(getCalled).To(BeTrue())
			Expect(listCalled).To(BeFalse())
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("Should return an error and empty result on other errors", func() {
			// Set up mock client behavior
			mockClient.getFunc = func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return fmt.Errorf("internal server error")
			}

			// Call Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: accessStrategyKey})

			// Verify results
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("internal server error"))
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("Should call Update to set the finalizer if missing and one workspace references the AccessStrategy", func() {
			// Set up mock client behavior - AccessStrategy without finalizer
			accessStrategyWithoutFinalizer := accessStrategy.DeepCopy()

			mockClient.getFunc = func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if key.Name == accessStrategy.Name && key.Namespace == accessStrategy.Namespace {
					accessStrategyWithoutFinalizer.DeepCopyInto(obj.(*workspacev1alpha1.WorkspaceAccessStrategy))
					return nil
				}
				return fmt.Errorf("unexpected key: %v", key)
			}

			// Mock hasWorkspaces to return true
			listCalled := false
			mockClient.listFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				listCalled = true

				// Add a workspace to the list
				workspaceList := list.(*workspacev1alpha1.WorkspaceList)
				workspaceList.Items = []workspacev1alpha1.Workspace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-workspace",
							Namespace: "test-namespace",
						},
						Spec: workspacev1alpha1.WorkspaceSpec{
							AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
								Name:      accessStrategy.Name,
								Namespace: accessStrategy.Namespace,
							},
						},
					},
				}
				return nil
			}

			// Mock update to capture the finalizer
			updateCalled := false
			var updatedAccessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
			mockClient.updateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				updateCalled = true
				updatedAccessStrategy = obj.(*workspacev1alpha1.WorkspaceAccessStrategy)
				return nil
			}

			// Call Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: accessStrategyKey})

			// Verify results
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(listCalled).To(BeTrue(), "List should be called to decide whether to add finalizers")
			Expect(updateCalled).To(BeTrue(), "Update should be called to add finalizer")
			Expect(updatedAccessStrategy).NotTo(BeNil())
			Expect(controllerutil.ContainsFinalizer(updatedAccessStrategy, workspace.AccessStrategyFinalizerName)).To(BeTrue(), "Finalizer should be added")
		})

		It("Should call Update to remove the finalizer if present and no workspace references the AccessStrategy", func() {
			// Set up mock client behavior - AccessStrategy with finalizer
			accessStrategyWithFinalizer := accessStrategy.DeepCopy()
			controllerutil.AddFinalizer(accessStrategyWithFinalizer, workspace.AccessStrategyFinalizerName)

			mockClient.getFunc = func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if key.Name == accessStrategy.Name && key.Namespace == accessStrategy.Namespace {
					accessStrategyWithFinalizer.DeepCopyInto(obj.(*workspacev1alpha1.WorkspaceAccessStrategy))
					return nil
				}
				return fmt.Errorf("unexpected key: %v", key)
			}

			// Mock hasWorkspaces to return false (no workspaces)
			mockClient.listFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				// Return empty list
				return nil
			}

			// Mock update to capture the finalizer
			updateCalled := false
			var updatedAccessStrategy *workspacev1alpha1.WorkspaceAccessStrategy
			mockClient.updateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				updateCalled = true
				updatedAccessStrategy = obj.(*workspacev1alpha1.WorkspaceAccessStrategy)
				return nil
			}

			// Call Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: accessStrategyKey})

			// Verify results
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(updateCalled).To(BeTrue(), "Update should be called to remove finalizer")
			Expect(updatedAccessStrategy).NotTo(BeNil())
			Expect(controllerutil.ContainsFinalizer(updatedAccessStrategy, workspace.AccessStrategyFinalizerName)).To(BeFalse(), "Finalizer should be removed")
		})

		It("Should not call Update if finalizer already present and one workspace references the AccessStrategy", func() {
			// Set up mock client behavior - AccessStrategy with finalizer
			accessStrategyWithFinalizer := accessStrategy.DeepCopy()
			controllerutil.AddFinalizer(accessStrategyWithFinalizer, workspace.AccessStrategyFinalizerName)

			mockClient.getFunc = func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if key.Name == accessStrategy.Name && key.Namespace == accessStrategy.Namespace {
					accessStrategyWithFinalizer.DeepCopyInto(obj.(*workspacev1alpha1.WorkspaceAccessStrategy))
					return nil
				}
				return fmt.Errorf("unexpected key: %v", key)
			}

			// Mock hasWorkspaces to return true
			mockClient.listFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				// Add a workspace to the list
				workspaceList := list.(*workspacev1alpha1.WorkspaceList)
				workspaceList.Items = []workspacev1alpha1.Workspace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-workspace",
							Namespace: "test-namespace",
						},
						Spec: workspacev1alpha1.WorkspaceSpec{
							AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
								Name:      accessStrategy.Name,
								Namespace: accessStrategy.Namespace,
							},
						},
					},
				}
				return nil
			}

			// Mock update to verify it's not called
			updateCalled := false
			mockClient.updateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				updateCalled = true
				return nil
			}

			// Call Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: accessStrategyKey})

			// Verify results
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(updateCalled).To(BeFalse(), "Update should not be called when finalizer already present")
		})

		It("Should not call Update if finalizer not present and no workspace references the AccessStrategy", func() {
			// Set up mock client behavior - AccessStrategy without finalizer
			accessStrategyWithoutFinalizer := accessStrategy.DeepCopy()

			mockClient.getFunc = func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if key.Name == accessStrategy.Name && key.Namespace == accessStrategy.Namespace {
					accessStrategyWithoutFinalizer.DeepCopyInto(obj.(*workspacev1alpha1.WorkspaceAccessStrategy))
					return nil
				}
				return fmt.Errorf("unexpected key: %v", key)
			}

			// Mock hasWorkspaces to return false (no workspaces)
			mockClient.listFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				// Return empty list
				return nil
			}

			// Mock update to verify it's not called
			updateCalled := false
			mockClient.updateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				updateCalled = true
				return nil
			}

			// Call Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: accessStrategyKey})

			// Verify results
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
			Expect(updateCalled).To(BeFalse(), "Update should not be called when no finalizer and no workspaces")
		})

		It("Should return an error if fetching the workspaces referencing the AccessStrategy fails", func() {
			// Set up mock client behavior
			mockClient.getFunc = func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if key.Name == accessStrategy.Name && key.Namespace == accessStrategy.Namespace {
					accessStrategy.DeepCopyInto(obj.(*workspacev1alpha1.WorkspaceAccessStrategy))
					return nil
				}
				return fmt.Errorf("unexpected key: %v", key)
			}

			// Mock list to return an error
			mockClient.listFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				return fmt.Errorf("failed to list workspaces")
			}

			// Call Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: accessStrategyKey})

			// Verify results
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to list workspaces"))
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("Should return an error if Updating the AccessStrategy to add the finalizer fails", func() {
			// Set up mock client behavior - AccessStrategy without finalizer
			accessStrategyWithoutFinalizer := accessStrategy.DeepCopy()

			mockClient.getFunc = func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if key.Name == accessStrategy.Name && key.Namespace == accessStrategy.Namespace {
					accessStrategyWithoutFinalizer.DeepCopyInto(obj.(*workspacev1alpha1.WorkspaceAccessStrategy))
					return nil
				}
				return fmt.Errorf("unexpected key: %v", key)
			}

			// Mock hasWorkspaces to return true
			mockClient.listFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				// Add a workspace to the list
				workspaceList := list.(*workspacev1alpha1.WorkspaceList)
				workspaceList.Items = []workspacev1alpha1.Workspace{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-workspace",
							Namespace: "test-namespace",
						},
						Spec: workspacev1alpha1.WorkspaceSpec{
							AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
								Name:      accessStrategy.Name,
								Namespace: accessStrategy.Namespace,
							},
						},
					},
				}
				return nil
			}

			// Mock update to return an error
			mockClient.updateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				return fmt.Errorf("update failed")
			}

			// Call Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: accessStrategyKey})

			// Verify results
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("update failed"))
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("Should return an error if Updating the AccessStrategy to remove the finalizer fails", func() {
			// Set up mock client behavior - AccessStrategy with finalizer
			accessStrategyWithFinalizer := accessStrategy.DeepCopy()
			controllerutil.AddFinalizer(accessStrategyWithFinalizer, workspace.AccessStrategyFinalizerName)

			mockClient.getFunc = func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if key.Name == accessStrategy.Name && key.Namespace == accessStrategy.Namespace {
					accessStrategyWithFinalizer.DeepCopyInto(obj.(*workspacev1alpha1.WorkspaceAccessStrategy))
					return nil
				}
				return fmt.Errorf("unexpected key: %v", key)
			}

			// Mock hasWorkspaces to return false
			mockClient.listFunc = func(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
				return nil // Return empty list
			}

			// Mock update to return an error
			mockClient.updateFunc = func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
				return fmt.Errorf("update failed")
			}

			// Call Reconcile
			result, err := reconciler.Reconcile(ctx, reconcile.Request{NamespacedName: accessStrategyKey})

			// Verify results
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("update failed"))
			Expect(result).To(Equal(ctrl.Result{}))
		})
	})

	Context("findAccessStrategiesForWorkspace", func() {
		var (
			ctx        context.Context
			reconciler *WorkspaceAccessStrategyReconciler
		)

		BeforeEach(func() {
			ctx = context.Background()
			reconciler = &WorkspaceAccessStrategyReconciler{
				Client: &MockClient{},
			}
		})

		It("should return nil if not pass a Workspace", func() {
			// Create a non-workspace object
			notWorkspace := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "not-a-workspace",
					Namespace: "test-namespace",
				},
			}

			// Call the function
			result := reconciler.findAccessStrategiesForWorkspace(ctx, notWorkspace)

			// Verify result
			Expect(result).To(BeEmpty())
		})

		It("should return the correct AccessStrategy reconcile request if the label are present on the Workspace", func() {
			// Create a workspace with access strategy labels
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "test-namespace",
					Labels: map[string]string{
						workspace.LabelAccessStrategyName:      "test-strategy",
						workspace.LabelAccessStrategyNamespace: "strategy-namespace",
					},
				},
			}

			// Call the function
			result := reconciler.findAccessStrategiesForWorkspace(ctx, workspace)

			// Verify result
			Expect(result).To(HaveLen(1))
			Expect(result[0].Name).To(Equal("test-strategy"))
			Expect(result[0].Namespace).To(Equal("strategy-namespace"))
		})

		It("should return nil if the label of the AccessStrategy name is missing on the Workspace", func() {
			// Create a workspace with missing name label
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "test-namespace",
					Labels: map[string]string{
						// Missing LabelAccessStrategyName
						workspace.LabelAccessStrategyNamespace: "strategy-namespace",
					},
				},
			}

			// Call the function
			result := reconciler.findAccessStrategiesForWorkspace(ctx, workspace)

			// Verify result
			Expect(result).To(BeEmpty())
		})

		It("should return nil if the label of the AccessStrategy namespace is missing on the Workspace", func() {
			// Create a workspace with missing namespace label
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "test-namespace",
					Labels: map[string]string{
						workspace.LabelAccessStrategyName: "test-strategy",
						// Missing LabelAccessStrategyNamespace
					},
				},
			}

			// Call the function
			result := reconciler.findAccessStrategiesForWorkspace(ctx, workspace)

			// Verify result
			Expect(result).To(BeEmpty())
		})
	})

})
