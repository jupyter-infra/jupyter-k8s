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

package extensionapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/jupyter-infra/jupyter-k8s/internal/stringutil"
	"k8s.io/apiserver/pkg/endpoints/request"
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

// GetUser extracts the user from Kubernetes request context or falls back to headers
func GetUser(r *http.Request) string {
	// Try to get user info from Kubernetes request context first
	if userInfo, ok := request.UserFrom(r.Context()); ok {
		if userInfo != nil && userInfo.GetName() != "" {
			return stringutil.SanitizeUsername(userInfo.GetName())
		}
	}

	// Fallback to headers for backward compatibility
	if user := r.Header.Get(HeaderUser); user != "" {
		return stringutil.SanitizeUsername(user)
	}
	if user := r.Header.Get(HeaderRemoteUser); user != "" {
		return stringutil.SanitizeUsername(user)
	}

	return ""
}
