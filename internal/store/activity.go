package store

import "context"

// LogActivity registra um evento para o feed "atividades recentes". Erros são
// ignorados de propósito: o feed é informativo e não deve derrubar a operação.
func (s *Store) LogActivity(ctx context.Context, actorID *string, kind, message string) {
	_, _ = s.pool.Exec(ctx,
		`insert into activity_log (actor_id, kind, message) values ($1, $2, $3)`, actorID, kind, message)
}

func (s *Store) RecentActivity(ctx context.Context, limit int) ([]Activity, error) {
	rows, err := s.pool.Query(ctx,
		`select id, kind, message, created_at from activity_log order by created_at desc limit $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.Kind, &a.Message, &a.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, a)
	}
	return items, rows.Err()
}
