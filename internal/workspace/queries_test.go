/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package workspace

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	workspacev1alpha1 "github.com/jupyter-ai-contrib/jupyter-k8s/api/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// MockClient is a mock implementation of client.Client for testing error cases
type MockClient struct {
	client.Client
	ListError error
	GetError  error

	// Store the list options for verification
	ListOptions []client.ListOption
}

func (m *MockClient) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	// Save the options for later verification
	m.ListOptions = opts

	if m.ListError != nil {
		return m.ListError
	}
	return nil
}

// Helper functions to extract information from list options
func getLabelsFromOption(optString string) map[string]string {
	labels := make(map[string]string)

	// Check if it's a map format like "map[key:value key2:value2]"
	if strings.HasPrefix(optString, "map[") {
		// Remove the "map[" prefix and "]" suffix
		mapContent := optString[4 : len(optString)-1]

		// Split by spaces to get individual key:value pairs
		pairs := strings.Split(mapContent, " ")
		for _, pair := range pairs {
			parts := strings.SplitN(pair, ":", 2)
			if len(parts) == 2 {
				key := parts[0]
				value := parts[1]
				labels[key] = value
			}
		}
		return labels
	}

	// Legacy format for older tests
	if strings.Contains(optString, "labels") {
		// Extract the part between square brackets
		startIdx := strings.Index(optString, "[")
		endIdx := strings.LastIndex(optString, "]")
		if startIdx != -1 && endIdx != -1 && startIdx < endIdx {
			labelsStr := optString[startIdx+1 : endIdx]

			// Split by comma to get individual key-value pairs
			pairs := strings.Split(labelsStr, ",")
			for _, pair := range pairs {
				parts := strings.SplitN(pair, ":", 2)
				if len(parts) == 2 {
					// Trim any whitespace
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					labels[key] = value
				}
			}
		}
	}
	return labels
}

func getLimitFromOption(optString string) int64 {
	// Direct integer value like "10"
	if limit, err := strconv.ParseInt(optString, 10, 64); err == nil {
		return limit
	}

	// Legacy format: "limit=X" pattern
	if strings.Contains(optString, "limit=") {
		parts := strings.Split(optString, "=")
		if len(parts) == 2 {
			limit, err := strconv.ParseInt(parts[1], 10, 64)
			if err == nil {
				return limit
			}
		}
	}
	return 0
}

func getContinueFromOption(optString string) string {
	// Direct string value that doesn't look like a map or number
	if !strings.HasPrefix(optString, "map[") && !strings.HasPrefix(optString, "{") &&
		optString != "" && len(optString) > 0 {
		// Try to parse as int - if it fails, it might be a continue token
		_, err := strconv.ParseInt(optString, 10, 64)
		if err != nil {
			return optString
		}
	}

	// Legacy format: "continue=X" pattern
	if strings.Contains(optString, "continue=") {
		parts := strings.Split(optString, "=")
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return ""
}

func (m *MockClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if m.GetError != nil {
		return m.GetError
	}
	return nil
}

func TestGetTemplateRefNamespace(t *testing.T) {
	// Test cases:
	// a) No template ref - should return workspace namespace
	// b) Template ref with empty namespace - should return workspace namespace
	// c) Template ref with namespace set - should return template namespace

	testCases := []struct {
		name           string
		workspace      *workspacev1alpha1.Workspace
		expectedResult string
	}{
		{
			name: "No template ref",
			workspace: &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					// No TemplateRef
				},
			},
			expectedResult: "default", // Should return workspace namespace
		},
		{
			name: "Empty namespace in template ref",
			workspace: &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name: "test-template",
						// No Namespace specified
					},
				},
			},
			expectedResult: "default", // Should return workspace namespace
		},
		{
			name: "With namespace in template ref",
			workspace: &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					TemplateRef: &workspacev1alpha1.TemplateRef{
						Name:      "test-template",
						Namespace: "template-namespace",
					},
				},
			},
			expectedResult: "template-namespace", // Should return template namespace
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetTemplateRefNamespace(tc.workspace)
			if result != tc.expectedResult {
				t.Errorf("GetTemplateRefNamespace() = %v, want %v", result, tc.expectedResult)
			}
		})
	}
}

