package extensionapi

import "net/http"

// handleHealth responds to health check requests
func (s *ExtensionServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, err := w.Write([]byte(`{"status":"ok"}`))

	if err != nil {
		WriteError(w, http.StatusInternalServerError, "failed to write response")
	}
}
