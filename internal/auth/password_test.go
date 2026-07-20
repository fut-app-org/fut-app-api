package auth

import "testing"

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{name: "accepts uppercase digit and symbol", password: "Senha123@"},
		{name: "accepts uppercase digit without symbol", password: "Senha123"},
		{name: "requires eight characters", password: "Abc123", wantErr: ErrPasswordTooShort},
		{name: "requires uppercase letter", password: "senha123", wantErr: ErrPasswordNoUpper},
		{name: "requires digit", password: "SenhaSem", wantErr: ErrPasswordNoDigit},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePassword(tt.password); err != tt.wantErr {
				t.Fatalf("ValidatePassword(%q) error = %v, want %v", tt.password, err, tt.wantErr)
			}
		})
	}
}
