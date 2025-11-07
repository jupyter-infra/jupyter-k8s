package authmiddleware

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	// TestDefaultNamespace is the default namespace used in tests
	TestDefaultNamespace = "default"
	// Test paths
	testWorkspacePath = "/workspaces/default/myworkspace/lab"
	// ExpectApiGroup
	expectApiGroupVersion = "connection.workspace.jupyter.org/v1alpha1"
	// Test user constants
	testUserValue   = "test-user"
	testUIDValue    = "test-uid"
	testPathValue   = "/workspaces/ns1/app1"
	testDomainValue = "example.com"
)

func TestExtractWorkspaceInfoWithDefaultRegexes(t *testing.T) {
	// Set up server with default regex patterns
	server := &Server{
		config: &Config{
			WorkspaceNamespacePathRegex: DefaultWorkspaceNamespacePathRegex,
			WorkspaceNamePathRegex:      DefaultWorkspaceNamePathRegex,
		},
		logger: slog.Default(),
	}

	testCases := []struct {
		name       string
		path       string
		expectInfo *WorkspaceInfo
		expectErr  bool
	}{
		{
			name: "Standard workspace path",
			path: "/workspaces/default/myApp",
			expectInfo: &WorkspaceInfo{
				Namespace: "default",
				Name:      "myApp",
			},
			expectErr: false,
		},
		{
			name: "Path with lab suffix",
			path: "/workspaces/default/myApp/lab",
			expectInfo: &WorkspaceInfo{
				Namespace: "default",
				Name:      "myApp",
			},
			expectErr: false,
		},
		{
			name: "Path with notebooks suffix",
			path: "/workspaces/default/myApp/notebooks/mynb.ipynb",
			expectInfo: &WorkspaceInfo{
				Namespace: "default",
				Name:      "myApp",
			},
			expectErr: false,
		},
		{
			name: "Different namespace and app name",
			path: "/workspaces/ns/app",
			expectInfo: &WorkspaceInfo{
				Namespace: "ns",
				Name:      "app",
			},
			expectErr: false,
		},
		{
			name:       "Root path should error",
			path:       "/",
			expectInfo: nil,
			expectErr:  true,
		},
		{
			name:       "Non-workspaces path should error",
			path:       "/not-workspaces/default/myApp",
			expectInfo: nil,
			expectErr:  true,
		},
		{
			name:       "Missing app name should error",
			path:       "/workspaces/myApp",
			expectInfo: nil,
			expectErr:  true,
		},
		{
			name:       "Empty namespace should error",
			path:       "/workspaces//myApp",
			expectInfo: nil,
			expectErr:  true,
		},
		{
			name:       "Missing app name at end should error",
			path:       "/workspaces/",
			expectInfo: nil,
			expectErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := server.ExtractWorkspaceInfo(tc.path)

			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, info)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, info)
				assert.Equal(t, tc.expectInfo.Namespace, info.Namespace)
				assert.Equal(t, tc.expectInfo.Name, info.Name)
			}
		})
	}
}

func TestExtractWorkspaceInfoWithCustomRegexes(t *testing.T) {
	// Set up server with custom regex patterns for a path like /services/[ws-ns]/workspaces/[ws-name]
	server := &Server{
		config: &Config{
			WorkspaceNamespacePathRegex: `^/services/([^/]+)/workspaces/[^/]+`,
			WorkspaceNamePathRegex:      `^/services/[^/]+/workspaces/([^/]+)`,
		},
		logger: slog.Default(),
	}

	testCases := []struct {
		name       string
		path       string
		expectInfo *WorkspaceInfo
		expectErr  bool
	}{
		{
			name: "Standard custom path format",
			path: "/services/tenant1/workspaces/workspace1",
			expectInfo: &WorkspaceInfo{
				Namespace: "tenant1",
				Name:      "workspace1",
			},
			expectErr: false,
		},
		{
			name: "Path with lab suffix",
			path: "/services/tenant1/workspaces/workspace1/lab",
			expectInfo: &WorkspaceInfo{
				Namespace: "tenant1",
				Name:      "workspace1",
			},
			expectErr: false,
		},
		{
			name: "Path with notebooks suffix",
			path: "/services/tenant1/workspaces/workspace1/notebooks/file.ipynb",
			expectInfo: &WorkspaceInfo{
				Namespace: "tenant1",
				Name:      "workspace1",
			},
			expectErr: false,
		},
		{
			name:       "Wrong path structure should error",
			path:       "/workspaces/default/myApp",
			expectInfo: nil,
			expectErr:  true,
		},
		{
			name:       "Root path should error",
			path:       "/",
			expectInfo: nil,
			expectErr:  true,
		},
		{
			name:       "Incomplete path should error",
			path:       "/services/tenant1",
			expectInfo: nil,
			expectErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			info, err := server.ExtractWorkspaceInfo(tc.path)

			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, info)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, info)
				assert.Equal(t, tc.expectInfo.Namespace, info.Namespace)
				assert.Equal(t, tc.expectInfo.Name, info.Name)
			}
		})
	}
}

