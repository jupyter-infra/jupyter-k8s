/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

// BasePath is the API version prefix for all plugin routes.
const BasePath = "/v1alpha1"

// Plugin HTTP routes. Each route defines its path and HTTP method.
var (
	RouteJWTSign          = Route{Method: "POST", Path: BasePath + "/jwt/sign"}
	RouteJWTVerify        = Route{Method: "POST", Path: BasePath + "/jwt/verify"}
	RouteRemoteAccessInit = Route{Method: "POST", Path: BasePath + "/remote-access/initialize"}
	RouteRegisterNode     = Route{Method: "POST", Path: BasePath + "/remote-access/register-node"}
	RouteDeregisterNode   = Route{Method: "POST", Path: BasePath + "/remote-access/deregister-node"}
	RouteCreateSession    = Route{Method: "POST", Path: BasePath + "/remote-access/create-session"}
	RouteHealthz          = Route{Method: "GET", Path: "/healthz"}
)

// Route defines an HTTP method and path for a plugin endpoint.
type Route struct {
	Method string
	Path   string
}

// Pattern returns the method + path string used by Go 1.22+ ServeMux (e.g. "POST /v1alpha1/jwt/sign").
func (r Route) Pattern() string {
	return r.Method + " " + r.Path
}
