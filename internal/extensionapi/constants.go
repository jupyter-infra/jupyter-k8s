package extensionapi

const (
	// WorkspaceConnectionAPIVersion is the API version for workspace connections
	WorkspaceConnectionAPIVersion = "connection.workspace.jupyter.org/v1alpha1"
	// WorkspaceConnectionKind is the kind for workspace connection resources
	WorkspaceConnectionKind = "WorkspaceConnection"

	// ConnectionTypeVSCodeRemote represents VSCode remote connection type
	ConnectionTypeVSCodeRemote = "vscode-remote"
	// ConnectionTypeWebUI represents web UI connection type
	ConnectionTypeWebUI = "web-ui"

	// VSCodeScheme is the URL scheme for VSCode remote connections
	VSCodeScheme = "vscode://amazonwebservices.aws-toolkit-vscode/connect/sagemaker"
)
