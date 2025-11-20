/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package workspace

import (
	"context"
	"fmt"
	"testing"

	workspacev1alpha1 "github.com/jupyter-infra/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// FakeClient is a unified fake client that can be configured for various test scenarios
type FakeClient struct {
	client.Client
	// Update behavior settings
	ConflictOnFirstUpdate bool  // Whether to return a conflict error on first Update call
	UpdateError           error // Custom error to return on Update (if not nil)
	FailedOnce            bool  // Tracks if an update has already failed with conflict
	UpdateCalled          int   // Number of times Update was called

	// Get behavior settings
	GetError  error // Custom error to return on Get (if not nil)
	GetCalled int   // Number of times Get was called
}

func (f *FakeClient) Update(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	f.UpdateCalled++

	// If UpdateError is set, always return that error
	if f.UpdateError != nil {
		return f.UpdateError
	}

	// Otherwise check if we should return a conflict error
	if f.ConflictOnFirstUpdate && !f.FailedOnce {
		f.FailedOnce = true
		return errors.NewConflict(
			schema.GroupResource{Group: "workspace.jupyter.org", Resource: "workspaceaccessstrategies"},
			obj.GetName(),
			fmt.Errorf("conflict updating the resource"),
		)
	}

	// Default: success case
	return f.Client.Update(ctx, obj, opts...)
}

func (f *FakeClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	f.GetCalled++
	if f.GetError != nil {
		return f.GetError
	}
	return f.Client.Get(ctx, key, obj, opts...)
}

func TestSafelyAddFinalizerToAccessStrategy_HasFinalizer_IsNoop(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy with the finalizer already added
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			Finalizers: []string{
				AccessStrategyFinalizerName,
			},
		},
	}

	// Create a fake client with method call tracking
	fakeClient := &FakeClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(accessStrategy).Build(),
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyAddFinalizerToAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
	)

	// Assertions
	assert.NoError(t, err, "Expected no error when finalizer is already present")

	// Verify that no update was attempted by checking call count
	assert.Equal(t, 0, fakeClient.UpdateCalled, "Update should not have been called")
	// Verify Get was not called
	assert.Equal(t, 0, fakeClient.GetCalled, "Get should not have been called")

	// Verify that the finalizer is still present
	assert.True(t, controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName),
		"Finalizer should still be present")
	assert.Len(t, accessStrategy.Finalizers, 1, "Should still have exactly one finalizer")
}

func TestSafelyAddFinalizerToAccessStrategy_NoFinalizer_CallsUpdate(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy without the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
		},
	}

	// Create a fake client with method call tracking
	fakeClient := &FakeClient{
		Client:                fake.NewClientBuilder().WithScheme(scheme).WithObjects(accessStrategy).Build(),
		ConflictOnFirstUpdate: false, // No conflict
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyAddFinalizerToAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
	)

	// Assertions
	assert.NoError(t, err, "Expected no error when adding finalizer")

	// Verify Update was called exactly once
	assert.Equal(t, 1, fakeClient.UpdateCalled, "Update should have been called exactly once")
	// Verify Get was not called
	assert.Equal(t, 0, fakeClient.GetCalled, "Get should not have been called")

	// Verify the finalizer was added
	assert.True(t, controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName),
		"Finalizer should be added")
	assert.Len(t, accessStrategy.Finalizers, 1, "Should have exactly one finalizer")

	// Verify the object was updated in the client
	updatedAccessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{}
	err = fakeClient.Get(context.Background(),
		client.ObjectKey{Namespace: "default", Name: "test-access-strategy"},
		updatedAccessStrategy)
	assert.NoError(t, err, "Should be able to get the updated object")
	assert.True(t, controllerutil.ContainsFinalizer(updatedAccessStrategy, AccessStrategyFinalizerName),
		"Finalizer should be present in the updated object")
}