func TestCreateConnectionAccessReview_ReturnsErrorWhenK8SClientNotSet(t *testing.T) {
	// Create a server with no REST client
	server := &Server{
		config:     &Config{},
		logger:     slog.Default(),
		restClient: nil,
	}

	// Try to check permission
	result, err := server.createConnectionAccessReview(
		context.TODO(), // Use context.TODO instead of nil
		"test-user",
		[]string{"group1", "group2"},
		TestDefaultNamespace,
		"workspace1",
		"test-uid1",
		nil,
	)

	// Verify that we get an error and false
	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "REST client not initialized")
}

func TestCreateConnectionAccessReview_CallsCreateAccessReview(t *testing.T) {
	// Define test username and groups
	username := testUserValue
	groups := []string{"group1", "group2"}
	uid := "test-uid1"
	namespace := "testNamespace"
	workspaceName := "testWorkspaceName"

	// Create a mock K8s server for testing
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	// mock response
	reason := "User owner of private Workspace"
	mockedResponse := CreateConnectionAccessReviewResponse(
		namespace,
		workspaceName,
		username,
		groups,
		uid,
		true,  // allowed
		false, // not found
		reason,
	)

	// Set up the mock server to return a success response
	mockServer.SetupServer200OK(mockedResponse)

	// Create a REST client pointing to our test server
	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	// Create a server with our mock REST client
	server := &Server{
		config:     &Config{},
		logger:     slog.Default(),
		restClient: restClient,
	}

	// Expected path for the connection access review
	expectedPath := fmt.Sprintf("/apis/%s/namespaces/%s/connectionaccessreview",
		expectApiGroupVersion, namespace)

	// Call the method being tested
	response, err := server.createConnectionAccessReview(
		context.Background(),
		username,
		groups,
		namespace,
		workspaceName,
		"test-uid2",
		nil,
	)

	// Verify the results
	assert.NoError(t, err)
	assert.True(t, response.Allowed)
	assert.False(t, response.NotFound)
	assert.Equal(t, reason, response.Reason)

	// Verify the request was made with the correct path
	mockServer.AssertRequestPath(expectedPath)

	// Verify the request was made with POST method
	mockServer.AssertRequestMethod("POST")
}

func TestCreateConnectionAccessReview_ReturnsError_WhenApiCallFails(t *testing.T) {
	// Create a mock K8s server for testing
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	// Set up the mock server to returns a permission error
	mockServer.SetupServer404NotFound()

	// Create a REST client pointing to our test server
	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	// Create a server with our mock REST client
	server := &Server{
		config:     &Config{},
		logger:     slog.Default(),
		restClient: restClient,
	}

	// Define test values
	username := testUserValue
	groups := []string{"group1", "group2"}
	namespace := "testNamespace2"
	workspaceName := "testWorkspaceName2"

	// Call the method being tested
	response, err := server.createConnectionAccessReview(
		context.Background(),
		username,
		groups,
		namespace,
		workspaceName,
		"test-uid3",
		nil,
	)

	assert.Error(t, err, "Expected error for Internal server error response")
	assert.Nil(t, response)
}