func TestGetAccessStrategyRefNamespace(t *testing.T) {
	// Test cases:
	// a) No access strategy ref - should return workspace namespace
	// b) Access strategy ref with empty namespace - should return workspace namespace
	// c) Access strategy ref with namespace set - should return access strategy namespace

	testCases := []struct {
		name           string
		workspace      *workspacev1alpha1.Workspace
		expectedResult string
	}{
		{
			name: "No access strategy ref",
			workspace: &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					// No AccessStrategy
				},
			},
			expectedResult: "default", // Should return workspace namespace
		},
		{
			name: "Empty namespace in access strategy ref",
			workspace: &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name: "test-access-strategy",
						// No Namespace specified
					},
				},
			},
			expectedResult: "default", // Should return workspace namespace
		},
		{
			name: "With namespace in access strategy ref",
			workspace: &workspacev1alpha1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workspace",
					Namespace: "default",
				},
				Spec: workspacev1alpha1.WorkspaceSpec{
					DisplayName: "Test Workspace",
					AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
						Name:      "test-access-strategy",
						Namespace: "access-strategy-namespace",
					},
				},
			},
			expectedResult: "access-strategy-namespace", // Should return access strategy namespace
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GetAccessStrategyRefNamespace(tc.workspace)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestHasActiveWorkspacesWithTemplate_UsesLabelMatcher(t *testing.T) {
	// Setup test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a mock client to capture list options
	mockClient := &MockClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	// Call the function
	_, err := HasActiveWorkspacesWithTemplate(context.Background(), mockClient, "test-template", "template-namespace")

	// Assertions
	assert.NoError(t, err)

	// Verify that the List function was called with the correct labels
	assert.NotEmpty(t, mockClient.ListOptions, "List options should not be empty")

	// Extract the matching labels from the list options
	labels := make(map[string]string)
	for _, opt := range mockClient.ListOptions {
		optString := fmt.Sprintf("%v", opt)
		if matches := getLabelsFromOption(optString); len(matches) > 0 {
			for k, v := range matches {
				labels[k] = v
			}
		}
	}

	assert.NotEmpty(t, labels, "List should be called with labels")
	assert.Equal(t, "test-template", labels[LabelWorkspaceTemplate], "Should filter by template name")
	assert.Equal(t, "template-namespace", labels[LabelWorkspaceTemplateNamespace], "Should filter by template namespace")
}

func TestHasActiveWorkspacesWithTemplate_SkipsDeletedWorkspaces(t *testing.T) {
	// Setup test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a deleted workspace with matching labels and template ref
	deletionTime := metav1.Now()
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-workspace",
			Namespace:         "default",
			DeletionTimestamp: &deletionTime,
			// Add a finalizer since k8s objects with deletion timestamp must have at least one finalizer
			Finalizers: []string{"test-finalizer"},
			Labels: map[string]string{
				LabelWorkspaceTemplate:          "test-template",
				LabelWorkspaceTemplateNamespace: "template-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace",
			TemplateRef: &workspacev1alpha1.TemplateRef{
				Name:      "test-template",
				Namespace: "template-namespace",
			},
		},
	}

	// Create fake client with the workspace
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()

	// Call the function
	result, err := HasActiveWorkspacesWithTemplate(context.Background(), fakeClient, "test-template", "template-namespace")

	// Assertions
	assert.NoError(t, err)
	assert.False(t, result, "Expected to not find active workspaces with template")
}

func TestHasActiveWorkspacesWithTemplate_ReturnTrueOnFirstMatch(t *testing.T) {
	// Setup test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create multiple workspaces with matching labels and template ref
	ws1 := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace-1",
			Namespace: "default",
			Labels: map[string]string{
				LabelWorkspaceTemplate:          "test-template",
				LabelWorkspaceTemplateNamespace: "template-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace 1",
			TemplateRef: &workspacev1alpha1.TemplateRef{
				Name:      "test-template",
				Namespace: "template-namespace",
			},
		},
	}

	ws2 := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace-2",
			Namespace: "default",
			Labels: map[string]string{
				LabelWorkspaceTemplate:          "test-template",
				LabelWorkspaceTemplateNamespace: "template-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace 2",
			TemplateRef: &workspacev1alpha1.TemplateRef{
				Name:      "test-template",
				Namespace: "template-namespace",
			},
		},
	}

	// Create fake client with the workspaces
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws1, ws2).Build()

	// Call the function
	result, err := HasActiveWorkspacesWithTemplate(context.Background(), fakeClient, "test-template", "template-namespace")

	// Assertions
	assert.NoError(t, err)
	assert.True(t, result, "Expected to find workspace with template")
}