func TestSafelyAddFinalizerToAccessStrategy_OnConflictWithFinalizerAdded_CallsGetAndReturn(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy without the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			// No finalizer initially
		},
	}

	// Create a version with the finalizer already added to simulate what happens after conflict
	latestAccessStrategy := accessStrategy.DeepCopy()
	controllerutil.AddFinalizer(latestAccessStrategy, AccessStrategyFinalizerName)

	// Create a fake client that will return a conflict on the first update and then return the updated object on Get
	fakeClient := &FakeClient{
		Client:                fake.NewClientBuilder().WithScheme(scheme).WithObjects(latestAccessStrategy).Build(),
		ConflictOnFirstUpdate: true,
		FailedOnce:            false,
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyAddFinalizerToAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
	)

	// Assertions
	assert.NoError(t, err, "Expected no error when conflict occurs but finalizer is already added")

	// Verify the client first failed with a conflict and then made a Get call
	assert.True(t, fakeClient.FailedOnce, "Update should have failed once")
	assert.Equal(t, 1, fakeClient.UpdateCalled, "Update should have been called exactly once")
	assert.Equal(t, 1, fakeClient.GetCalled, "Get should have been called exactly once")
}

func TestSafelyAddFinalizerToAccessStrategy_OnConflictWithFinalizerNotAdded_ReturnUpdateError(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy without the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			// No finalizer initially
		},
	}

	// Create a version without the finalizer to simulate what happens after conflict
	// This represents the case where after a conflict, we get the latest version
	// and it doesn't have the finalizer either
	latestAccessStrategy := accessStrategy.DeepCopy()
	// No finalizer added to the latest version

	// Create a fake client that will return a conflict on the first update
	fakeClient := &FakeClient{
		Client:                fake.NewClientBuilder().WithScheme(scheme).WithObjects(latestAccessStrategy).Build(),
		ConflictOnFirstUpdate: true,
		FailedOnce:            false,
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyAddFinalizerToAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
	)

	// Assertions
	assert.Error(t, err, "Expected error when conflict occurs and finalizer not present in latest version")
	assert.True(t, errors.IsConflict(err), "Expected a conflict error")

	// Verify the client first failed with a conflict and then made a Get call
	assert.True(t, fakeClient.FailedOnce, "Update should have failed once")
	assert.Equal(t, 1, fakeClient.UpdateCalled, "Update should have been called exactly once")
	assert.Equal(t, 1, fakeClient.GetCalled, "Get should have been called exactly once")

	// Verify that finalizer is still added to our local copy (but update failed)
	assert.True(t, controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName),
		"Finalizer should still be present in our local copy")
}

func TestSafelyAddFinalizerToAccessStrategy_OnConflictWithGetError_ReturnGetError(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy without the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			// No finalizer initially
		},
	}

	// Create a Get error to be returned after conflict
	getError := errors.NewInternalError(fmt.Errorf("internal error getting resource"))

	// Create a fake client that will return a conflict on the first update and then fail on Get
	fakeClient := &FakeClient{
		Client:                fake.NewClientBuilder().WithScheme(scheme).Build(),
		ConflictOnFirstUpdate: true,
		FailedOnce:            false,
		GetError:              getError,
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyAddFinalizerToAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
	)

	// Assertions
	assert.Error(t, err, "Expected error when Get fails after conflict")
	assert.Equal(t, getError, err, "Expected the Get error to be returned")

	// Verify the client first failed with a conflict and then attempted a Get call
	assert.True(t, fakeClient.FailedOnce, "Update should have failed once")
	assert.Equal(t, 1, fakeClient.UpdateCalled, "Update should have been called exactly once")
	assert.Equal(t, 1, fakeClient.GetCalled, "Get should have been called exactly once")

	// Verify that finalizer is still added to our local copy (but update failed)
	assert.True(t, controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName),
		"Finalizer should still be present in our local copy")
}

