/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package extensionapi

const (
	// VSCodeScheme is the URL scheme for VSCode remote connections
	VSCodeScheme = "vscode://amazonwebservices.aws-toolkit-vscode/connect/sagemaker"

	// HeaderUser is the X-User header
	HeaderUser = "X-User"
	// HeaderRemoteUser is the X-Remote-User header
	HeaderRemoteUser = "X-Remote-User"

	// WebUIURLFormat is the URL format for WebUI bearer token authentication
	WebUIURLFormat = "%s/workspaces/%s/%s/bearer-auth?token=%s"
)
