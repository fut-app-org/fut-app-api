package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"futdarapaziada/api/internal/notify"
)

// inviteURL monta o link público de cadastro a partir do APP_URL configurado.
func inviteURL(appURL, token string) string {
	return strings.TrimRight(appURL, "/") + "/convite/" + token
}

// inviteMessage é o texto enviado ao convidado pelo WhatsApp (Evolution Go).
func inviteMessage(invitedName, url string) string {
	name := strings.TrimSpace(invitedName)
	if name == "" {
		name = "jogador"
	}
	return "Fala, " + name + "! Você foi convidado pro Fut da Rapaziada. " +
		"Crie sua conta aqui: " + url
}

// handleInviteWhatsAppSend manda o link do convite direto pelo Evolution Go e
// registra o envio na fila de notifications, como já é feito nas cobranças.
func (s *Server) handleInviteWhatsAppSend(w http.ResponseWriter, r *http.Request) {
	invite, err := s.store.InviteByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if invite.UsedAt != nil || invite.RevokedAt != nil || invite.ExpiresAt.Before(time.Now()) {
		writeError(w, http.StatusBadRequest, "convite não está mais válido")
		return
	}
	phone, err := notify.NormalizeNumber(invite.Phone)
	if err != nil {
		writeError(w, http.StatusBadRequest, "convite sem WhatsApp válido; edite o convite ou copie o link")
		return
	}

	message := inviteMessage(invite.InvitedName, inviteURL(s.cfg.AppURL, invite.Token))
	admin := currentUser(r)

	// Convite não pertence a um usuário/cobrança ainda, então vai sem vínculo.
	notificationID, err := s.store.ScheduleNotification(r.Context(), admin.ID, nil, phone, message, time.Now())
	if err != nil {
		writeStoreError(w, err)
		return
	}

	providerID, err := s.sender.Send(phone, message)
	if err != nil {
		_ = s.store.MarkNotificationFailed(r.Context(), notificationID, err.Error())
		s.store.LogActivity(r.Context(), &admin.ID, "invite_whatsapp_failed",
			fmt.Sprintf("falha ao enviar convite de %s por WhatsApp: %v", invite.InvitedName, err))
		writeError(w, http.StatusBadGateway, fmt.Sprintf("falha ao enviar WhatsApp: %v", err))
		return
	}

	_ = s.store.MarkNotificationSent(r.Context(), notificationID, providerID)
	s.store.LogActivity(r.Context(), &admin.ID, "invite_whatsapp_sent",
		fmt.Sprintf("%s enviou o convite de %s por WhatsApp", admin.Name, invite.InvitedName))

	writeJSON(w, http.StatusOK, map[string]string{
		"message":             message,
		"provider_message_id": providerID,
	})
}
