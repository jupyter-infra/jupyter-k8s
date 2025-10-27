package extensionapi

import (
	"fmt"
	"net/http"
)

// handleDiscovery responds with API resource discovery information
func (s *ExtensionServer) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := fmt.Sprintf(`{
		"kind": "APIResourceList",
		"apiVersion": "v1",
		"groupVersion": "%s",
		"resources": [{
			"name": "connections",
			"singularName": "connection",
			"namespaced": true,
			"kind": "%s",
			"verbs": ["create"]
		}, {
			"name": "connectionaccessreviews",
			"singularName": "connectionaccessreview",
			"namespaced": true,
			"kind": "ConnectionAccessReview",
			"verbs": ["create"]
		}]
	}`, WorkspaceConnectionAPIVersion, WorkspaceConnectionKind)

	_, err := w.Write([]byte(response))
	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write discovery body")
	}
}
