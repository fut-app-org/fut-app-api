package jobs

import (
	"futdarapaziada/api/internal/store"
	"strings"
	"testing"
)

func TestMatchPushPointsToExactMatch(t *testing.T) {
	d := store.PushDelivery{MatchID: "test-id", MatchDate: "2026-10-15", StartTime: "20:00", Venue: "Quadra", Kind: "created"}
	m := matchPushMessage(d)
	if m.URL != "/partida?match=test-id" || !strings.Contains(m.Body, "15/10 às 20:00 · Quadra") {
		t.Fatalf("wrong message: %+v", m)
	}
	d.Kind = "reminder"
	r := matchPushMessage(d)
	if r.Title == m.Title || r.Tag == m.Tag {
		t.Fatal("reminder indistinguishable from new match")
	}
}
