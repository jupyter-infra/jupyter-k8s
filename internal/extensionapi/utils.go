package extensionapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

// WriteError writes an error response in Kubernetes Status format
func WriteError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	status := map[string]interface{}{
		"kind":       "Status",
		"apiVersion": "v1",
		"status":     "Failure",
		"message":    message,
		"code":       statusCode,
	}
	json.NewEncoder(w).Encode(status)
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
