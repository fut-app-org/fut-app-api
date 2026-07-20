package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const matchColumns = `
	m.id, m.match_date::text, to_char(m.start_time, 'HH24:MI'), to_char(m.end_time, 'HH24:MI'),
	m.venue, m.address, m.confirmation_deadline, m.status, m.cancel_reason, m.notes,
	m.voting_closes_at, m.finished_at, m.created_at,
	(select count(*) from match_confirmations c where c.match_id = m.id and c.response = 'going'),
	(select count(*) from match_confirmations c where c.match_id = m.id and c.response = 'not_going'),
	(select count(*) from match_confirmations c where c.match_id = m.id and c.response = 'no_response'),
	(select count(*) from match_media md where md.match_id = m.id and md.status = 'visible')`

func scanMatch(row pgx.Row) (Match, error) {
	var m Match
	err := row.Scan(&m.ID, &m.MatchDate, &m.StartTime, &m.EndTime, &m.Venue, &m.Address,
		&m.ConfirmationDeadline, &m.Status, &m.CancelReason, &m.Notes,
		&m.VotingClosesAt, &m.FinishedAt, &m.CreatedAt,
		&m.GoingCount, &m.NotGoingCount, &m.NoResponseCount, &m.MediaCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return m, ErrNotFound
	}
	return m, err
}

type MatchInput struct {
	MatchDate            string // YYYY-MM-DD
	StartTime            string // HH:MM
	EndTime              string
	Venue                string
	Address              string
	ConfirmationDeadline time.Time
	Notes                string
}

// CreateMatch cria a partida com confirmação aberta e uma linha de confirmação
// "sem resposta" para cada usuário ativo.
func (s *Store) CreateMatch(ctx context.Context, in MatchInput, createdBy string) (Match, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Match{}, err
	}
	defer tx.Rollback(ctx)

	var id string
	err = tx.QueryRow(ctx, `
		insert into matches (match_date, start_time, end_time, venue, address, confirmation_deadline, notes, created_by)
		values ($1, $2::time, $3::time, $4, $5, $6, $7, $8) returning id`,
		in.MatchDate, in.StartTime, in.EndTime, in.Venue, in.Address, in.ConfirmationDeadline, in.Notes, createdBy).Scan(&id)
	if err != nil {
		return Match{}, err
	}
	_, err = tx.Exec(ctx, `
		insert into match_confirmations (match_id, user_id)
		select $1, id from users where status = 'active'`, id)
	if err != nil {
		return Match{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Match{}, err
	}
	return s.MatchByID(ctx, id)
}

func (s *Store) MatchByID(ctx context.Context, id string) (Match, error) {
	return scanMatch(s.pool.QueryRow(ctx, `select `+matchColumns+` from matches m where m.id = $1`, id))
}

// NextMatch é a próxima partida não finalizada/cancelada, a mais próxima no futuro
// (ou de hoje).
func (s *Store) NextMatch(ctx context.Context) (Match, error) {
	return scanMatch(s.pool.QueryRow(ctx, `
		select `+matchColumns+` from matches m
		where m.status in ('open', 'closed', 'teams_drawn') and m.match_date >= current_date
		order by m.match_date, m.start_time limit 1`))
}

