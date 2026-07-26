package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"futdarapaziada/api/internal/auth"
	"futdarapaziada/api/internal/store"
)

const passwordResetDuration = 30 * time.Minute

func (s *Server) handlePasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	email := strings.TrimSpace(strings.ToLower(body.Email))
	if email == "" || !s.mailer.Enabled() || !s.passwordResetLimiter.Allow(email) {
		writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
		return
	}

	token, err := newPasswordResetToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "erro ao preparar a recuperacao de senha")
		return
	}
	user, err := s.store.CreatePasswordReset(r.Context(), email, hashPasswordResetToken(token), time.Now().Add(passwordResetDuration))
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	resetURL := strings.TrimRight(s.cfg.AppURL, "/") + "/redefinir-senha?token=" + url.QueryEscape(token)
	if err := s.mailer.SendPasswordReset(r.Context(), user.Email, resetURL); err != nil {
		log.Printf("enviando recuperaÃ§Ã£o de senha para %s: %v", user.ID, err)
	}
	writeJSON(w, http.StatusAccepted, map[string]bool{"ok": true})
}

func (s *Server) handlePasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := auth.ValidatePassword(body.Password); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	err = s.store.ResetPassword(r.Context(), hashPasswordResetToken(body.Token), hash)
	if errors.Is(err, store.ErrNotFound) {
		writeError(w, http.StatusBadRequest, "link invÃ¡lido ou expirado")
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func newPasswordResetToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashPasswordResetToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
