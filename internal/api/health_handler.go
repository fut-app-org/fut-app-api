package api

import (
	"context"
	"log"
	"net/http"
	"time"
)

// handleHealth confirms that both the HTTP process and PostgreSQL are ready.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		log.Printf("healthcheck PostgreSQL falhou: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC(),
	})
}
