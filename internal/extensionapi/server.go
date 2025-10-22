package extensionapi

import (
	"net/http"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupExtensionAPIServerWithManager sets up the extension API server
func SetupExtensionAPIServerWithManager() error {
	setupLog := ctrl.Log.WithName("extension-api")
	
	go func() {
		setupLog.Info("Extension server goroutine started")
		
		http.HandleFunc("/apis/connection.workspaces.jupyter.org/", func(w http.ResponseWriter, r *http.Request) {
			setupLog.Info("API request", "method", r.Method, "path", r.URL.Path)
			
			if r.URL.Path == "/apis/connection.workspaces.jupyter.org/v1alpha1" {
				handleDiscovery(w, r)
				return
			}
			
			// Handle CREATE requests for connections
			if r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/apis/connection.workspaces.jupyter.org/v1alpha1/namespaces/") {
				handleConnectionCreate(w, r)
				return
			}
			
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte(`{"error": "API endpoint not found"}`))
		})
		
		setupLog.Info("Extension API server starting on :8444")
		if err := http.ListenAndServeTLS(":8444", "/tmp/extension-server/serving-certs/tls.crt", "/tmp/extension-server/serving-certs/tls.key", nil); err != nil {
			setupLog.Error(err, "Extension API server failed")
		}
	}()
	
	return nil
}

func handleDiscovery(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{
		"kind": "APIResourceList",
		"apiVersion": "v1",
		"groupVersion": "connection.workspaces.jupyter.org/v1alpha1",
		"resources": [{
			"name": "connections",
			"singularName": "connection",
			"namespaced": true,
			"kind": "Connection",
			"verbs": ["create"]
		}]
	}`))
}

func handleConnectionCreate(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement connection creation logic
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(`{"status": "created"}`))
}
