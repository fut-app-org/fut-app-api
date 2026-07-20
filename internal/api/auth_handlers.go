package api

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"net/http"
	"strings"
	"time"

	"futdarapaziada/api/internal/auth"
	"futdarapaziada/api/internal/store"
)

// avatarPalette segue os tons de avatar do mockup; a cor é derivada do nome.
var avatarPalette = []string{
	"#3B82A0", "#A0623B", "#6B4FA0", "#A03B62", "#4FA07E",
	"#8A6D3B", "#3B5FA0", "#A0503B", "#7E4FA0", "#4F8AA0", "#A08A3B", "#5FA03B",
}

func avatarColorFor(name string) string {
	h := fnv.New32a()
	h.Write([]byte(name))
	return avatarPalette[h.Sum32()%uint32(len(avatarPalette))]
}

func (s *Server) setSessionCookie(w http.ResponseWriter, userID string) error {
	token, expires, err := auth.NewSessionToken(s.cfg.JWTSecret, userID)
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.cfg.Env == "production",
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))

	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	if !s.loginLimiter.Allow(body.Email) || !s.loginLimiter.Allow("ip:"+ip) {
		writeError(w, http.StatusTooManyRequests, "muitas tentativas; aguarde alguns minutos")
		return
	}

	user, hash, err := s.store.UserByEmail(r.Context(), body.Email)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !auth.CheckPassword(hash, body.Password)) {
		writeError(w, http.StatusUnauthorized, "e-mail ou senha incorretos")
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if user.Status == "archived" {
		writeError(w, http.StatusForbidden, "conta arquivada; fale com um administrador")
		return
	}

	s.loginLimiter.Reset(body.Email)
	if err := s.setSessionCookie(w, user.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: auth.CookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	stats, err := s.store.UserStats(r.Context(), user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "stats": stats})
}

func (s *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name            *string `json:"name"`
		Phone           *string `json:"phone"`
		Email           *string `json:"email"`
		Password        *string `json:"password"`
		CurrentPassword string  `json:"current_password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	user := currentUser(r)

	if body.Password != nil {
		_, hash, err := s.store.UserByEmail(r.Context(), user.Email)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		if !auth.CheckPassword(hash, body.CurrentPassword) {
			writeError(w, http.StatusForbidden, "senha atual incorreta")
			return
		}
		newHash, err := auth.HashPassword(*body.Password)
		if err != nil {
			writeStoreError(w, err)
			return
		}
		if err := s.store.UpdatePassword(r.Context(), user.ID, newHash); err != nil {
			writeStoreError(w, err)
			return
		}
	}

	err := s.store.UpdateUser(r.Context(), user.ID, store.UserUpdate{
		Name: body.Name, Phone: body.Phone, Email: body.Email,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	updated, err := s.store.UserByID(r.Context(), user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// --- Convite e cadastro ---

func (s *Server) handleGetInvite(w http.ResponseWriter, r *http.Request) {
	invite, err := s.store.InviteByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	_ = s.store.TouchInviteAccess(r.Context(), invite.ID)

	status := inviteStatus(invite)
	writeJSON(w, http.StatusOK, map[string]any{
		"invited_name": invite.InvitedName,
		"creator_name": invite.CreatorName,
		"expires_at":   invite.ExpiresAt,
		"status":       status,
	})
}

func inviteStatus(in store.Invite) string {
	switch {
	case in.UsedAt != nil:
		return "used"
	case in.RevokedAt != nil:
		return "revoked"
	case time.Now().After(in.ExpiresAt):
		return "expired"
	default:
		return "pending"
	}
}

func (s *Server) handleSignup(w http.ResponseWriter, r *http.Request) {
	invite, err := s.store.InviteByToken(r.Context(), r.PathValue("token"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if st := inviteStatus(invite); st != "pending" {
		writeError(w, http.StatusConflict, "convite indisponível ("+st+")")
		return
	}

	var body struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	body.Email = strings.TrimSpace(strings.ToLower(body.Email))
	if body.Name == "" || body.Email == "" || len(body.Password) < 8 {
		writeError(w, http.StatusBadRequest, "nome, e-mail e senha (mínimo 8 caracteres) são obrigatórios")
		return
	}

	hash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	user, err := s.store.CreateUser(r.Context(), body.Name, body.Email, body.Phone, hash,
		avatarColorFor(body.Name), invite.Role)
	if err != nil {
		// e-mail duplicado é o caso mais provável aqui
		writeError(w, http.StatusConflict, "não foi possível criar a conta; e-mail já cadastrado?")
		return
	}
	if err := s.store.UseInvite(r.Context(), invite.ID, user.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	s.store.LogActivity(r.Context(), &user.ID, "signup",
		fmt.Sprintf("%s entrou no grupo pelo convite de %s", user.Name, invite.CreatorName))

	if err := s.setSessionCookie(w, user.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, user)
}

// NewInviteToken gera um token de convite curto e imprevisível, seguro para URL.
func NewInviteToken() string {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		panic(err) // rand.Read não falha em plataformas suportadas
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
