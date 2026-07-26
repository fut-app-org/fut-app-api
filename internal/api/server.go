// Package api monta o servidor HTTP: rotas, middlewares e handlers.
package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"futdarapaziada/api/internal/auth"
	"futdarapaziada/api/internal/config"
	"futdarapaziada/api/internal/mercadopago"
	"futdarapaziada/api/internal/store"
)

type Server struct {
	cfg          config.Config
	store        *store.Store
	mercadoPago  *mercadopago.Client
	loginLimiter *auth.LoginLimiter
}

func NewServer(cfg config.Config, st *store.Store) *Server {
	return &Server{
		cfg:          cfg,
		store:        st,
		mercadoPago:  mercadopago.New(cfg.MercadoPagoAccessToken),
		loginLimiter: auth.NewLoginLimiter(10, 15*time.Minute),
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.Timeout(120 * time.Second))

	r.Route("/api", func(r chi.Router) {
		// Rotas públicas
		r.Get("/healthz", s.handleHealth)
		r.Get("/invites/{token}", s.handleGetInvite)
		r.Post("/invites/{token}/signup", s.handleSignup)
		r.Post("/login", s.handleLogin)
		r.Post("/logout", s.handleLogout)
		r.Post("/webhooks/mercadopago", s.handleMercadoPagoWebhook)

		// Autenticado — acessível também a usuários inativos (consultar bloqueio e pagar)
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth)
			r.Get("/me", s.handleMe)
			r.Patch("/me", s.handleUpdateMe)
			r.Get("/charges/me", s.handleMyCharges)
			r.Post("/charges/{id}/pix", s.handleCreatePixCharge)
			r.Get("/media/files/{matchId}/{filename}", s.handleServeMedia)
		})

		// Autenticado e ativo — operações do dia a dia
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth, s.requireActive)
			r.Get("/matches/next", s.handleNextMatch)
			r.Get("/matches", s.handleListMatches)
			r.Get("/matches/{id}", s.handleGetMatch)
			r.Post("/matches/{id}/confirm", s.handleConfirm)
			r.Get("/matches/{id}/confirmations", s.handleConfirmations)
			r.Get("/matches/{id}/teams", s.handleTeams)
			r.Post("/matches/{id}/votes", s.handleVote)
			r.Get("/matches/{id}/media", s.handleListMedia)
			r.Post("/matches/{id}/media", s.handleUploadMedia)
			r.Delete("/media/{id}", s.handleDeleteMedia)
			r.Get("/activity", s.handleActivity)
		})

		// Administração
		r.Group(func(r chi.Router) {
			r.Use(s.requireAuth, s.requireActive, s.requireAdmin)
			r.Get("/admin/dashboard", s.handleDashboard)

			r.Get("/admin/users", s.handleListUsers)
			r.Patch("/admin/users/{id}", s.handleUpdateUser)

			r.Post("/admin/invites", s.handleCreateInvite)
			r.Get("/admin/invites", s.handleListInvites)
			r.Post("/admin/invites/{id}/revoke", s.handleRevokeInvite)

			r.Post("/admin/matches", s.handleCreateMatch)
			r.Patch("/admin/matches/{id}", s.handleUpdateMatch)
			r.Post("/matches/{id}/cancel", s.handleCancelMatch)
			r.Post("/matches/{id}/close-confirmations", s.handleCloseConfirmations)
			r.Post("/matches/{id}/reopen-confirmations", s.handleReopenConfirmations)
			r.Post("/matches/{id}/draw-teams", s.handleDrawTeams)
			r.Post("/matches/{id}/finish", s.handleFinishMatch)
			r.Post("/matches/{id}/close-voting", s.handleCloseVoting)

			r.Get("/admin/charges", s.handleAdminCharges)
			r.Post("/admin/charges/generate", s.handleGenerateCharges)
			r.Post("/admin/charges/{id}/whatsapp-reminder", s.handleWhatsAppReminder)
			r.Post("/admin/charges/{id}/mark-paid", s.handleMarkPaid)
			r.Post("/admin/charges/{id}/cancel", s.handleCancelCharge)
			r.Post("/admin/charges/{id}/exempt", s.handleExemptCharge)

			r.Get("/admin/settings", s.handleGetSettings)
			r.Put("/admin/settings", s.handleUpdateSettings)
		})
	})

	return r
}