func TestVerifyWorkspaceAccess_ReturnsResultInfoAndNoError_WhenAccessReviewSucceeds(t *testing.T) {
	// Define test values
	username := testUserValue
	groups := []string{"group1", "group2"}
	uid := "test-uid4"
	namespace := "default"
	workspaceName := "myworkspace"

	// Mock response
	reason := "Workspace not found"
	mockedResponse := CreateConnectionAccessReviewResponse(
		namespace,
		workspaceName,
		username,
		groups,
		uid,
		false, // allowed
		true,  // not found
		reason,
	)

	// Create a mock K8s server for testing
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	// Set up the mock server to return a 200 OK response
	mockServer.SetupServer200OK(mockedResponse)

	// Create a REST client pointing to our test server
	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	// Create a server with our mock REST client and workspace path regex patterns
	server := &Server{
		config: &Config{
			WorkspaceNamespacePathRegex: DefaultWorkspaceNamespacePathRegex,
			WorkspaceNamePathRegex:      DefaultWorkspaceNamePathRegex,
		},
		logger:     slog.Default(),
		restClient: restClient,
	}

	// Use the predefined test workspace path
	path := testWorkspacePath

	// Call the method being tested
	response, workspaceInfo, err := server.VerifyWorkspaceAccess(
		context.Background(),
		path,
		username,
		groups,
		uid,
		nil,
	)

	// Verify the results - should be allowed with no error
	assert.NoError(t, err, "Expected no error for successful access")
	assert.NotNil(t, response, "Expected ConnectionAccessReview.Status not to be nil")
	assert.False(t, response.Allowed)
	assert.True(t, response.NotFound)
	assert.Equal(t, reason, response.Reason)
	assert.Equal(t, "myworkspace", workspaceInfo.Name, "Expected workspace name to be 'myworkspace'")
	assert.NotNil(t, workspaceInfo, "Expected workspace info to be returned")
	assert.Equal(t, "default", workspaceInfo.Namespace, "Expected namespace to be 'default'")
	assert.Equal(t, "myworkspace", workspaceInfo.Name, "Expected workspace name to be 'myworkspace'")

	// Verify the request was made to the correct URL
	expectedPath := fmt.Sprintf("/apis/%s/namespaces/%s/connectionaccessreview",
		expectApiGroupVersion, workspaceInfo.Namespace)
	mockServer.AssertRequestPath(expectedPath)
}

func TestVerifyWorkspaceAccess_ReturnsNilAndNoError_WhenPathInterpolationFails(t *testing.T) {
	// Create a mock K8s server for testing - we won't actually use this
	// since we expect the path interpolation to fail before reaching the API
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	// Set up the mock server to return a 200 OK response in case the test reaches it
	mockServer.SetupServerEmpty200OK()

	// Create a REST client pointing to our test server
	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	// Create a server with our mock REST client
	// Use workspace path regex patterns that won't match the path we'll provide
	server := &Server{
		config: &Config{
			// Expecting a different path format, not matching /workspaces/xxx/xxx
			WorkspaceNamespacePathRegex: `^/different/([^/]+)/path/[^/]+`,
			WorkspaceNamePathRegex:      `^/different/[^/]+/path/([^/]+)`,
		},
		logger:     slog.Default(),
		restClient: restClient,
	}

	// Define test values
	username := testUserValue
	groups := []string{"group1", "group2"}
	uid := "test-uid5"
	// This path won't match the regex patterns configured above
	path := "/workspaces/default/myworkspace/lab"

	// Call the method being tested
	response, workspaceInfo, err := server.VerifyWorkspaceAccess(
		context.Background(),
		path,
		username,
		groups,
		uid,
		nil,
	)

	assert.Error(t, err, "Expected error when path is invalid")
	assert.Nil(t, response, "Expected no ConnectionAccessReview response")
	assert.Nil(t, workspaceInfo, "Expected no workspace info to be returned")

	// Verify that no request was made to the API server
	assert.Nil(t, mockServer.GetLastRequest(), "Expected no request to be made to the API server")
}

func TestVerifyWorkspaceAccess_ReturnsNoResponseAndNoError_WhenAccessReviewFails(t *testing.T) {
	// Create a mock K8s server for testing
	mockServer := NewMockK8sServer(t)
	defer mockServer.Close()

	// Set up the mock server to return a 403 Forbidden response
	mockServer.SetupServer403Forbidden()

	// Create a REST client pointing to our test server
	restClient, err := mockServer.CreateRESTClient()
	require.NoError(t, err)

	// Create a server with our mock REST client and workspace path regex patterns
	server := &Server{
		config: &Config{
			WorkspaceNamespacePathRegex: DefaultWorkspaceNamespacePathRegex,
			WorkspaceNamePathRegex:      DefaultWorkspaceNamePathRegex,
		},
		logger:     slog.Default(),
		restClient: restClient,
	}

	// Define test values
	username := testUserValue
	groups := []string{"group1", "group2"}
	uid := "test-uid6"
	// Use the predefined test workspace path
	path := testWorkspacePath

	// Call the method being tested
	response, workspaceInfo, err := server.VerifyWorkspaceAccess(
		context.Background(),
		path,
		username,
		groups,
		uid,
		nil,
	)

	assert.Error(t, err, "Expected error when create ConnectionAccessReview fails")
	assert.Nil(t, response, "Expected ConnectionAccessReview response to be nil")
	assert.NotNil(t, workspaceInfo, "Expected workspace info to be returned")
	assert.Equal(t, "default", workspaceInfo.Namespace, "Expected namespace to be 'default'")
	assert.Equal(t, "myworkspace", workspaceInfo.Name, "Expected workspace name to be 'myworkspace'")
}
