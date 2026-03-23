/*
Copyright (c) Amazon Web Services
Distributed under the terms of the MIT license
*/

package plugin

import "fmt"

// StatusError represents an HTTP error with a status code and message.
// Used by both the plugin server (handler returns it to set the HTTP status)
// and the plugin client (created when parsing a non-200 response).
type StatusError struct {
	Code    int
	Message string
}

func (e *StatusError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("plugin error (HTTP %d): %s", e.Code, e.Message)
	}
	return fmt.Sprintf("plugin error (HTTP %d)", e.Code)
}
