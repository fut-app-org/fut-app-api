package store

import (
	"context"
)

// CastVote grava (ou troca) o voto do usuário na categoria.
func (s *Store) CastVote(ctx context.Context, matchID, voterID, category, candidateID string) error {
	_, err := s.pool.Exec(ctx, `
		insert into votes (match_id, voter_id, category, candidate_id)
		values ($1, $2, $3, $4)
		on conflict (match_id, voter_id, category) do update set candidate_id = $4, created_at = now()`,
		matchID, voterID, category, candidateID)
	return err
}

// MyVotes retorna categoria → candidato votado pelo usuário nesta partida.
func (s *Store) MyVotes(ctx context.Context, matchID, voterID string) (map[string]string, error) {
	rows, err := s.pool.Query(ctx,
		`select category, candidate_id from votes where match_id = $1 and voter_id = $2`, matchID, voterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	votes := map[string]string{}
	for rows.Next() {
		var category, candidate string
		if err := rows.Scan(&category, &candidate); err != nil {
			return nil, err
		}
		votes[category] = candidate
	}
	return votes, rows.Err()
}

// VoteResults apura as duas categorias; empate devolve mais de um vencedor.
func (s *Store) VoteResults(ctx context.Context, matchID string) ([]VoteResult, error) {
	results := []VoteResult{
		{Category: "top_scorer", Winners: []Winner{}},
		{Category: "worst_player", Winners: []Winner{}},
	}
	for i := range results {
		rows, err := s.pool.Query(ctx, `
			with tallies as (
				select v.candidate_id, count(*) as votes
				from votes v where v.match_id = $1 and v.category = $2
				group by v.candidate_id
			)
			select t.candidate_id, u.name, u.avatar_color, t.votes,
			       (select coalesce(sum(votes), 0) from tallies) as total
			from tallies t join users u on u.id = t.candidate_id
			where t.votes = (select max(votes) from tallies)
			order by u.name`, matchID, results[i].Category)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var w Winner
			var total int
			if err := rows.Scan(&w.UserID, &w.Name, &w.AvatarColor, &w.Votes, &total); err != nil {
				rows.Close()
				return nil, err
			}
			results[i].VoteCount = total
			results[i].Winners = append(results[i].Winners, w)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return results, nil
}
