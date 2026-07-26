// Package auth cuida de senhas, tokens de sessão e limite de tentativas de login.
package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const sessionDuration = 7 * 24 * time.Hour

// CookieName é o nome do cookie httpOnly que carrega o JWT de sessão.
const CookieName = "fut_session"

type SessionClaims struct {
	SessionVersion int `json:"sv"`
	jwt.RegisteredClaims
}

func NewSessionToken(secret []byte, userID string, sessionVersion int) (string, time.Time, error) {
	expires := time.Now().Add(sessionDuration)
	claims := SessionClaims{
		SessionVersion: sessionVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			ExpiresAt: jwt.NewNumericDate(expires),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	return token, expires, err
}

// ParseSessionToken valida o JWT e devolve o id do usuário.
func ParseSessionToken(secret []byte, raw string) (string, int, error) {
	token, err := jwt.ParseWithClaims(raw, &SessionClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("método de assinatura inesperado: %v", t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return "", 0, err
	}
	claims, ok := token.Claims.(*SessionClaims)
	if !ok || claims.Subject == "" || claims.SessionVersion < 1 {
		return "", 0, fmt.Errorf("token sem subject")
	}
	return claims.Subject, claims.SessionVersion, nil
}
