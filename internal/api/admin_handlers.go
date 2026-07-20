package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"futdarapaziada/api/internal/store"
)

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	stats, err := s.store.DashboardStats(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	delinquents, err := s.store.DelinquentUsers(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	activity, err := s.store.RecentActivity(r.Context(), 10)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	var next any
	match, err := s.store.NextMatch(r.Context())
	if err == nil {
		next = match
	} else if !errors.Is(err, store.ErrNotFound) {
		writeStoreError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"stats":       stats,
		"delinquents": orEmpty(delinquents),
		"activity":    orEmpty(activity),
		"next_match":  next,
	})
}

func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	users, total, err := s.store.ListUsers(r.Context(), store.UserFilter{
		Search:    q.Get("search"),
		Role:      q.Get("role"),
		Status:    q.Get("status"),
		Financial: q.Get("financial"),
		Page:      page,
		PerPage:   20,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": orEmpty(users), "total": total})
}

func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	targetID := r.PathValue("id")
	admin := currentUser(r)

	var body struct {
		Name   *string `json:"name"`
		Phone  *string `json:"phone"`
		Email  *string `json:"email"`
		Role   *string `json:"role"`
		Status *string `json:"status"`
		Reason string  `json:"reason"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Role != nil && *body.Role != "admin" && *body.Role != "player" {
		writeError(w, http.StatusBadRequest, "role deve ser admin ou player")
		return
	}

	err := s.store.UpdateUser(r.Context(), targetID, store.UserUpdate{
		Name: body.Name, Phone: body.Phone, Email: body.Email, Role: body.Role,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}

	if body.Status != nil {
		switch *body.Status {
		case "active", "inactive", "archived":
		default:
			writeError(w, http.StatusBadRequest, "status deve ser active, inactive ou archived")
			return
		}
		if err := s.store.ChangeUserStatus(r.Context(), targetID, *body.Status, body.Reason, &admin.ID); err != nil {
			writeStoreError(w, err)
			return
		}
		target, err := s.store.UserByID(r.Context(), targetID)
		if err == nil {
			s.store.LogActivity(r.Context(), &admin.ID, "user_status",
				fmt.Sprintf("%s alterou o status de %s para %s", admin.Name, target.Name, statusLabel(*body.Status)))
		}
	}

	updated, err := s.store.UserByID(r.Context(), targetID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func statusLabel(status string) string {
	switch status {
	case "active":
		return "ativo"
	case "inactive":
		return "inativo"
	case "archived":
		return "arquivado"
	}
	return status
}

func (s *Server) handleCreateInvite(w http.ResponseWriter, r *http.Request) {
	var body struct {
		InvitedName string `json:"invited_name"`
		Role        string `json:"role"`
		ValidDays   int    `json:"valid_days"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Role == "" {
		body.Role = "player"
	}
	if body.Role != "admin" && body.Role != "player" {
		writeError(w, http.StatusBadRequest, "role deve ser admin ou player")
		return
	}
	if body.ValidDays <= 0 {
		body.ValidDays = s.store.SettingInt(r.Context(), "invite_valid_days", 7)
	}

	admin := currentUser(r)
	invite, err := s.store.CreateInvite(r.Context(), NewInviteToken(), body.InvitedName, body.Role, admin.ID, body.ValidDays)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, invite)
}

func (s *Server) handleListInvites(w http.ResponseWriter, r *http.Request) {
	invites, err := s.store.ListInvites(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(invites))
}

func (s *Server) handleRevokeInvite(w http.ResponseWriter, r *http.Request) {
	if err := s.store.RevokeInvite(r.Context(), r.PathValue("id")); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := s.store.Settings(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var body map[string]string
	if !decodeJSON(w, r, &body) {
		return
	}
	for key, value := range body {
		if err := s.store.SetSetting(r.Context(), key, value); err != nil {
			writeStoreError(w, err)
			return
		}
	}
	settings, err := s.store.Settings(r.Context())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleActivity(w http.ResponseWriter, r *http.Request) {
	activity, err := s.store.RecentActivity(r.Context(), 15)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(activity))
}
