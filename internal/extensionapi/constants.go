package extensionapi

const (
	// VSCodeScheme is the URL scheme for VSCode remote connections
	VSCodeScheme = "vscode://amazonwebservices.aws-toolkit-vscode/connect/sagemaker"

	// HeaderUser is the X-User header
	HeaderUser = "X-User"
	// HeaderRemoteUser is the X-Remote-User header
	HeaderRemoteUser = "X-Remote-User"
	// HeaderRemoteGroup is the X-Remote-Group header
	HeaderRemoteGroup = "X-Remote-Group"
	// ExtraHeaderPrefix is the prefix for extra authentication headers
	ExtraHeaderPrefix = "X-Remote-Extra-"

	// AuthConfigMapName is the name of the authentication ConfigMap
	AuthConfigMapName = "extension-apiserver-authentication"
	// AuthConfigMapNamespace is the namespace containing the authentication ConfigMap
	AuthConfigMapNamespace = "kube-system"

	// RequestHeaderClientCAFileKey is the ConfigMap key for the client CA certificate
	RequestHeaderClientCAFileKey = "requestheader-client-ca-file"
	// RequestHeaderAllowedNamesKey is the ConfigMap key for allowed client names
	RequestHeaderAllowedNamesKey = "requestheader-allowed-names"

	// WebUIURLFormat is the URL format for WebUI bearer token authentication
	WebUIURLFormat = "%s/workspaces/%s/%s/bearer-auth?token=%s"
)
