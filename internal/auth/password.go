package auth

import (
	"errors"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const minPasswordLength = 8

var (
	ErrPasswordTooShort = errors.New("a senha deve ter pelo menos 8 caracteres")
	ErrPasswordTooLong  = errors.New("a senha deve ter no máximo 72 bytes")
	ErrPasswordNoUpper  = errors.New("a senha deve conter pelo menos uma letra maiúscula")
	ErrPasswordNoDigit  = errors.New("a senha deve conter pelo menos um número")
)

// ValidatePassword aplica a política mínima de senha do produto. Símbolos não
// são obrigatórios, mas são aceitos; por exemplo, Senha123@ é válida.
func ValidatePassword(plain string) error {
	if utf8.RuneCountInString(plain) < minPasswordLength {
		return ErrPasswordTooShort
	}
	if len(plain) > 72 {
		return ErrPasswordTooLong
	}

	var hasUpper, hasDigit bool
	for _, r := range plain {
		hasUpper = hasUpper || unicode.IsUpper(r)
		hasDigit = hasDigit || unicode.IsDigit(r)
	}
	if !hasUpper {
		return ErrPasswordNoUpper
	}
	if !hasDigit {
		return ErrPasswordNoDigit
	}
	return nil
}

func HashPassword(plain string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(hash), err
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
