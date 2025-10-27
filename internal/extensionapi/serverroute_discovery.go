package extensionapi

import "net/http"

// handleDiscovery responds with API resource discovery information
func (s *ExtensionServer) handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(`{
		"kind": "APIResourceList",
		"apiVersion": "v1",
		"groupVersion": "connection.workspace.jupyter.org/v1alpha1",
		"resources": [{
			"name": "connections",
			"singularName": "connection",
			"namespaced": true,
			"kind": "Connection",
			"verbs": ["create"]
		}, {
			"name": "connectionaccessreviews",
			"singularName": "connectionaccessreview",
			"namespaced": true,
			"kind": "ConnectionAccessReview",
			"verbs": ["create"]
		}]
	}`))

	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write discovery body")
	}
}
