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

package authmiddleware

import (
	"fmt"
	"net/http"
	"strings"
)

// GetForwardedHost extracts the X-Forwarded-Host header from the request
func GetForwardedHost(r *http.Request) (string, error) {
	host := r.Header.Get(HeaderForwardedHost)
	if host == "" {
		return "", fmt.Errorf("missing %s header", HeaderForwardedHost)
	}
	return host, nil
}

// GetForwardedURI extracts the X-Forwarded-URI header from the request
func GetForwardedURI(r *http.Request) (string, error) {
	uri := r.Header.Get(HeaderForwardedURI)
	if uri == "" {
		return "", fmt.Errorf("missing %s header", HeaderForwardedURI)
	}
	return uri, nil
}

// ExtractSubdomain extracts the subdomain part from a host (before first dot)
func ExtractSubdomain(host string) string {
	parts := strings.Split(host, ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return host
}
