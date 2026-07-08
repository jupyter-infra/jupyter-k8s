/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/jupyter-infra/jupyter-k8s/internal/controller"
	workspaceutil "github.com/jupyter-infra/jupyter-k8s/internal/workspace"
)

// Mock client implementations to help with testing
type fakeClientWithError struct {
	client.Client
	getFunc func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
}

func (f *fakeClientWithError) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return f.getFunc(ctx, key, obj, opts...)
}

type fakeClientWithUpdateSpy struct {
	client.Client
	updateFunc func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
}

func (f *fakeClientWithUpdateSpy) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return f.updateFunc(ctx, obj, opts...)
}

type fakeClientWithUpdateError struct {
	client.Client
	updateFunc func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
}

func (f *fakeClientWithUpdateError) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return f.updateFunc(ctx, obj, opts...)
}

// Combined spy for both Get and Update operations
type fakeCombinedClientSpy struct {
	client.Client
	getFunc    func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error
	updateFunc func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error
}

func (f *fakeCombinedClientSpy) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	return f.getFunc(ctx, key, obj, opts...)
}

func (f *fakeCombinedClientSpy) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return f.updateFunc(ctx, obj, opts...)
}

// Tests are integrated into the main Webhook Suite via webhook_suite_test.go

var _ = Describe("Lazy Finalizer Logic", func() {
	var (
		ctx       context.Context
		scheme    *runtime.Scheme
		k8sClient client.Client
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(workspacev1alpha1.AddToScheme(scheme)).To(Succeed())
		Expect(corev1.AddToScheme(scheme)).To(Succeed())
	})

	Context("ensureTemplateFinalizer", func() {
		It("should not add finalizer when no active workspaces exist", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testTemplateName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  testTemplateDisplayName,
					DefaultImage: testImageTestLatest,
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template).
				Build()

			err := ensureTemplateFinalizer(ctx, k8sClient, testTemplateName, testDefaultNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was NOT added
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: testTemplateName, Namespace: testDefaultNamespace}, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeFalse())
		})

		It("should add finalizer when active workspace exists", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testTemplateName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  testTemplateDisplayName,
					DefaultImage: testImageTestLatest,
				},
			}

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
					Labels: map[string]string{
						controller.LabelWorkspaceTemplate:          testTemplateName,
						controller.LabelWorkspaceTemplateNamespace: testDefaultNamespace,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: testWorkspaceDisplayName,
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testTemplateName,
						Namespace: testDefaultNamespace,
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template, workspace).
				Build()

			err := ensureTemplateFinalizer(ctx, k8sClient, testTemplateName, testDefaultNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer WAS added
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: testTemplateName, Namespace: testDefaultNamespace}, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeTrue())
		})

		It("should not add finalizer when all workspaces have DeletionTimestamp", func() {
			now := metav1.Now()
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testTemplateName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  testTemplateDisplayName,
					DefaultImage: testImageTestLatest,
				},
			}

			// Per K8s semantics, objects with DeletionTimestamp must have finalizers
			// This represents a workspace that is being deleted but blocked by finalizers
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:              testWorkspaceName,
					Namespace:         testDefaultNamespace,
					DeletionTimestamp: &now,
					Finalizers:        []string{"test-finalizer"}, // Required to set DeletionTimestamp
					Labels: map[string]string{
						controller.LabelWorkspaceTemplate:          testTemplateName,
						controller.LabelWorkspaceTemplateNamespace: testDefaultNamespace,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: testWorkspaceDisplayName,
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testTemplateName,
						Namespace: testDefaultNamespace,
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template, workspace).
				Build()

			err := ensureTemplateFinalizer(ctx, k8sClient, testTemplateName, testDefaultNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was NOT added (workspace is being deleted)
			// ListActiveWorkspacesByTemplate filters out workspaces with DeletionTimestamp
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: testTemplateName, Namespace: testDefaultNamespace}, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeFalse())
		})

		It("should handle namespace filtering correctly", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testTemplateName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  testTemplateDisplayName,
					DefaultImage: testImageTestLatest,
				},
			}

			// Workspace in different namespace
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
					Labels: map[string]string{
						controller.LabelWorkspaceTemplate:          testTemplateName,
						controller.LabelWorkspaceTemplateNamespace: "other-namespace",
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: testWorkspaceDisplayName,
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testTemplateName,
						Namespace: "other-namespace",
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template, workspace).
				Build()

			// Query for template in "default" namespace - should not find workspace
			err := ensureTemplateFinalizer(ctx, k8sClient, testTemplateName, testDefaultNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was NOT added (workspace references different namespace)
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: testTemplateName, Namespace: testDefaultNamespace}, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeFalse())
		})

		It("should skip finalizer addition when template does not exist", func() {
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				Build()

			// Should not error when template doesn't exist
			err := ensureTemplateFinalizer(ctx, k8sClient, "nonexistent-template", testDefaultNamespace)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should not re-add finalizer if already present", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:       testTemplateName,
					Namespace:  testDefaultNamespace,
					Finalizers: []string{workspaceutil.TemplateFinalizerName},
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  testTemplateDisplayName,
					DefaultImage: testImageTestLatest,
				},
			}

			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
					Labels: map[string]string{
						controller.LabelWorkspaceTemplate:          testTemplateName,
						controller.LabelWorkspaceTemplateNamespace: testDefaultNamespace,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: testWorkspaceDisplayName,
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testTemplateName,
						Namespace: testDefaultNamespace,
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template, workspace).
				Build()

			err := ensureTemplateFinalizer(ctx, k8sClient, testTemplateName, testDefaultNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer still present (only one)
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: testTemplateName, Namespace: testDefaultNamespace}, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeTrue())
			Expect(updatedTemplate.Finalizers).To(HaveLen(1))
		})

		It("should use limit=1 optimization by returning early", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testTemplateName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  testTemplateDisplayName,
					DefaultImage: testImageTestLatest,
				},
			}

			// Create multiple workspaces
			workspace1 := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-1",
					Namespace: testDefaultNamespace,
					Labels: map[string]string{
						controller.LabelWorkspaceTemplate:          testTemplateName,
						controller.LabelWorkspaceTemplateNamespace: testDefaultNamespace,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace 1",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testTemplateName,
						Namespace: testDefaultNamespace,
					},
				},
			}

			workspace2 := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace-2",
					Namespace: testDefaultNamespace,
					Labels: map[string]string{
						controller.LabelWorkspaceTemplate:          testTemplateName,
						controller.LabelWorkspaceTemplateNamespace: testDefaultNamespace,
					},
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace 2",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testTemplateName,
						Namespace: testDefaultNamespace,
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template, workspace1, workspace2).
				Build()

			// Should add finalizer after finding first workspace (limit=1 optimization)
			err := ensureTemplateFinalizer(ctx, k8sClient, testTemplateName, testDefaultNamespace)
			Expect(err).NotTo(HaveOccurred())

			// Verify finalizer was added
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			err = k8sClient.Get(ctx, client.ObjectKey{Name: testTemplateName, Namespace: testDefaultNamespace}, updatedTemplate)
			Expect(err).NotTo(HaveOccurred())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeTrue())
		})
	})

	Context("ensureAccessStrategyFinalizer", func() {
		It("Should be a no-op if the Workspace does not reference an access strategy", func() {
			// Create a workspace without an access strategy reference
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: testWorkspaceDisplayName,
					// No AccessStrategy field
				},
			}

			// Track if Get is called
			getCalled := false

			// Use fake client with a spy for Get
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace).
				Build()

			// Patch the Get method to track calls
			originalGet := k8sClient.Get
			k8sClient = &fakeClientWithError{
				Client: k8sClient,
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					// Check if this is a Get for an AccessStrategy
					getCalled = true
					return originalGet(ctx, key, obj, opts...)
				},
			}

			// Call the function
			err := ensureAccessStrategyFinalizer(ctx, k8sClient, workspace)

			// Verify no error occurred
			Expect(err).NotTo(HaveOccurred())

			// Verify Get(AccessStrategy) was not called
			Expect(getCalled).To(BeFalse(), "Get(AccessStrategy) should not be called when Workspace has no AccessStrategy reference")
		})

		It("Should be a no-op if the Workspace is being deleted", func() {
			// Create a deletion timestamp for the test
			now := metav1.Now()

			// Create a workspace that is being deleted
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:              testWorkspaceName,
					Namespace:         testDefaultNamespace,
					DeletionTimestamp: &now,
					Finalizers:        []string{"test-finalizer"}, // Required to set DeletionTimestamp
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: testWorkspaceDisplayName,
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testStrategyName,
						Namespace: testDefaultNamespace,
					},
				},
			}

			// Track if Get and Update are called
			getCalled := false
			updateCalled := false

			// Create a client
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace).
				Build()

			// Create an access strategy - to make sure it exists but won't be fetched
			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testStrategyName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName: testStrategyDisplayName,
				},
			}

			// Add the access strategy to the client
			err := k8sClient.Create(ctx, accessStrategy)
			Expect(err).NotTo(HaveOccurred())

			// Patch the Get method to track calls
			originalGet := k8sClient.Get
			originalUpdate := k8sClient.Update

			// Create a combined spy client for both Get and Update
			k8sClient = &fakeCombinedClientSpy{
				Client: k8sClient,
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					// Check if this is a Get for an AccessStrategy
					getCalled = true
					return originalGet(ctx, key, obj, opts...)
				},
				updateFunc: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					updateCalled = true
					return originalUpdate(ctx, obj, opts...)
				},
			}

			// Call the function
			err = ensureAccessStrategyFinalizer(ctx, k8sClient, workspace)

			// Verify no error occurred
			Expect(err).NotTo(HaveOccurred())

			// Verify Get(AccessStrategy) was not called
			Expect(getCalled).To(BeFalse(), "Get(AccessStrategy) should not be called when Workspace is being deleted")

			// Verify Update(AccessStrategy) was not called
			Expect(updateCalled).To(BeFalse(), "Update(AccessStrategy) should not be called when Workspace is being deleted")
		})

		It("Should return an error if the access strategy is not found", func() {
			// Create a workspace that references a non-existent access strategy
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: testWorkspaceDisplayName,
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      "nonexistent-strategy",
						Namespace: testDefaultNamespace,
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace).
				Build()

			// Call the function
			err := ensureAccessStrategyFinalizer(ctx, k8sClient, workspace)

			// Verify error occurred with correct message
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("referenced AccessStrategy nonexistent-strategy not found"))
		})

		It("Should return an error if getting the access strategy fails for another reason", func() {
			// Create a workspace with a reference to an access strategy
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: testWorkspaceDisplayName,
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testStrategyName,
						Namespace: testDefaultNamespace,
					},
				},
			}

			// Create an empty client with only the workspace
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace).
				Build()

			// Create a custom client that returns a non-NotFound error for Get
			k8sClient = &fakeClientWithError{
				Client: k8sClient,
				getFunc: func(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
					return fmt.Errorf("internal server error")
				},
			}

			// Call the function
			err := ensureAccessStrategyFinalizer(ctx, k8sClient, workspace)

			// Verify error occurred with correct message
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("internal server error"))
		})

		It("Should not call update if the access strategy already has the finalizer", func() {
			// Create a workspace with a reference to an access strategy
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: testWorkspaceDisplayName,
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testStrategyName,
						Namespace: testDefaultNamespace,
					},
				},
			}

			// Create an access strategy that already has the finalizer
			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:       testStrategyName,
					Namespace:  testDefaultNamespace,
					Finalizers: []string{workspaceutil.AccessStrategyFinalizerName},
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName: testStrategyDisplayName,
				},
			}

			// Create a client with a spy for Update
			updateCalled := false
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace, accessStrategy).
				Build()

			// Patch the Update method to track calls
			originalUpdate := k8sClient.Update
			k8sClient = &fakeClientWithUpdateSpy{
				Client: k8sClient,
				updateFunc: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					updateCalled = true
					return originalUpdate(ctx, obj, opts...)
				},
			}

			// Call the function
			err := ensureAccessStrategyFinalizer(ctx, k8sClient, workspace)

			// Verify no error occurred
			Expect(err).NotTo(HaveOccurred())

			// Verify Update was not called
			Expect(updateCalled).To(BeFalse(), "Update should not be called when finalizer is already present")
		})

		It("Should add the finalizer if the access strategy does not have the finalizer (running)", func() {
			// Create a workspace with a reference to an access strategy
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName:   testWorkspaceDisplayName,
					DesiredStatus: controller.DesiredStateRunning,
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testStrategyName,
						Namespace: testDefaultNamespace,
					},
				},
			}

			// Create an access strategy without the finalizer
			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testStrategyName,
					Namespace: testDefaultNamespace,
					// No finalizers
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName: testStrategyDisplayName,
				},
			}

			// Create a client with a spy for Update
			updateCalled := false
			var updatedObj client.Object

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace, accessStrategy).
				Build()

			// Patch the Update method to track calls
			originalUpdate := k8sClient.Update
			k8sClient = &fakeClientWithUpdateSpy{
				Client: k8sClient,
				updateFunc: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					updateCalled = true
					updatedObj = obj
					return originalUpdate(ctx, obj, opts...)
				},
			}

			// Call the function
			err := ensureAccessStrategyFinalizer(ctx, k8sClient, workspace)

			// Verify no error occurred
			Expect(err).NotTo(HaveOccurred())

			// Verify Update was called
			Expect(updateCalled).To(BeTrue(), "Update should be called to add finalizer")

			// Verify finalizer was added
			updatedAccessStrategy, ok := updatedObj.(*workspacev1alpha1.WorkspaceAccessStrategy)
			Expect(ok).To(BeTrue())
			Expect(controllerutil.ContainsFinalizer(updatedAccessStrategy, workspaceutil.AccessStrategyFinalizerName)).To(BeTrue())
		})

		It("Should add the finalizer if the access strategy does not have the finalizer (stopped)", func() {
			// Create a workspace with a reference to an access strategy
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName:   testWorkspaceDisplayName,
					DesiredStatus: controller.DesiredStateStopped,
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testStrategyName,
						Namespace: testDefaultNamespace,
					},
				},
			}

			// Create an access strategy without the finalizer
			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testStrategyName,
					Namespace: testDefaultNamespace,
					// No finalizers
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName: testStrategyDisplayName,
				},
			}

			// Create a client with a spy for Update
			updateCalled := false
			var updatedObj client.Object

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace, accessStrategy).
				Build()

			// Patch the Update method to track calls
			originalUpdate := k8sClient.Update
			k8sClient = &fakeClientWithUpdateSpy{
				Client: k8sClient,
				updateFunc: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					updateCalled = true
					updatedObj = obj
					return originalUpdate(ctx, obj, opts...)
				},
			}

			// Call the function
			err := ensureAccessStrategyFinalizer(ctx, k8sClient, workspace)

			// Verify no error occurred
			Expect(err).NotTo(HaveOccurred())

			// Verify Update was called
			Expect(updateCalled).To(BeTrue(), "Update should be called to add finalizer")

			// Verify finalizer was added
			updatedAccessStrategy, ok := updatedObj.(*workspacev1alpha1.WorkspaceAccessStrategy)
			Expect(ok).To(BeTrue())
			Expect(controllerutil.ContainsFinalizer(updatedAccessStrategy, workspaceutil.AccessStrategyFinalizerName)).To(BeTrue())
		})

		It("Should return an error if adding the finalizer fails", func() {
			// Create a workspace with a reference to an access strategy
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: testWorkspaceDisplayName,
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testStrategyName,
						Namespace: testDefaultNamespace,
					},
				},
			}

			// Create an access strategy without the finalizer
			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testStrategyName,
					Namespace: testDefaultNamespace,
					// No finalizers
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName: testStrategyDisplayName,
				},
			}

			// Create a client that will fail on Update
			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(workspace, accessStrategy).
				Build()

			// Patch the Update method to return an error
			k8sClient = &fakeClientWithUpdateError{
				Client: k8sClient,
				updateFunc: func(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
					return fmt.Errorf("update failed")
				},
			}

			// Call the function
			err := ensureAccessStrategyFinalizer(ctx, k8sClient, workspace)

			// Verify error occurred with correct message
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to add finalizer to AccessStrategy"))
			Expect(err.Error()).To(ContainSubstring("update failed"))
		})
	})

	// The mutating webhook must namespace-check a reference before stamping a protection finalizer,
	// so it never finalizes a template or access strategy the workspace is not allowed to reference
	// (the validating webhook would reject such a workspace anyway). sharedNamespace is empty here,
	// so any cross-namespace reference is out of scope.
	Context("Default namespace-scope gate before finalizing", func() {
		var defaulter WorkspaceCustomDefaulter

		newDefaulter := func(k8sClient client.Client) WorkspaceCustomDefaulter {
			return WorkspaceCustomDefaulter{
				templateDefaulter:       NewTemplateDefaulter(k8sClient, ""),
				serviceAccountDefaulter: NewServiceAccountDefaulter(k8sClient),
				templateGetter:          NewTemplateGetter(k8sClient, ""),
				templateValidator:       NewTemplateValidator(k8sClient, ""),
				accessStrategyValidator: NewAccessStrategyValidator(""),
				client:                  k8sClient,
			}
		}

		It("should reject and not finalize a template in a disallowed namespace", func() {
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testTemplateName,
					Namespace: testOtherNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  testTemplateDisplayName,
					DefaultImage: testImageTestLatest,
				},
			}
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName:   testWorkspaceDisplayName,
					Image:         testImageTestLatest,
					DesiredStatus: testStatusRunning,
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testTemplateName,
						Namespace: testOtherNamespace,
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template, workspace).
				Build()
			defaulter = newDefaulter(k8sClient)

			err := defaulter.Default(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("templateRef.namespace"))

			// Verify the finalizer was NOT stamped on the out-of-scope template
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: testTemplateName, Namespace: testOtherNamespace}, updatedTemplate)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeFalse())
		})

		It("should reject and not finalize an access strategy in a disallowed namespace", func() {
			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testStrategyName,
					Namespace: testOtherNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName: testStrategyDisplayName,
				},
			}
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName:   testWorkspaceDisplayName,
					Image:         testImageTestLatest,
					DesiredStatus: testStatusRunning,
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testStrategyName,
						Namespace: testOtherNamespace,
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(accessStrategy, workspace).
				Build()
			defaulter = newDefaulter(k8sClient)

			err := defaulter.Default(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("accessStrategy.namespace"))

			// Verify the finalizer was NOT stamped on the out-of-scope access strategy
			updatedStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: testStrategyName, Namespace: testOtherNamespace}, updatedStrategy)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updatedStrategy, workspaceutil.AccessStrategyFinalizerName)).To(BeFalse())
		})

		It("should not finalize an in-scope template when the access strategy namespace is disallowed", func() {
			// Both references are present: the template is in-scope (workspace's own namespace) but
			// the access strategy is out of scope. Because both namespace checks run before any
			// finalizer is stamped, the rejected access strategy must not leave a finalizer behind
			// on the otherwise-valid template.
			template := &workspacev1alpha1.WorkspaceTemplate{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testTemplateName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceTemplateSpec{
					DisplayName:  testTemplateDisplayName,
					DefaultImage: testImageTestLatest,
				},
			}
			accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testStrategyName,
					Namespace: testOtherNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceAccessStrategySpec{
					DisplayName: testStrategyDisplayName,
				},
			}
			workspace := &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      testWorkspaceName,
					Namespace: testDefaultNamespace,
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName:   testWorkspaceDisplayName,
					Image:         testImageTestLatest,
					DesiredStatus: testStatusRunning,
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      testTemplateName,
						Namespace: testDefaultNamespace,
					},
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      testStrategyName,
						Namespace: testOtherNamespace,
					},
				},
			}

			k8sClient = fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(template, accessStrategy, workspace).
				Build()
			defaulter = newDefaulter(k8sClient)

			err := defaulter.Default(ctx, workspace)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("accessStrategy.namespace"))

			// The in-scope template must NOT carry a protection finalizer: the access strategy
			// namespace check runs before any finalizer is stamped.
			updatedTemplate := &workspacev1alpha1.WorkspaceTemplate{}
			Expect(k8sClient.Get(ctx, client.ObjectKey{Name: testTemplateName, Namespace: testDefaultNamespace}, updatedTemplate)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(updatedTemplate, workspaceutil.TemplateFinalizerName)).To(BeFalse())
		})
	})
})
