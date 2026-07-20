// Package draw sorteia os times de uma partida a partir dos confirmados.
package draw

import (
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

// Teams embaralha os jogadores e distribui em round-robin, o que garante
// diferença máxima de um jogador entre os times.
func Teams(players []store.TeamMember, teamCount int) []store.Team {
	if teamCount < 2 {
		teamCount = 2
	}
	if teamCount > MaxTeams {
		teamCount = MaxTeams
	}

	shuffled := make([]store.TeamMember, len(players))
	copy(shuffled, players)
	rand.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	teams := make([]store.Team, teamCount)
	for i := range teams {
		teams[i] = store.Team{
			TeamName:  presets[i].Name,
			TeamColor: presets[i].Color,
			Position:  i,
			Members:   []store.TeamMember{},
		}
	}
	for i, p := range shuffled {
		t := &teams[i%teamCount]
		t.Members = append(t.Members, p)
	}
	return teams
}