func TestHasActiveWorkspacesWithTemplate_ReturnFalseOnNoMatch(t *testing.T) {
	// Setup test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a workspace with different template
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace",
			Namespace: "default",
			Labels: map[string]string{
				LabelWorkspaceTemplate:          "different-template",
				LabelWorkspaceTemplateNamespace: "template-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace",
			TemplateRef: &workspacev1alpha1.TemplateRef{
				Name:      "different-template",
				Namespace: "template-namespace",
			},
		},
	}

	// Create fake client with the workspace
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()

	// Call the function
	result, err := HasActiveWorkspacesWithTemplate(context.Background(), fakeClient, "test-template", "template-namespace")

	// Assertions
	assert.NoError(t, err)
	assert.False(t, result, "Expected to not find workspace with template")
}

func TestHasActiveWorkspacesWithTemplate_OnListError_ReturnError(t *testing.T) {
	// Create a mock client that returns an error on List
	mockClient := &MockClient{
		ListError: fmt.Errorf("mock list error"),
	}

	// Call the function
	result, err := HasActiveWorkspacesWithTemplate(context.Background(), mockClient, "test-template", "template-namespace")

	// Assertions
	assert.Error(t, err)
	assert.False(t, result, "Expected false when client returns an error")
	assert.Contains(t, err.Error(), "failed to check workspaces by template label: mock list error")
}

func TestHasActiveWorkspacesWithAccessStrategy_UsesLabelMatcher(t *testing.T) {
	// Setup test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a mock client to capture list options
	mockClient := &MockClient{
		Client: fake.NewClientBuilder().WithScheme(scheme).Build(),
	}

	// Call the function
	_, err := HasActiveWorkspacesWithAccessStrategy(
		context.Background(),
		mockClient,
		"test-access-strategy",
		"access-strategy-namespace")

	// Assertions
	assert.NoError(t, err)

	// Verify that the List function was called with the correct labels
	assert.NotEmpty(t, mockClient.ListOptions, "List options should not be empty")

	// Extract the matching labels from the list options
	labels := make(map[string]string)
	for _, opt := range mockClient.ListOptions {
		optString := fmt.Sprintf("%v", opt)
		if matches := getLabelsFromOption(optString); len(matches) > 0 {
			for k, v := range matches {
				labels[k] = v
			}
		}
	}

	assert.NotEmpty(t, labels, "List should be called with labels")
	assert.Equal(t, "test-access-strategy", labels[LabelAccessStrategyName], "Should filter by access strategy name")
	assert.Equal(t, "access-strategy-namespace", labels[LabelAccessStrategyNamespace], "Should filter by access strategy namespace")
}

func TestHasActiveWorkspacesWithAccessStrategy_SkipsDeletedWorkspaces(t *testing.T) {
	// Setup test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a deleted workspace with matching labels and access strategy ref
	deletionTime := metav1.Now()
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-workspace",
			Namespace:         "default",
			DeletionTimestamp: &deletionTime,
			// Add a finalizer since k8s objects with deletion timestamp must have at least one finalizer
			Finalizers: []string{"test-finalizer"},
			Labels: map[string]string{
				LabelAccessStrategyName:      "test-access-strategy",
				LabelAccessStrategyNamespace: "access-strategy-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace",
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name:      "test-access-strategy",
				Namespace: "access-strategy-namespace",
			},
		},
	}

	// Create fake client with the workspace
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()

	// Call the function
	result, err := HasActiveWorkspacesWithAccessStrategy(
		context.Background(),
		fakeClient,
		"test-access-strategy",
		"access-strategy-namespace")

	// Assertions
	assert.NoError(t, err)
	assert.False(t, result, "Expected to not find active workspaces with access strategy")
}

