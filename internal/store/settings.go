package store

import (
	"context"
	"strconv"
)

func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `select key, value from settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settings := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		settings[k] = v
	}
	return settings, rows.Err()
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.pool.Exec(ctx, `
		insert into settings (key, value) values ($1, $2)
		on conflict (key) do update set value = $2`, key, value)
	return err
}

// SettingInt lê uma configuração numérica com valor padrão.
func (s *Store) SettingInt(ctx context.Context, key string, fallback int) int {
	var raw string
	if err := s.pool.QueryRow(ctx, `select value from settings where key = $1`, key).Scan(&raw); err != nil {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}
