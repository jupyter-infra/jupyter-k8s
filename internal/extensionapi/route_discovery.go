package extensionapi

import "net/http"

func handleDiscovery(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{
		"kind": "APIResourceList",
		"apiVersion": "v1",
		"groupVersion": "connection.workspaces.jupyter.org/v1alpha1",
		"resources": [{
			"name": "connections",
			"singularName": "connection",
			"namespaced": true,
			"kind": "WorkspaceConnection",
			"verbs": ["create"]
		}]
	}`))
}
