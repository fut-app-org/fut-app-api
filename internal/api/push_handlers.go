package api

import (
	"futdarapaziada/api/internal/push"
	"net/http"
)

func (s *Server) handlePushConfig(w http.ResponseWriter, r *http.Request) {
	enabled := push.New(s.cfg.VAPIDPublicKey, s.cfg.VAPIDPrivateKey, s.cfg.VAPIDSubject).Enabled()
	writeJSON(w, http.StatusOK, map[string]any{"enabled": enabled, "public_key": s.cfg.VAPIDPublicKey})
}
func (s *Server) handlePushStatus(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if !decodeJSON(w, r, &body) {
		return
	}
	exists, err := s.store.HasPushSubscription(r.Context(), currentUser(r).ID, body.Endpoint)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"subscribed": exists})
}
func (s *Server) handleSubscribePush(w http.ResponseWriter, r *http.Request) {
	if !push.New(s.cfg.VAPIDPublicKey, s.cfg.VAPIDPrivateKey, s.cfg.VAPIDSubject).Enabled() {
		writeError(w, http.StatusServiceUnavailable, "notificações ainda não estão disponíveis")
		return
	}
	var sub push.Subscription
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if !decodeJSON(w, r, &sub) {
		return
	}
	if err := push.ValidateSubscription(sub); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SavePushSubscription(r.Context(), currentUser(r).ID, sub); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
func (s *Server) handleUnsubscribePush(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Endpoint string `json:"endpoint"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.store.DeletePushSubscription(r.Context(), currentUser(r).ID, body.Endpoint); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
