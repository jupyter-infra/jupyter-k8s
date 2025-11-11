package extensionapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/jupyter-ai-contrib/jupyter-k8s/internal/stringutil"
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

// GetUserFromHeaders extracts the user from request headers
func GetUserFromHeaders(r *http.Request) string {
	if user := r.Header.Get(HeaderUser); user != "" {
		return stringutil.SanitizeUsername(user)
	}
	if user := r.Header.Get(HeaderRemoteUser); user != "" {
		return stringutil.SanitizeUsername(user)
	}
	return ""
}
