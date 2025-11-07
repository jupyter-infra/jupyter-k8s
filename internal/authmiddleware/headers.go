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
