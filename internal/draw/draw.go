// Package draw sorteia os times de uma partida a partir dos confirmados.
package draw

import (
	"errors"
	"fmt"
	"math/rand/v2"

	"futdarapaziada/api/internal/store"
)

// Presets de identidade visual dos times, na ordem em que são criados.
var presets = []struct{ Name, Color string }{
	{"Time Colete Verde", "#C8F14B"},
	{"Time Colete Laranja", "#F59E0B"},
	{"Time Colete Azul", "#3B82F6"},
	{"Time Colete Preto", "#1F2937"},
}

// MaxTeams é o limite de times por partida (um por preset).
const MaxTeams = 4

// Empty monta os times vazios com nome, cor e posição dos presets visuais.
func Empty(teamCount int) []store.Team {
	if teamCount < 2 {
		teamCount = 2
	}
	if teamCount > MaxTeams {
		teamCount = MaxTeams
	}
	teams := make([]store.Team, teamCount)
	for i := range teams {
		teams[i] = store.Team{
			TeamName:  presets[i].Name,
			TeamColor: presets[i].Color,
			Position:  i,
			Members:   []store.TeamMember{},
		}
	}
	return teams
}

// Teams embaralha os jogadores e distribui em round-robin, o que garante
// diferença máxima de um jogador entre os times.
func Teams(players []store.TeamMember, teamCount int) []store.Team {
	teams := Empty(teamCount)
	teamCount = len(teams)

	shuffled := make([]store.TeamMember, len(players))
	copy(shuffled, players)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	for i, p := range shuffled {
		t := &teams[i%teamCount]
		t.Members = append(t.Members, p)
	}
	return teams
}

// Manual monta os times a partir de uma escalação explícita: cada entrada de
// assignment é um time com os IDs dos jogadores. Todo confirmado precisa
// aparecer exatamente uma vez.
func Manual(players []store.TeamMember, assignment [][]string) ([]store.Team, error) {
	if len(assignment) < 2 || len(assignment) > MaxTeams {
		return nil, fmt.Errorf("escalação deve ter entre 2 e %d times", MaxTeams)
	}

	byID := make(map[string]store.TeamMember, len(players))
	for _, p := range players {
		byID[p.UserID] = p
	}

	teams := Empty(len(assignment))
	seen := make(map[string]bool, len(players))
	for i, ids := range assignment {
		for _, id := range ids {
			p, ok := byID[id]
			if !ok {
				return nil, errors.New("escalação inclui jogador que não confirmou presença")
			}
			if seen[id] {
				return nil, fmt.Errorf("jogador %s escalado em mais de um time", p.Name)
			}
			seen[id] = true
			teams[i].Members = append(teams[i].Members, p)
		}
	}
	if len(seen) != len(players) {
		return nil, errors.New("todo confirmado precisa estar escalado em algum time")
	}
	return teams, nil
}
