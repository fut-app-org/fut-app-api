package auth

import "testing"

func TestSessionTokenPreservesSessionVersion(t *testing.T) {
	const sessionVersion = 3

	token, _, err := NewSessionToken([]byte("test-secret"), "user-123", sessionVersion)
	if err != nil {
		t.Fatal(err)
	}

	userID, gotVersion, err := ParseSessionToken([]byte("test-secret"), token)
	if err != nil {
		t.Fatal(err)
	}
	if userID != "user-123" {
		t.Errorf("userID = %q, want user-123", userID)
	}
	if gotVersion != sessionVersion {
		t.Errorf("session version = %d, want %d", gotVersion, sessionVersion)
	}
}

func TestSessionTokenRejectsDifferentSecret(t *testing.T) {
	token, _, err := NewSessionToken([]byte("test-secret"), "user-123", 1)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := ParseSessionToken([]byte("another-secret"), token); err == nil {
		t.Fatal("ParseSessionToken() error = nil, want invalid signature error")
	}
}
