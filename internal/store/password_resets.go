package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// CreatePasswordReset invalidates previous unused tokens before saving a new one.
func (s *Store) CreatePasswordReset(ctx context.Context, email, tokenHash string, expiresAt time.Time) (User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, err
	}
	defer tx.Rollback(ctx)

	var userID string
	err = tx.QueryRow(ctx, `select id from users where lower(email) = lower($1) and status <> 'archived' for update`, email).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `update password_reset_tokens set used_at = now() where user_id = $1 and used_at is null`, userID); err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `insert into password_reset_tokens (user_id, token_hash, expires_at) values ($1, $2, $3)`, userID, tokenHash, expiresAt); err != nil {
		return User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, err
	}
	return s.UserByID(ctx, userID)
}

// ResetPassword consumes a valid token and invalidates every existing session.
func (s *Store) ResetPassword(ctx context.Context, tokenHash, passwordHash string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var userID string
	err = tx.QueryRow(ctx, `select user_id from password_reset_tokens where token_hash = $1 and used_at is null and expires_at > now() for update`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `update password_reset_tokens set used_at = now() where token_hash = $1`, tokenHash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `update users set password_hash = $2, session_version = session_version + 1, updated_at = now() where id = $1`, userID, passwordHash); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
