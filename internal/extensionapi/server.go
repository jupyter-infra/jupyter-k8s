// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"net/http"
	"strings"

	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupExtensionAPIServerWithManager sets up the extension API server
func SetupExtensionAPIServerWithManager() error {
	setupLog := ctrl.Log.WithName("extension-api")

	// Initialize configuration
	config, err := NewConfig()
	if err != nil {
		return err
	}

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
				handleConnectionCreate(config)(w, r)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error": "API endpoint not found"}`))
		})

		setupLog.Info("Extension API server starting on :8444")
		if err := http.ListenAndServeTLS(":8444", "/tmp/extension-server/serving-certs/tls.crt", "/tmp/extension-server/serving-certs/tls.key", nil); err != nil {
			setupLog.Error(err, "Extension API server failed")
		}
	}()

	return nil
}
