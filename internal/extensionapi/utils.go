package extensionapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
)

type userInfoKey struct{}

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
		return user
	}
	if user := r.Header.Get(HeaderRemoteUser); user != "" {
		return user
	}
	return ""
}

// AddUserInfoToContext returns a new Context with the given user info
func AddUserInfoToContext(ctx context.Context, userInfo *UserInfo) context.Context {
	return context.WithValue(ctx, userInfoKey{}, userInfo)
}

// GetUserInfoFromContext returns the UserInfo stored in context
// Returns nil if none is found
func GetUserInfoFromContext(ctx context.Context) *UserInfo {
	if userInfo, ok := ctx.Value(userInfoKey{}).(*UserInfo); ok {
		return userInfo
	}
	return nil
}
