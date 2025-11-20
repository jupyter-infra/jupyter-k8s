/*
Copyright (c) 2025 Amazon Web Services

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package authmiddleware

// HTTP header constants used by the authentication middleware
const (
	// Headers from auth proxy
	HeaderAuthRequestUser              = "X-Auth-Request-User"
	HeaderAuthRequestGroups            = "X-Auth-Request-Groups"
	HeaderAuthRequestPreferredUsername = "X-Auth-Request-Preferred-Username"
	HeaderAuthorization                = "Authorization"

	// Headers from reverse proxy
	HeaderForwardedURI   = "X-Forwarded-Uri"
	HeaderForwardedHost  = "X-Forwarded-Host"
	HeaderForwardedProto = "X-Forwarded-Proto"

	// No headers set by middleware yet

	// Special groups
	SystemAuthenticatedGroup = "system:authenticated"

	// Routing modes
	RoutingModeSubdomain = "subdomain"
	RoutingModePath      = "path"

	// OIDC constants
	OIDCAuthHeaderPrefix = "Bearer "
)
