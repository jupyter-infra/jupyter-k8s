package authmiddleware

import (
	"encoding/json"
	"net/http"
	"time"
)

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		s.logger.Error("Failed to encode health response", "error", err)
	}
}
