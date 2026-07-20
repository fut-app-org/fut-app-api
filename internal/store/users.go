package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("registro não encontrado")

// userColumns traz o usuário junto com o estado financeiro derivado das cobranças:
// delinquent = tem cobrança vencida; last_payment_at = último pagamento confirmado.
const userColumns = `
	u.id, u.name, u.email, u.phone, u.avatar_color, u.role, u.status, u.inactive_reason, u.created_at,
	exists (select 1 from charges c where c.user_id = u.id and c.status = 'overdue') as delinquent,
	(select max(c.paid_at) from charges c where c.user_id = u.id and c.status in ('paid','manual_paid')) as last_payment_at`

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.AvatarColor, &u.Role, &u.Status,
		&u.InactiveReason, &u.CreatedAt, &u.Delinquent, &u.LastPaymentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *Store) UserByID(ctx context.Context, id string) (User, error) {
	return scanUser(s.pool.QueryRow(ctx, `select `+userColumns+` from users u where u.id = $1`, id))
}

func (s *Store) UserByEmail(ctx context.Context, email string) (User, string, error) {
	var u User
	var hash string
	err := s.pool.QueryRow(ctx, `select `+userColumns+`, u.password_hash from users u where lower(u.email) = lower($1)`, email).
		Scan(&u.ID, &u.Name, &u.Email, &u.Phone, &u.AvatarColor, &u.Role, &u.Status,
			&u.InactiveReason, &u.CreatedAt, &u.Delinquent, &u.LastPaymentAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, "", ErrNotFound
	}
	return u, hash, err
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.pool.QueryRow(ctx, `select count(*) from users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(ctx context.Context, name, email, phone, passwordHash, avatarColor, role string) (User, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
		insert into users (name, email, phone, password_hash, avatar_color, role)
		values ($1, $2, $3, $4, $5, $6) returning id`,
		name, email, phone, passwordHash, avatarColor, role).Scan(&id)
	if err != nil {
		return User{}, err
	}
	return s.UserByID(ctx, id)
}

type UserFilter struct {
	Search    string
	Role      string // "", "admin", "player"
	Status    string // "", "active", "inactive", "archived"
	Financial string // "", "paid", "pending", "overdue"
	Page      int
	PerPage   int
}

func (s *Store) ListUsers(ctx context.Context, f UserFilter) ([]User, int, error) {
	where := []string{"true"}
	args := []any{}
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	if f.Search != "" {
		where = append(where, "(u.name ilike "+arg("%"+f.Search+"%")+" or u.email ilike "+arg("%"+f.Search+"%")+")")
	}
	if f.Role != "" {
		where = append(where, "u.role = "+arg(f.Role))
	}
	if f.Status != "" {
		where = append(where, "u.status = "+arg(f.Status))
	}
	switch f.Financial {
	case "paid":
		where = append(where, `not exists (select 1 from charges c where c.user_id = u.id and c.status in ('pending','overdue'))`)
	case "pending":
		where = append(where, `exists (select 1 from charges c where c.user_id = u.id and c.status = 'pending')`)
	case "overdue":
		where = append(where, `exists (select 1 from charges c where c.user_id = u.id and c.status = 'overdue')`)
	}

	cond := strings.Join(where, " and ")

	var total int
	if err := s.pool.QueryRow(ctx, `select count(*) from users u where `+cond, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if f.PerPage <= 0 {
		f.PerPage = 20
	}
	if f.Page <= 0 {
		f.Page = 1
	}
	query := `select ` + userColumns + ` from users u where ` + cond +
		` order by u.name limit ` + arg(f.PerPage) + ` offset ` + arg((f.Page-1)*f.PerPage)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, 0, err
		}
		users = append(users, u)
	}
	return users, total, rows.Err()
}

func (s *Store) ActiveUsers(ctx context.Context) ([]User, error) {
	rows, err := s.pool.Query(ctx, `select `+userColumns+` from users u where u.status = 'active' order by u.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

type UserUpdate struct {
	Name  *string
	Phone *string
	Email *string
	Role  *string
}

func (s *Store) UpdateUser(ctx context.Context, id string, up UserUpdate) error {
	_, err := s.pool.Exec(ctx, `
		update users set
			name  = coalesce($2, name),
			phone = coalesce($3, phone),
			email = coalesce($4, email),
			role  = coalesce($5, role),
			updated_at = now()
		where id = $1`,
		id, up.Name, up.Phone, up.Email, up.Role)
	return err
}

func (s *Store) UpdatePassword(ctx context.Context, id, hash string) error {
	_, err := s.pool.Exec(ctx, `update users set password_hash = $2, updated_at = now() where id = $1`, id, hash)
	return err
}

// ChangeUserStatus troca o status e registra a transição no histórico.
func (s *Store) ChangeUserStatus(ctx context.Context, userID, newStatus, reason string, changedBy *string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var previous string
	if err := tx.QueryRow(ctx, `select status from users where id = $1 for update`, userID).Scan(&previous); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	if previous == newStatus {
		return tx.Commit(ctx)
	}

	reasonForUser := ""
	if newStatus == "inactive" {
		reasonForUser = reason
	}
	if _, err := tx.Exec(ctx, `update users set status = $2, inactive_reason = $3, updated_at = now() where id = $1`,
		userID, newStatus, reasonForUser); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		insert into user_status_history (user_id, previous_status, new_status, reason, changed_by)
		values ($1, $2, $3, $4, $5)`,
		userID, previous, newStatus, reason, changedBy); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type UserStats struct {
	MatchesPlayed  int `json:"matches_played"`
	TopScorerCount int `json:"top_scorer_count"`
	WorstCount     int `json:"worst_count"`
}

// UserStats conta participações e títulos em partidas finalizadas. Um "título" é
// estar entre os mais votados da categoria (empate conta para todos).
func (s *Store) UserStats(ctx context.Context, userID string) (UserStats, error) {
	var st UserStats
	err := s.pool.QueryRow(ctx, `
		select count(*) from match_confirmations mc
		join matches m on m.id = mc.match_id
		where mc.user_id = $1 and mc.response = 'going' and m.status = 'finished'`, userID).
		Scan(&st.MatchesPlayed)
	if err != nil {
		return st, err
	}

	countTitles := func(category string) (int, error) {
		var n int
		err := s.pool.QueryRow(ctx, `
			with tallies as (
				select v.match_id, v.candidate_id, count(*) as votes,
				       max(count(*)) over (partition by v.match_id) as max_votes
				from votes v
				join matches m on m.id = v.match_id and m.status = 'finished'
				where v.category = $1
				group by v.match_id, v.candidate_id
			)
			select count(*) from tallies where candidate_id = $2 and votes = max_votes`,
			category, userID).Scan(&n)
		return n, err
	}
	if st.TopScorerCount, err = countTitles("top_scorer"); err != nil {
		return st, err
	}
	st.WorstCount, err = countTitles("worst_player")
	return st, err
}