func TestSafelyAddFinalizerToAccessStrategy_OnNonConflictError_ReturnUpdateError(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy without the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			// No finalizer initially
		},
	}

	// Create a non-conflict error to be returned by the client
	updateError := errors.NewInternalError(fmt.Errorf("internal error updating resource"))

	// Create a custom Update function with a non-conflict error
	fakeClient := &FakeClient{
		Client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
		UpdateError: updateError,
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyAddFinalizerToAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
	)

	// Assertions
	assert.Error(t, err, "Expected error for non-conflict update error")
	assert.Equal(t, updateError, err, "Expected the update error to be returned")

	// Verify Update was called exactly once
	assert.Equal(t, 1, fakeClient.UpdateCalled, "Update should have been called exactly once")
	// Verify Get was not called
	assert.Equal(t, 0, fakeClient.GetCalled, "Get should not have been called")

	// Verify that finalizer is still added to our local copy (but update failed)
	assert.True(t, controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName),
		"Finalizer should still be present in our local copy")
}

// Tests for SafelyRemoveFinalizerFromAccessStrategy

func TestSafelyRemoveFinalizerFromAccessStrategy_NoFinalizer_IsNoop(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy without the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			// No finalizer initially
		},
	}

	// Create a fake client with method call tracking
	fakeClient := &FakeClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(accessStrategy).Build(),
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyRemoveFinalizerFromAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
		false, // deletedOk = false
	)

	// Assertions
	assert.NoError(t, err, "Expected no error when finalizer is already not present")

	// Verify that Update was not called
	assert.Equal(t, 0, fakeClient.UpdateCalled, "Update should not have been called")
	// Verify Get was not called
	assert.Equal(t, 0, fakeClient.GetCalled, "Get should not have been called")

	// Verify that the finalizer is still not present
	assert.False(t, controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName),
		"Finalizer should not be present")
	assert.Empty(t, accessStrategy.Finalizers, "Should have no finalizers")
}

func TestSafelyRemoveFinalizerFromAccessStrategy_WithFinalizer_CallsUpdate(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy with the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			Finalizers: []string{
				AccessStrategyFinalizerName,
			},
		},
	}

	// Create a fake client with method call tracking
	fakeClient := &FakeClient{
		Client:                fake.NewClientBuilder().WithScheme(scheme).WithObjects(accessStrategy).Build(),
		ConflictOnFirstUpdate: false, // No conflict
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyRemoveFinalizerFromAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
		false, // deletedOk = false
	)

	// Assertions
	assert.NoError(t, err, "Expected no error when removing finalizer")

	// Verify Update was called exactly once and Get was not called
	assert.Equal(t, 1, fakeClient.UpdateCalled, "Update should have been called exactly once")
	assert.Equal(t, 0, fakeClient.GetCalled, "Get should not have been called")

	// Verify the finalizer was removed
	assert.False(t, controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName),
		"Finalizer should be removed")
	assert.Empty(t, accessStrategy.Finalizers, "Should have no finalizers")

	// Verify the object was updated in the client
	updatedAccessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{}
	err = fakeClient.Get(context.Background(),
		client.ObjectKey{Namespace: "default", Name: "test-access-strategy"},
		updatedAccessStrategy)
	assert.NoError(t, err, "Should be able to get the updated object")
	assert.False(t, controllerutil.ContainsFinalizer(updatedAccessStrategy, AccessStrategyFinalizerName),
		"Finalizer should be removed in the updated object")
}

