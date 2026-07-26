package api

import (
	"encoding/base64"
	"testing"
)

func TestNewPasswordResetToken(t *testing.T) {
	token, err := newPasswordResetToken()
	if err != nil {
		t.Fatal(err)
	}

	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("token is not base64url: %v", err)
	}
	if len(decoded) != 32 {
		t.Errorf("token bytes = %d, want 32", len(decoded))
	}
}

func TestHashPasswordResetToken(t *testing.T) {
	const token = "reset-token"

	if got, want := hashPasswordResetToken(token), hashPasswordResetToken(token); got != want {
		t.Errorf("hash is not deterministic: %q != %q", got, want)
	}
	if hashPasswordResetToken(token) == hashPasswordResetToken("another-token") {
		t.Error("different tokens produced the same hash")
	}
}
