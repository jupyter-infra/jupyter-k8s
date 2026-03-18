/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package v1alpha1

// ErrorResponse is the standard error response body returned by plugin endpoints.
type ErrorResponse struct {
	Error string `json:"error"`
}
