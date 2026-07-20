package draw

import (
	"fmt"
	"testing"

	"futdarapaziada/api/internal/store"
)

func players(n int) []store.TeamMember {
	list := make([]store.TeamMember, n)
	for i := range list {
		list[i] = store.TeamMember{UserID: fmt.Sprintf("u%d", i), Name: fmt.Sprintf("Jogador %d", i)}
	}
	return list
}

func TestTeamsBalance(t *testing.T) {
	for _, tc := range []struct{ players, teams int }{
		{14, 2}, {15, 2}, {16, 3}, {7, 2}, {9, 4},
	} {
		teams := Teams(players(tc.players), tc.teams)
		if len(teams) != tc.teams {
			t.Fatalf("%d jogadores em %d times: obteve %d times", tc.players, tc.teams, len(teams))
		}
		minSize, maxSize := tc.players, 0
		total := 0
		for _, team := range teams {
			size := len(team.Members)
			total += size
			if size < minSize {
				minSize = size
			}
			if size > maxSize {
				maxSize = size
			}
		}
		if total != tc.players {
			t.Errorf("jogadores distribuídos = %d, quer %d", total, tc.players)
		}
		// Requisito 5.4: diferença máxima de um jogador entre os times.
		if maxSize-minSize > 1 {
			t.Errorf("%d jogadores em %d times: diferença %d entre times", tc.players, tc.teams, maxSize-minSize)
		}
	}
}

func TestTeamsClampCount(t *testing.T) {
	if got := len(Teams(players(10), 1)); got != 2 {
		t.Errorf("teamCount 1 deveria virar 2, obteve %d", got)
	}
	if got := len(Teams(players(20), 9)); got != MaxTeams {
		t.Errorf("teamCount 9 deveria virar %d, obteve %d", MaxTeams, got)
	}
}
