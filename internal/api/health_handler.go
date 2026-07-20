package api

import (
	"net/http"
	"time"
)

// handleHealth confirms that the HTTP process is ready to receive requests.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}