func TestHasActiveWorkspacesWithAccessStrategy_ReturnTrueOnFirstMatch(t *testing.T) {
	// Setup test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create multiple workspaces with matching labels and access strategy ref
	ws1 := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace-1",
			Namespace: "default",
			Labels: map[string]string{
				LabelAccessStrategyName:      "test-access-strategy",
				LabelAccessStrategyNamespace: "access-strategy-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace 1",
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name:      "test-access-strategy",
				Namespace: "access-strategy-namespace",
			},
		},
	}

	ws2 := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace-2",
			Namespace: "default",
			Labels: map[string]string{
				LabelAccessStrategyName:      "test-access-strategy",
				LabelAccessStrategyNamespace: "access-strategy-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace 2",
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name:      "test-access-strategy",
				Namespace: "access-strategy-namespace",
			},
		},
	}

	// Create fake client with the workspaces
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws1, ws2).Build()

	// Call the function
	result, err := HasActiveWorkspacesWithAccessStrategy(
		context.Background(),
		fakeClient,
		"test-access-strategy",
		"access-strategy-namespace")

	// Assertions
	assert.NoError(t, err)
	assert.True(t, result, "Expected to find workspace with access strategy")
}

func TestHasActiveWorkspacesWithAccessStrategy_ReturnFalseOnNoMatch(t *testing.T) {
	// Setup test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create a workspace with different access strategy
	ws := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace",
			Namespace: "default",
			Labels: map[string]string{
				LabelAccessStrategyName:      "different-access-strategy",
				LabelAccessStrategyNamespace: "access-strategy-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace",
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name:      "different-access-strategy",
				Namespace: "access-strategy-namespace",
			},
		},
	}

	// Create fake client with the workspace
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws).Build()

	// Call the function
	result, err := HasActiveWorkspacesWithAccessStrategy(
		context.Background(),
		fakeClient,
		"test-access-strategy",
		"access-strategy-namespace")

	// Assertions
	assert.NoError(t, err)
	assert.False(t, result, "Expected to not find workspace with access strategy")
}

func TestHasActiveWorkspacesWithAccessStrategy_OnListError_ReturnError(t *testing.T) {
	// Create a mock client that returns an error on List
	mockClient := &MockClient{
		ListError: fmt.Errorf("mock list error"),
	}

	// Call the function
	result, err := HasActiveWorkspacesWithAccessStrategy(
		context.Background(),
		mockClient,
		"test-access-strategy",
		"access-strategy-namespace")

	// Assertions
	assert.Error(t, err)
	assert.False(t, result, "Expected false when client returns an error")
	assert.Contains(t, err.Error(), "failed to check workspaces by access strategy label: mock list error")
}

