/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package workspace

import (
	"encoding/base32"
	"strings"
)

// EncodeNamespaceB32 encodes a namespace string to base32 for use in subdomains
// Returns lowercase base32 encoding without padding for DNS compatibility
func EncodeNamespaceB32(namespace string) string {
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte(namespace)))
}

// DecodeNamespaceB32 decodes a base32 encoded namespace string
// Returns the original namespace string or an error if decoding fails
func DecodeNamespaceB32(encoded string) (string, error) {
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(encoded))
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}
