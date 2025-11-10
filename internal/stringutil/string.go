// Package stringutil provides string utility functions.
package stringutil

import "encoding/json"

// SanitizeUsername sanitizes a username by properly escaping it
func SanitizeUsername(username string) string {
	// Use Go's JSON marshaling to properly escape the string
	escaped, _ := json.Marshal(username)
	// Remove the surrounding quotes that json.Marshal adds
	return string(escaped[1 : len(escaped)-1])
}
