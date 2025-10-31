package extensionapi

const (
	// VSCodeScheme is the URL scheme for VSCode remote connections
	VSCodeScheme = "vscode://amazonwebservices.aws-toolkit-vscode/connect/sagemaker"

	// WebUIURLFormat is the URL format for WebUI bearer token authentication
	WebUIURLFormat = "%s/workspaces/%s/%s/bearer-auth?token=%s"
)
