package extensionapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

// WriteError writes an error response in JSON format
func WriteError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	errorResponse := map[string]string{"error": message}
	_ = json.NewEncoder(w).Encode(errorResponse)
}

// WriteKubernetesError writes an error response in Kubernetes Status format
func WriteKubernetesError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	status := map[string]interface{}{
		"kind":       "Status",
		"apiVersion": "v1",
		"status":     "Failure",
		"message":    message,
		"code":       statusCode,
	}
	_ = json.NewEncoder(w).Encode(status)
}

// GetNamespaceFromPath extracts the namespace from a URL path using regex
// Path format expected: /apis/connection.workspace.jupyter.org/v1alpha1/namespaces/{namespace}/resource
func GetNamespaceFromPath(path string) (string, error) {
	re := regexp.MustCompile(`/namespaces/([^/]+)/`)
	matches := re.FindStringSubmatch(path)
	if len(matches) > 1 {
		return matches[1], nil
	}
	return "", fmt.Errorf("cannot find the namespace in URL")
}

// GetUserFromHeaders extracts user information from request headers
func GetUserFromHeaders(r *http.Request) string {
	// Try common user headers in order of preference
	if user := r.Header.Get("X-Remote-User"); user != "" {
		return user
	}
	if user := r.Header.Get("X-Forwarded-User"); user != "" {
		return user
	}
	if user := r.Header.Get("Remote-User"); user != "" {
		return user
	}
	return ""
}
