// Package extensionapi provides extension API server functionality.
package extensionapi

import (
	"net/http"
)

// HandleConnectionCreate handles POST requests to create a connection
func (s *ExtensionServer) HandleConnectionCreate(w http.ResponseWriter, r *http.Request) {
	logger := GetLoggerFromContext(r.Context())

	if r.Method != "POST" {
		WriteError(w, http.StatusBadRequest, "Connection must use POST method")
		return
	}

	logger.Info("Handling Connection creation", "method", r.Method, "path", r.URL.Path)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status": "created"}`))
}