func TestListActiveWorkspacesByAccessStrategy_CallsListWithMatchLabels_ReturnWorkspaces(t *testing.T) {
	// Setup test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create workspaces with matching labels and access strategy refs
	ws1 := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace-1",
			Namespace: "default",
			Labels: map[string]string{
				LabelAccessStrategyName:      "test-access-strategy",
				LabelAccessStrategyNamespace: "access-strategy-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace 1",
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name:      "test-access-strategy",
				Namespace: "access-strategy-namespace",
			},
		},
	}

	ws2 := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace-2",
			Namespace: "default",
			Labels: map[string]string{
				LabelAccessStrategyName:      "test-access-strategy",
				LabelAccessStrategyNamespace: "access-strategy-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace 2",
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name:      "test-access-strategy",
				Namespace: "access-strategy-namespace",
			},
		},
	}

	// Create a deleted workspace that should be skipped
	deletionTime := metav1.Now()
	wsDeleted := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-workspace-deleted",
			Namespace:         "default",
			DeletionTimestamp: &deletionTime,
			// Add a finalizer since k8s objects with deletion timestamp must have at least one finalizer
			Finalizers: []string{"test-finalizer"},
			Labels: map[string]string{
				LabelAccessStrategyName:      "test-access-strategy",
				LabelAccessStrategyNamespace: "access-strategy-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace Deleted",
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name:      "test-access-strategy",
				Namespace: "access-strategy-namespace",
			},
		},
	}

	// Create a regular fakeClient with objects for testing the results
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws1, ws2, wsDeleted).Build()

	// Create a mock client to capture list options
	mockClient := &MockClient{
		// Clone the real client's scheme
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws1, ws2, wsDeleted).Build(),
	}

	// Call the function with the mock client to check labels
	_, _, err := ListActiveWorkspacesByAccessStrategy(
		context.Background(),
		mockClient,
		"test-access-strategy",
		"access-strategy-namespace",
		"token-123", // Add a token to verify it's passed correctly
		10)          // Add a limit to verify it's passed correctly

	// Assertions for the mock client
	assert.NoError(t, err)

	// Verify that the List function was called with the correct labels
	assert.NotEmpty(t, mockClient.ListOptions, "List options should not be empty")

	// Extract the labels, limit, and continue token from the list options
	labels := make(map[string]string)
	var limitValue int64
	var continueToken string

	// Convert list options to strings for inspection
	for _, opt := range mockClient.ListOptions {
		optString := fmt.Sprintf("%v", opt)

		// Check for labels
		if matches := getLabelsFromOption(optString); len(matches) > 0 {
			for k, v := range matches {
				labels[k] = v
			}
		}

		// Check for limit
		if limitMatch := getLimitFromOption(optString); limitMatch > 0 {
			limitValue = limitMatch
		}

		// Check for continue token
		if tokenMatch := getContinueFromOption(optString); tokenMatch != "" {
			continueToken = tokenMatch
		}
	}

	// Verify the labels
	assert.NotEmpty(t, labels, "List should be called with labels")
	assert.Equal(t, "test-access-strategy", labels[LabelAccessStrategyName], "Should filter by access strategy name")
	assert.Equal(t, "access-strategy-namespace", labels[LabelAccessStrategyNamespace], "Should filter by access strategy namespace")

	// Verify limit and continue token
	assert.Equal(t, int64(10), limitValue, "Limit should be 10")
	assert.Equal(t, "token-123", continueToken, "Continue token should match")

	// Now call the function with the real client to test the actual functionality
	workspaces, nextToken, err := ListActiveWorkspacesByAccessStrategy(
		context.Background(),
		fakeClient,
		"test-access-strategy",
		"access-strategy-namespace",
		"", // No continuation token
		0)  // No limit

	// Assertions for the real client
	assert.NoError(t, err)
	assert.Equal(t, 2, len(workspaces), "Expected to find 2 active workspaces")
	assert.Empty(t, nextToken, "Expected no continuation token")

	// Check that we got the expected workspaces and not the deleted one
	workspaceNames := []string{workspaces[0].Name, workspaces[1].Name}
	assert.Contains(t, workspaceNames, "test-workspace-1")
	assert.Contains(t, workspaceNames, "test-workspace-2")
	assert.NotContains(t, workspaceNames, "test-workspace-deleted")
}

func TestListActiveWorkspacesByAccessStrategy_OnListError_ReturnErrors(t *testing.T) {
	// Create a mock client that returns an error on List
	mockClient := &MockClient{
		ListError: fmt.Errorf("mock list error"),
	}

	// Call the function
	workspaces, nextToken, err := ListActiveWorkspacesByAccessStrategy(
		context.Background(),
		mockClient,
		"test-access-strategy",
		"access-strategy-namespace",
		"",
		0)

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, workspaces)
	assert.Empty(t, nextToken)
	assert.Contains(t, err.Error(), "failed to list workspaces by AccessStrategy label: mock list error")
}

