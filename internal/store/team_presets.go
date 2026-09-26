package store

import "context"

// ListTeamPresets retorna os presets de times em ordem alfabética.
func (s *Store) ListTeamPresets(ctx context.Context) ([]TeamPreset, error) {
	rows, err := s.pool.Query(ctx, `
		select id, name, member_ids, created_at from team_presets order by name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var presets []TeamPreset
	for rows.Next() {
		var p TeamPreset
		if err := rows.Scan(&p.ID, &p.Name, &p.MemberIDs, &p.CreatedAt); err != nil {
			return nil, err
		}
		presets = append(presets, p)
	}
	return presets, rows.Err()
}

func (s *Store) CreateTeamPreset(ctx context.Context, name string, memberIDs []string, createdBy string) (TeamPreset, error) {
	var p TeamPreset
	err := s.pool.QueryRow(ctx, `
		insert into team_presets (name, member_ids, created_by) values ($1, $2, $3)
		returning id, name, member_ids, created_at`,
		name, memberIDs, createdBy).Scan(&p.ID, &p.Name, &p.MemberIDs, &p.CreatedAt)
	return p, err
}

func (s *Store) UpdateTeamPreset(ctx context.Context, id, name string, memberIDs []string) error {
	tag, err := s.pool.Exec(ctx, `
		update team_presets set name = $2, member_ids = $3 where id = $1`, id, name, memberIDs)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteTeamPreset(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `delete from team_presets where id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
