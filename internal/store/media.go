package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Store) AddMedia(ctx context.Context, matchID, uploadedBy, mediaType, url, caption string) (Media, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		insert into match_media (match_id, uploaded_by, type, url, caption)
		values ($1, $2, $3, $4, $5) returning id`,
		matchID, uploadedBy, mediaType, url, caption).Scan(&id)
	if err != nil {
		return Media{}, err
	}
	return s.MediaByID(ctx, id)
}

func (s *Store) MediaByID(ctx context.Context, id string) (Media, error) {
	var m Media
	err := s.pool.QueryRow(ctx, `
		select md.id, md.match_id, md.uploaded_by, u.name, md.type, md.url, md.thumbnail_url, md.caption, md.created_at
		from match_media md join users u on u.id = md.uploaded_by
		where md.id = $1 and md.status = 'visible'`, id).
		Scan(&m.ID, &m.MatchID, &m.UploadedBy, &m.UploaderName, &m.Type, &m.URL, &m.ThumbnailURL, &m.Caption, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

func (s *Store) MediaByMatch(ctx context.Context, matchID string) ([]Media, error) {
	rows, err := s.pool.Query(ctx, `
		select md.id, md.match_id, md.uploaded_by, u.name, md.type, md.url, md.thumbnail_url, md.caption, md.created_at
		from match_media md join users u on u.id = md.uploaded_by
		where md.match_id = $1 and md.status = 'visible'
		order by md.created_at`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var media []Media
	for rows.Next() {
		var m Media
		if err := rows.Scan(&m.ID, &m.MatchID, &m.UploadedBy, &m.UploaderName, &m.Type, &m.URL, &m.ThumbnailURL, &m.Caption, &m.CreatedAt); err != nil {
			return nil, err
		}
		media = append(media, m)
	}
	return media, rows.Err()
}

// RemoveMedia oculta a mídia (soft delete) preservando o registro.
func (s *Store) RemoveMedia(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx,
		`update match_media set status = 'removed' where id = $1 and status = 'visible'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
