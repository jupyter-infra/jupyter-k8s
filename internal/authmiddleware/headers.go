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

import (
	"errors"
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

// ExtractBearerToken extracts a bearer token from an Authorization header
func ExtractBearerToken(authHeader string) (string, error) {
	if authHeader == "" {
		return "", errors.New("authorization header is empty")
	}

	if !strings.HasPrefix(authHeader, OIDCAuthHeaderPrefix) {
		return "", errors.New("authorization header is not a bearer token")
	}

	token := strings.TrimPrefix(authHeader, OIDCAuthHeaderPrefix)
	if token == "" {
		return "", errors.New("bearer token is empty")
	}

	return token, nil
}