// ListMatches retorna o histórico (mais recentes primeiro). month opcional em YYYY-MM.
func (s *Store) ListMatches(ctx context.Context, month string) ([]Match, error) {
	query := `select ` + matchColumns + ` from matches m`
	args := []any{}
	if month != "" {
		query += ` where to_char(m.match_date, 'YYYY-MM') = $1`
		args = append(args, month)
	}
	query += ` order by m.match_date desc, m.start_time desc`

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var matches []Match
	for rows.Next() {
		m, err := scanMatch(rows)
		if err != nil {
			return nil, err
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

type MatchUpdate struct {
	MatchDate            *string
	StartTime            *string
	EndTime              *string
	Venue                *string
	Address              *string
	ConfirmationDeadline *time.Time
	Notes                *string
}

func (s *Store) UpdateMatch(ctx context.Context, id string, up MatchUpdate) error {
	_, err := s.pool.Exec(ctx, `
		update matches set
			match_date = coalesce($2::date, match_date),
			start_time = coalesce($3::time, start_time),
			end_time   = coalesce($4::time, end_time),
			venue      = coalesce($5, venue),
			address    = coalesce($6, address),
			confirmation_deadline = coalesce($7, confirmation_deadline),
			notes      = coalesce($8, notes)
		where id = $1`,
		id, up.MatchDate, up.StartTime, up.EndTime, up.Venue, up.Address, up.ConfirmationDeadline, up.Notes)
	return err
}

// SetMatchStatus faz a transição de status validando o estado de origem permitido.
func (s *Store) SetMatchStatus(ctx context.Context, id, newStatus string, allowedFrom ...string) error {
	tag, err := s.pool.Exec(ctx,
		`update matches set status = $2 where id = $1 and status = any($3)`, id, newStatus, allowedFrom)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CancelMatch(ctx context.Context, id, reason string) error {
	tag, err := s.pool.Exec(ctx, `
		update matches set status = 'cancelled', cancel_reason = $2
		where id = $1 and status not in ('finished', 'cancelled')`, id, reason)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// FinishMatch encerra a partida e abre a votação até votingCloses.
func (s *Store) FinishMatch(ctx context.Context, id, finishedBy string, votingCloses time.Time) error {
	tag, err := s.pool.Exec(ctx, `
		update matches set status = 'voting', finished_at = now(), finished_by = $2, voting_closes_at = $3
		where id = $1 and status in ('closed', 'teams_drawn')`, id, finishedBy, votingCloses)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CloseExpiredConfirmations fecha partidas abertas cujo prazo passou; retorna os ids.
func (s *Store) CloseExpiredConfirmations(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		update matches set status = 'closed'
		where status = 'open' and confirmation_deadline <= now()
		returning id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectIDs(rows)
}

// CloseExpiredVoting finaliza partidas cuja votação venceu; retorna os ids.
func (s *Store) CloseExpiredVoting(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		update matches set status = 'finished'
		where status = 'voting' and voting_closes_at is not null and voting_closes_at <= now()
		returning id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectIDs(rows)
}

func collectIDs(rows pgx.Rows) ([]string, error) {
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

var ErrConfirmationClosed = errors.New("confirmação encerrada para esta partida")

// Confirm registra (ou altera) a resposta do usuário enquanto a confirmação está aberta.
func (s *Store) Confirm(ctx context.Context, matchID, userID, response string) error {
	var status string
	err := s.pool.QueryRow(ctx, `select status from matches where id = $1`, matchID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if status != "open" {
		return ErrConfirmationClosed
	}
	_, err = s.pool.Exec(ctx, `
		insert into match_confirmations (match_id, user_id, response, responded_at)
		values ($1, $2, $3, now())
		on conflict (match_id, user_id) do update set response = $3, responded_at = now()`,
		matchID, userID, response)
	return err
}

func (s *Store) Confirmations(ctx context.Context, matchID string) ([]ConfirmationEntry, error) {
	rows, err := s.pool.Query(ctx, `
		select c.user_id, u.name, u.avatar_color, u.role, c.response, c.responded_at
		from match_confirmations c
		join users u on u.id = c.user_id
		where c.match_id = $1
		order by c.response, u.name`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []ConfirmationEntry
	for rows.Next() {
		var e ConfirmationEntry
		if err := rows.Scan(&e.UserID, &e.Name, &e.AvatarColor, &e.Role, &e.Response, &e.RespondedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func (s *Store) IsParticipant(ctx context.Context, matchID, userID string) (bool, error) {
	var going bool
	err := s.pool.QueryRow(ctx, `
		select exists (select 1 from match_confirmations
		where match_id = $1 and user_id = $2 and response = 'going')`, matchID, userID).Scan(&going)
	return going, err
}

// ReplaceTeams grava um novo sorteio, descartando o anterior.
func (s *Store) ReplaceTeams(ctx context.Context, matchID string, teams []Team) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `delete from match_teams where match_id = $1`, matchID); err != nil {
		return err
	}
	for _, t := range teams {
		var teamID string
		err := tx.QueryRow(ctx, `
			insert into match_teams (match_id, team_name, team_color, position)
			values ($1, $2, $3, $4) returning id`,
			matchID, t.TeamName, t.TeamColor, t.Position).Scan(&teamID)
		if err != nil {
			return err
		}
		for _, m := range t.Members {
			if _, err := tx.Exec(ctx,
				`insert into match_team_members (team_id, user_id) values ($1, $2)`, teamID, m.UserID); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(ctx,
		`update matches set status = 'teams_drawn' where id = $1 and status in ('closed', 'teams_drawn')`, matchID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Teams(ctx context.Context, matchID string) ([]Team, error) {
	rows, err := s.pool.Query(ctx, `
		select t.id, t.match_id, t.team_name, t.team_color, t.position, u.id, u.name, u.avatar_color
		from match_teams t
		join match_team_members tm on tm.team_id = t.id
		join users u on u.id = tm.user_id
		where t.match_id = $1
		order by t.position, u.name`, matchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var teams []Team
	index := map[string]int{}
	for rows.Next() {
		var t Team
		var m TeamMember
		if err := rows.Scan(&t.ID, &t.MatchID, &t.TeamName, &t.TeamColor, &t.Position, &m.UserID, &m.Name, &m.AvatarColor); err != nil {
			return nil, err
		}
		i, ok := index[t.ID]
		if !ok {
			teams = append(teams, t)
			i = len(teams) - 1
			index[t.ID] = i
		}
		teams[i].Members = append(teams[i].Members, m)
	}
	return teams, rows.Err()
}
