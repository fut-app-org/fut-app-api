package api

import (
	"context"
	"net/http"

	"futdarapaziada/api/internal/auth"
	"futdarapaziada/api/internal/store"
)

type contextKey string

const userKey contextKey = "user"

// currentUser retorna o usuário autenticado colocado no contexto pelo requireAuth.
func currentUser(r *http.Request) store.User {
	return r.Context().Value(userKey).(store.User)
}

// requireAuth valida o cookie de sessão e carrega o usuário do banco a cada
// requisição, para que inativações tenham efeito imediato.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.CookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "sessão ausente")
			return
		}
		userID, sessionVersion, err := auth.ParseSessionToken(s.cfg.JWTSecret, cookie.Value)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "sessão inválida ou expirada")
			return
		}
		user, err := s.store.UserByID(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "usuário não encontrado")
			return
		}
		if user.Status == "archived" {
			writeError(w, http.StatusForbidden, "conta arquivada")
			return
		}
		if user.SessionVersion != sessionVersion {
			writeError(w, http.StatusUnauthorized, "sessÃ£o expirada")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey, user)))
	})
}

// requireActive bloqueia usuários inativos nas rotas operacionais. Eles ainda
// enxergam as rotas fora deste middleware (perfil, cobranças, logout) para
// consultar o motivo do bloqueio e pagar o que está pendente.
func (s *Server) requireActive(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).Status != "active" {
			writeErrorCode(w, http.StatusForbidden,
				"seu acesso está bloqueado; regularize o pagamento pendente", "inactive")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r).Role != "admin" {
			writeError(w, http.StatusForbidden, "apenas administradores")
			return
		}
		next.ServeHTTP(w, r)
	})
}
