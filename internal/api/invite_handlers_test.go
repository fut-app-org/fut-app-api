package api

import "testing"

func TestInviteMessage(t *testing.T) {
	got := inviteMessage("Bruno", "https://fut.example/convite/abc123")

	want := "Fala, Bruno! Você foi convidado pro Fut da Rapaziada. " +
		"Crie sua conta aqui: https://fut.example/convite/abc123"
	if got != want {
		t.Errorf("inviteMessage() = %q, want %q", got, want)
	}
}

// Convite sem nome preenchido ainda precisa gerar uma saudação legível.
func TestInviteMessageSemNome(t *testing.T) {
	got := inviteMessage("  ", "https://fut.example/convite/abc123")

	want := "Fala, jogador! Você foi convidado pro Fut da Rapaziada. " +
		"Crie sua conta aqui: https://fut.example/convite/abc123"
	if got != want {
		t.Errorf("inviteMessage() = %q, want %q", got, want)
	}
}

func TestInviteURL(t *testing.T) {
	// A barra final do APP_URL não pode virar "//convite".
	got := inviteURL("https://fut.example/", "abc123")

	want := "https://fut.example/convite/abc123"
	if got != want {
		t.Errorf("inviteURL() = %q, want %q", got, want)
	}
}