func TestSafelyRemoveFinalizerFromAccessStrategy_OnConflictWithFinalizerRemoved_CallsGetAndReturn(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy with the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			Finalizers: []string{
				AccessStrategyFinalizerName,
			},
		},
	}

	// Create a version without the finalizer to simulate what happens after conflict
	// This represents the case where after a conflict, we get the latest version
	// and it doesn't have the finalizer (someone else removed it)
	latestAccessStrategy := accessStrategy.DeepCopy()
	controllerutil.RemoveFinalizer(latestAccessStrategy, AccessStrategyFinalizerName)

	// Create a fake client that will return a conflict on the first update and then return the updated object on Get
	fakeClient := &FakeClient{
		Client:                fake.NewClientBuilder().WithScheme(scheme).WithObjects(latestAccessStrategy).Build(),
		ConflictOnFirstUpdate: true,
		FailedOnce:            false,
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyRemoveFinalizerFromAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
		false, // deletedOk = false
	)

	// Assertions
	assert.NoError(t, err, "Expected no error when conflict occurs but finalizer is already removed")

	// Verify the client first failed with a conflict and then made a Get call
	assert.True(t, fakeClient.FailedOnce, "Update should have failed once")
	assert.Equal(t, 1, fakeClient.UpdateCalled, "Update should have been called exactly once")
	assert.Equal(t, 1, fakeClient.GetCalled, "Get should have been called exactly once")
}

func TestSafelyRemoveFinalizerFromAccessStrategy_OnConflictWithFinalizerNotRemoved_ReturnUpdateError(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy with the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			Finalizers: []string{
				AccessStrategyFinalizerName,
			},
		},
	}

	// Create a version with the finalizer still present to simulate what happens after conflict
	// This represents the case where after a conflict, we get the latest version
	// and the finalizer is still present (our removal attempt failed)
	latestAccessStrategy := accessStrategy.DeepCopy()
	// Keep the finalizer in the latest version

	// Create a fake client that will return a conflict on the first update
	fakeClient := &FakeClient{
		Client:                fake.NewClientBuilder().WithScheme(scheme).WithObjects(latestAccessStrategy).Build(),
		ConflictOnFirstUpdate: true,
		FailedOnce:            false,
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyRemoveFinalizerFromAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
		false, // deletedOk = false
	)

	// Assertions
	assert.Error(t, err, "Expected error when conflict occurs and finalizer still present in latest version")
	assert.True(t, errors.IsConflict(err), "Expected a conflict error")

	// Verify the client first failed with a conflict and then made a Get call
	assert.True(t, fakeClient.FailedOnce, "Update should have failed once")
	assert.Equal(t, 1, fakeClient.UpdateCalled, "Update should have been called exactly once")
	assert.Equal(t, 1, fakeClient.GetCalled, "Get should have been called exactly once")

	// Verify that finalizer is removed from our local copy (but update failed)
	assert.False(t, controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName),
		"Finalizer should be removed from our local copy")
}

func TestSafelyRemoveFinalizerFromAccessStrategy_OnConflictWithGetError_ReturnGetError(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy with the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			Finalizers: []string{
				AccessStrategyFinalizerName,
			},
		},
	}

	// Create a Get error to be returned after conflict
	getError := errors.NewInternalError(fmt.Errorf("internal error getting resource"))

	// Create a fake client that will return a conflict on the first update and then fail on Get
	fakeClient := &FakeClient{
		Client:                fake.NewClientBuilder().WithScheme(scheme).Build(),
		ConflictOnFirstUpdate: true,
		FailedOnce:            false,
		GetError:              getError,
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyRemoveFinalizerFromAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
		false, // deletedOk = false
	)

	// Assertions
	assert.Error(t, err, "Expected error when Get fails after conflict")
	assert.Equal(t, getError, err, "Expected the Get error to be returned")

	// Verify the client first failed with a conflict and then attempted a Get call
	assert.True(t, fakeClient.FailedOnce, "Update should have failed once")
	assert.Equal(t, 1, fakeClient.UpdateCalled, "Update should have been called exactly once")
	assert.Equal(t, 1, fakeClient.GetCalled, "Get should have been called exactly once")

	// Verify that finalizer is removed from our local copy (but update failed)
	assert.False(t, controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName),
		"Finalizer should be removed from our local copy")
}

