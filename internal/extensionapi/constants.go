package extensionapi

const (
	// VSCodeScheme is the URL scheme for VSCode remote connections
	VSCodeScheme = "vscode://amazonwebservices.aws-toolkit-vscode/connect/sagemaker"

	// OwnershipTypePublic represents public workspace ownership
	OwnershipTypePublic = "Public"
	// OwnershipTypeOwnerOnly represents owner-only workspace ownership
	OwnershipTypeOwnerOnly = "OwnerOnly"

	// HeaderUser is the X-User header
	HeaderUser = "X-User"
	// HeaderRemoteUser is the X-Remote-User header
	HeaderRemoteUser = "X-Remote-User"
)
