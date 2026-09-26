package api

import (
	"net/http"

	"github.com/google/uuid"
)

func (s *Server) handleListTeamPresets(w http.ResponseWriter, r *http.Request) {
	presets, err := s.store.ListTeamPresets(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(presets))
}

// decodeTeamPresetBody valida nome e IDs de membros comuns ao create/update.
func decodeTeamPresetBody(w http.ResponseWriter, r *http.Request) (string, []string, bool) {
	var body struct {
		Name      string   `json:"name"`
		MemberIDs []string `json:"member_ids"`
	}
	if !decodeJSON(w, r, &body) {
		return "", nil, false
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "nome é obrigatório")
		return "", nil, false
	}
	for _, id := range body.MemberIDs {
		if _, err := uuid.Parse(id); err != nil {
			writeError(w, http.StatusBadRequest, "member_ids contém ID inválido")
			return "", nil, false
		}
	}
	return body.Name, body.MemberIDs, true
}

func (s *Server) handleCreateTeamPreset(w http.ResponseWriter, r *http.Request) {
	name, memberIDs, ok := decodeTeamPresetBody(w, r)
	if !ok {
		return
	}
	preset, err := s.store.CreateTeamPreset(r.Context(), name, memberIDs, currentUser(r).ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, preset)
}

func (s *Server) handleUpdateTeamPreset(w http.ResponseWriter, r *http.Request) {
	name, memberIDs, ok := decodeTeamPresetBody(w, r)
	if !ok {
		return
	}
	if err := s.store.UpdateTeamPreset(r.Context(), r.PathValue("id"), name, memberIDs); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteTeamPreset(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteTeamPreset(r.Context(), r.PathValue("id")); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