func TestSafelyRemoveFinalizerFromAccessStrategy_OnNonConflictError_ReturnUpdateError(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy with the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			Finalizers: []string{
				AccessStrategyFinalizerName,
			},
		},
	}

	// Create a non-conflict error to be returned by the client
	updateError := errors.NewInternalError(fmt.Errorf("internal error updating resource"))

	// Create a custom Update function with a non-conflict error
	fakeClient := &FakeClient{
		Client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
		UpdateError: updateError,
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyRemoveFinalizerFromAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
		false, // deletedOk = false
	)

	// Assertions
	assert.Error(t, err, "Expected error for non-conflict update error")
	assert.Equal(t, updateError, err, "Expected the update error to be returned")

	// Verify Update was called exactly once
	assert.Equal(t, 1, fakeClient.UpdateCalled, "Update should have been called exactly once")
	// Verify Get was not called
	assert.Equal(t, 0, fakeClient.GetCalled, "Get should not have been called")

	// Verify that finalizer is removed from our local copy (but update failed)
	assert.False(t, controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName),
		"Finalizer should be removed from our local copy")
}

func TestSafelyRemoveFinalizerFromAccessStrategy_WithDeletedOkTrue_NotFoundIsOk(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy with the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			Finalizers: []string{
				AccessStrategyFinalizerName,
			},
		},
	}

	// Create a not found error to be returned by the client
	notFoundError := errors.NewNotFound(
		schema.GroupResource{Group: "workspace.jupyter.org", Resource: "workspaceaccessstrategies"},
		accessStrategy.Name,
	)

	// Create a custom Update function that returns a not found error
	fakeClient := &FakeClient{
		Client:      fake.NewClientBuilder().WithScheme(scheme).Build(),
		UpdateError: notFoundError,
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyRemoveFinalizerFromAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
		true, // deletedOk = true
	)

	// Assertions
	assert.NoError(t, err, "Expected no error when resource is not found and deletedOk is true")

	// Verify Update was called exactly once
	assert.Equal(t, 1, fakeClient.UpdateCalled, "Update should have been called exactly once")

	// Verify that finalizer is removed from our local copy
	assert.False(t, controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName),
		"Finalizer should be removed from our local copy")
}

func TestSafelyRemoveFinalizerFromAccessStrategy_OnConflictWithNotFoundGetAndDeletedOkTrue_ReturnNil(t *testing.T) {
	// Set up a test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a test access strategy with the finalizer
	accessStrategy := &workspacev1alpha1.WorkspaceAccessStrategy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-access-strategy",
			Namespace: "default",
			Finalizers: []string{
				AccessStrategyFinalizerName,
			},
		},
	}

	// Create a not found error for the Get call after conflict
	notFoundError := errors.NewNotFound(
		schema.GroupResource{Group: "workspace.jupyter.org", Resource: "workspaceaccessstrategies"},
		accessStrategy.Name,
	)

	// Create a fake client that will return a conflict on the first update and then a not found error on Get
	fakeClient := &FakeClient{
		Client:                fake.NewClientBuilder().WithScheme(scheme).Build(),
		ConflictOnFirstUpdate: true,
		FailedOnce:            false,
		GetError:              notFoundError,
	}

	// Set up a test logger
	logger := zap.New(zap.UseDevMode(true)).WithName("test")

	// Call the function being tested
	err := SafelyRemoveFinalizerFromAccessStrategy(
		context.Background(),
		logger,
		fakeClient,
		accessStrategy,
		true, // deletedOk = true
	)

	// Assertions
	assert.NoError(t, err, "Expected no error when Get returns NotFound after conflict and deletedOk is true")

	// Verify the client first failed with a conflict and then attempted a Get call
	assert.True(t, fakeClient.FailedOnce, "Update should have failed once")
	assert.Equal(t, 1, fakeClient.UpdateCalled, "Update should have been called exactly once")
	assert.Equal(t, 1, fakeClient.GetCalled, "Get should have been called exactly once")

	// Verify that finalizer is removed from our local copy
	assert.False(t, controllerutil.ContainsFinalizer(accessStrategy, AccessStrategyFinalizerName),
		"Finalizer should be removed from our local copy")
}
