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

func TestManual(t *testing.T) {
	list := players(5)

	t.Run("sucesso", func(t *testing.T) {
		teams, err := Manual(list, [][]string{{"u0", "u1", "u2"}, {"u3", "u4"}})
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if len(teams) != 2 {
			t.Fatalf("obteve %d times, quer 2", len(teams))
		}
		if got := len(teams[0].Members); got != 3 {
			t.Errorf("time 1 com %d jogadores, quer 3", got)
		}
		if teams[0].Members[0].UserID != "u0" || teams[1].Members[1].UserID != "u4" {
			t.Errorf("escalação não preservou a ordem informada")
		}
		if teams[0].TeamName == "" || teams[1].TeamColor == "" {
			t.Errorf("times deveriam herdar nome e cor dos presets visuais")
		}
	})

	t.Run("jogador duplicado", func(t *testing.T) {
		if _, err := Manual(list, [][]string{{"u0", "u1", "u2"}, {"u2", "u3", "u4"}}); err == nil {
			t.Error("esperava erro para jogador em dois times")
		}
	})

	t.Run("jogador não confirmado", func(t *testing.T) {
		if _, err := Manual(list, [][]string{{"u0", "u1", "u2"}, {"u3", "u4", "u9"}}); err == nil {
			t.Error("esperava erro para ID fora dos confirmados")
		}
	})

	t.Run("confirmado sem time", func(t *testing.T) {
		if _, err := Manual(list, [][]string{{"u0", "u1"}, {"u2", "u3"}}); err == nil {
			t.Error("esperava erro para confirmado não escalado")
		}
	})

	t.Run("quantidade de times inválida", func(t *testing.T) {
		if _, err := Manual(list, [][]string{{"u0", "u1", "u2", "u3", "u4"}}); err == nil {
			t.Error("esperava erro para menos de 2 times")
		}
	})
}
