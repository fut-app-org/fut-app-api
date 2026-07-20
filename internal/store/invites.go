package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

const inviteColumns = `
	i.id, i.token, i.invited_name, i.role, i.created_by, u.name as creator_name,
	i.expires_at, i.used_at, i.used_by, i.revoked_at, i.access_count, i.created_at`

func scanInvite(row pgx.Row) (Invite, error) {
	var in Invite
	err := row.Scan(&in.ID, &in.Token, &in.InvitedName, &in.Role, &in.CreatedBy, &in.CreatorName,
		&in.ExpiresAt, &in.UsedAt, &in.UsedBy, &in.RevokedAt, &in.AccessCount, &in.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return in, ErrNotFound
	}
	return in, err
}

func (s *Store) CreateInvite(ctx context.Context, token, invitedName, role, createdBy string, validDays int) (Invite, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		insert into invites (token, invited_name, role, created_by, expires_at)
		values ($1, $2, $3, $4, now() + make_interval(days => $5))
		returning id`,
		token, invitedName, role, createdBy, validDays).Scan(&id)
	if err != nil {
		return Invite{}, err
	}
	return scanInvite(s.pool.QueryRow(ctx,
		`select `+inviteColumns+` from invites i join users u on u.id = i.created_by where i.id = $1`, id))
}

func (s *Store) InviteByToken(ctx context.Context, token string) (Invite, error) {
	return scanInvite(s.pool.QueryRow(ctx,
		`select `+inviteColumns+` from invites i join users u on u.id = i.created_by where i.token = $1`, token))
}

func (s *Store) InviteByID(ctx context.Context, id string) (Invite, error) {
	return scanInvite(s.pool.QueryRow(ctx,
		`select `+inviteColumns+` from invites i join users u on u.id = i.created_by where i.id = $1`, id))
}

func (s *Store) ListInvites(ctx context.Context) ([]Invite, error) {
	rows, err := s.pool.Query(ctx,
		`select `+inviteColumns+` from invites i join users u on u.id = i.created_by order by i.created_at desc`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []Invite
	for rows.Next() {
		in, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, in)
	}
	return invites, rows.Err()
}

func (s *Store) TouchInviteAccess(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `update invites set access_count = access_count + 1 where id = $1`, id)
	return err
}

func (s *Store) RevokeInvite(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`update invites set revoked_at = now() where id = $1 and used_at is null and revoked_at is null`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UseInvite marca o convite como consumido; falha se já usado, revogado ou expirado.
func (s *Store) UseInvite(ctx context.Context, id, usedBy string) error {
	tag, err := s.pool.Exec(ctx, `
		update invites set used_at = now(), used_by = $2
		where id = $1 and used_at is null and revoked_at is null and expires_at > now()`, id, usedBy)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CountPendingInvites(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`select count(*) from invites where used_at is null and revoked_at is null and expires_at > now()`).Scan(&n)
	return n, err
}