func TestGetWorkspaceReconciliationRequestsForAccessStrategy_CallsListWithMatchLabels_ReturnRequests(t *testing.T) {
	// Setup test scheme
	scheme := runtime.NewScheme()
	_ = workspacev1alpha1.AddToScheme(scheme)

	// Create workspaces with matching labels and access strategy refs
	ws1 := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace-1",
			Namespace: "default",
			Labels: map[string]string{
				LabelAccessStrategyName:      "test-access-strategy",
				LabelAccessStrategyNamespace: "access-strategy-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace 1",
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name:      "test-access-strategy",
				Namespace: "access-strategy-namespace",
			},
		},
	}

	ws2 := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-workspace-2",
			Namespace: "another-namespace",
			Labels: map[string]string{
				LabelAccessStrategyName:      "test-access-strategy",
				LabelAccessStrategyNamespace: "access-strategy-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace 2",
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name:      "test-access-strategy",
				Namespace: "access-strategy-namespace",
			},
		},
	}

	// Create a deleted workspace that should be skipped
	deletionTime := metav1.Now()
	wsDeleted := &workspacev1alpha1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-workspace-deleted",
			Namespace:         "default",
			DeletionTimestamp: &deletionTime,
			// Add a finalizer since k8s objects with deletion timestamp must have at least one finalizer
			Finalizers: []string{"test-finalizer"},
			Labels: map[string]string{
				LabelAccessStrategyName:      "test-access-strategy",
				LabelAccessStrategyNamespace: "access-strategy-namespace",
			},
		},
		Spec: workspacev1alpha1.WorkspaceSpec{
			DisplayName: "Test Workspace Deleted",
			AccessStrategy: &workspacev1alpha1.AccessStrategyRef{
				Name:      "test-access-strategy",
				Namespace: "access-strategy-namespace",
			},
		},
	}

	// Create a regular fakeClient with objects for testing the results
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws1, ws2, wsDeleted).Build()

	// Create a mock client to capture list options
	mockClient := &MockClient{
		// Clone the real client's scheme
		Client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(ws1, ws2, wsDeleted).Build(),
	}

	// Call the function with the mock client to check labels
	_, err := GetWorkspaceReconciliationRequestsForAccessStrategy(
		context.Background(),
		mockClient,
		"test-access-strategy",
		"access-strategy-namespace")

	// Assertions for the mock client
	assert.NoError(t, err)

	// Verify that the List function was called with the correct labels
	assert.NotEmpty(t, mockClient.ListOptions, "List options should not be empty")

	// Extract the matching labels from the list options
	labels := make(map[string]string)
	for _, opt := range mockClient.ListOptions {
		optString := fmt.Sprintf("%v", opt)
		if matches := getLabelsFromOption(optString); len(matches) > 0 {
			for k, v := range matches {
				labels[k] = v
			}
		}
	}

	// Verify the labels
	assert.NotEmpty(t, labels, "List should be called with labels")
	assert.Equal(t, "test-access-strategy", labels[LabelAccessStrategyName], "Should filter by access strategy name")
	assert.Equal(t, "access-strategy-namespace", labels[LabelAccessStrategyNamespace], "Should filter by access strategy namespace")

	// Now call the function with the real client to test the actual functionality
	requests, err := GetWorkspaceReconciliationRequestsForAccessStrategy(
		context.Background(),
		fakeClient,
		"test-access-strategy",
		"access-strategy-namespace")

	// Assertions for the real client
	assert.NoError(t, err)
	assert.Equal(t, 2, len(requests), "Expected to get 2 reconciliation requests")

	// Check that we got the expected reconciliation requests
	// Create a map of namespace/name to track which ones we found
	foundRequests := make(map[string]bool)
	for _, req := range requests {
		key := req.Namespace + "/" + req.Name
		foundRequests[key] = true
	}

	assert.True(t, foundRequests["default/test-workspace-1"], "Expected request for test-workspace-1")
	assert.True(t, foundRequests["another-namespace/test-workspace-2"], "Expected request for test-workspace-2")
	assert.False(t, foundRequests["default/test-workspace-deleted"], "Should not have request for deleted workspace")
}

func TestGetWorkspaceReconciliationRequestsForAccessStrategy_OnListError_ReturnErrors(t *testing.T) {
	// Create a mock client that returns an error on List
	mockClient := &MockClient{
		ListError: fmt.Errorf("mock list error"),
	}

	// Call the function
	requests, err := GetWorkspaceReconciliationRequestsForAccessStrategy(
		context.Background(),
		mockClient,
		"test-access-strategy",
		"access-strategy-namespace")

	// Assertions
	assert.Error(t, err)
	assert.Nil(t, requests)
	assert.Contains(t, err.Error(), "failed to list workspaces by access strategy: failed to list workspaces by AccessStrategy label: mock list error")
}
