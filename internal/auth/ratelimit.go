package auth

import (
	"sync"
	"time"
)

// LoginLimiter limita tentativas de login por chave (e-mail ou IP) numa janela
// deslizante, em memória — suficiente para uma instância única.
type LoginLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	max      int
	window   time.Duration
}

func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{attempts: map[string][]time.Time{}, max: max, window: window}
}

// Allow registra uma tentativa e diz se ela pode prosseguir.
func (l *LoginLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := time.Now().Add(-l.window)
	recent := l.attempts[key][:0]
	for _, t := range l.attempts[key] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= l.max {
		l.attempts[key] = recent
		return false
	}
	l.attempts[key] = append(recent, time.Now())
	return true
}

// Reset limpa as tentativas após um login bem-sucedido.
func (l *LoginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
