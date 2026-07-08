/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

// Shared test constants for repeated string literals across the package's tests.
const (
	// namespaceDefault is the default Kubernetes namespace used in tests.
	namespaceDefault = "default"

	// testToken is a placeholder JWT token used in tests.
	testToken = "test-token"

	// testStrategy is a placeholder access strategy name used in tests.
	testStrategy = "test-strategy"

	// testWorkspace is a placeholder workspace name used in tests.
	testWorkspace = "test-workspace"

	// testUser1 is a placeholder username used in tests.
	testUser1 = "test-user1"

	// differentUser is a placeholder username distinct from the owner used in tests.
	differentUser = "different-user"

	// exampleURL is a placeholder URL used in tests.
	exampleURL = "https://example.com"

	// systemAuthenticated is the standard Kubernetes authenticated-users group.
	systemAuthenticated = "system:authenticated"

	// accessTypeOwnerOnly is the OwnerOnly workspace access type value used in tests.
	accessTypeOwnerOnly = "OwnerOnly"

	// connectionTypeVSCodeRemote is the vscode-remote connection type used in tests.
	connectionTypeVSCodeRemote = "vscode-remote"

	// connectionTypeWebUI is the web-ui connection type used in tests.
	connectionTypeWebUI = "web-ui"

	// invalidConnectionType is an unsupported connection type used in tests.
	invalidConnectionType = "invalid-type"

	// pluginNameAWS is the AWS plugin name used in tests.
	pluginNameAWS = "aws"

	// connectionsPath is the connections API path used in tests.
	connectionsPath = "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/default/connections"

	// errCannotFindNamespace is the expected error substring when a namespace is missing.
	errCannotFindNamespace = "cannot find the namespace"
)
