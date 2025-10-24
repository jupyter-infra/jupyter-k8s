package authmiddleware

// HTTP header constants used by the authentication middleware
const (
	// Headers from auth proxy
	HeaderAuthRequestUser              = "X-Auth-Request-User"
	HeaderAuthRequestGroups            = "X-Auth-Request-Groups"
	HeaderAuthRequestPreferredUsername = "X-Auth-Request-Preferred-Username"

	// Headers from reverse proxy
	HeaderForwardedURI   = "X-Forwarded-Uri"
	HeaderForwardedHost  = "X-Forwarded-Host"
	HeaderForwardedProto = "X-Forwarded-Proto"

	// No headers set by middleware yet

	// Special groups
	SystemAuthenticatedGroup = "system:authenticated"
)
