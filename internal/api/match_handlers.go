package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"futdarapaziada/api/internal/draw"
	"futdarapaziada/api/internal/store"
)

func (s *Server) handleNextMatch(w http.ResponseWriter, r *http.Request) {
	match, err := s.store.NextMatch(r.Context())
	if errors.Is(err, store.ErrNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"match": nil})
		return
	}
	if err != nil {
		writeStoreError(w, err)
		return
	}
	myResponse := "no_response"
	entries, err := s.store.Confirmations(r.Context(), match.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	for _, e := range entries {
		if e.UserID == currentUser(r).ID {
			myResponse = e.Response
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"match":         match,
		"my_response":   myResponse,
		"confirmations": orEmpty(entries),
	})
}

func (s *Server) handleListMatches(w http.ResponseWriter, r *http.Request) {
	matches, err := s.store.ListMatches(r.Context(), r.URL.Query().Get("month"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	// O histórico mostra os vencedores de cada partida finalizada.
	type matchWithResults struct {
		store.Match
		Results []store.VoteResult `json:"results,omitempty"`
	}
	out := make([]matchWithResults, 0, len(matches))
	for _, m := range matches {
		item := matchWithResults{Match: m}
		if m.Status == "finished" {
			results, err := s.store.VoteResults(r.Context(), m.ID)
			if err != nil {
				writeStoreError(w, err)
				return
			}
			item.Results = results
		}
		out = append(out, item)
	}
	writeJSON(w, http.StatusOK, orEmpty(out))
}

func (s *Server) handleGetMatch(w http.ResponseWriter, r *http.Request) {
	match, err := s.store.MatchByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	user := currentUser(r)

	entries, err := s.store.Confirmations(r.Context(), match.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	teams, err := s.store.Teams(r.Context(), match.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	media, err := s.store.MediaByMatch(r.Context(), match.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	isParticipant, err := s.store.IsParticipant(r.Context(), match.ID, user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	myVotes, err := s.store.MyVotes(r.Context(), match.ID, user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}

	// Resultado só é exposto depois do encerramento, para não influenciar votos.
	var results []store.VoteResult
	if match.Status == "finished" {
		if results, err = s.store.VoteResults(r.Context(), match.ID); err != nil {
			writeStoreError(w, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"match":          match,
		"confirmations":  orEmpty(entries),
		"teams":          orEmpty(teams),
		"media":          orEmpty(media),
		"is_participant": isParticipant,
		"my_votes":       myVotes,
		"results":        results,
	})
}

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Response string `json:"response"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Response != "going" && body.Response != "not_going" {
		writeError(w, http.StatusBadRequest, "response deve ser going ou not_going")
		return
	}
	if err := s.store.Confirm(r.Context(), r.PathValue("id"), currentUser(r).ID, body.Response); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleConfirmations(w http.ResponseWriter, r *http.Request) {
	entries, err := s.store.Confirmations(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(entries))
}

func (s *Server) handleTeams(w http.ResponseWriter, r *http.Request) {
	teams, err := s.store.Teams(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(teams))
}

func (s *Server) handleVote(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	user := currentUser(r)

	var body struct {
		Category    string `json:"category"`
		CandidateID string `json:"candidate_id"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.Category != "top_scorer" && body.Category != "worst_player" {
		writeError(w, http.StatusBadRequest, "categoria inválida")
		return
	}
	if _, err := uuid.Parse(body.CandidateID); err != nil {
		writeError(w, http.StatusBadRequest, "candidate_id inválido")
		return
	}

	match, err := s.store.MatchByID(r.Context(), matchID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if match.Status != "voting" {
		writeError(w, http.StatusConflict, "a votação desta partida não está aberta")
		return
	}
	// Só participa da votação (votando ou recebendo voto) quem jogou.
	voterOK, err := s.store.IsParticipant(r.Context(), matchID, user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	candidateOK, err := s.store.IsParticipant(r.Context(), matchID, body.CandidateID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !voterOK || !candidateOK {
		writeError(w, http.StatusForbidden, "somente quem participou da partida pode votar e receber votos")
		return
	}
	if body.Category == "worst_player" && body.CandidateID == user.ID {
		writeError(w, http.StatusBadRequest, "não vale votar em si mesmo nessa categoria")
		return
	}

	if err := s.store.CastVote(r.Context(), matchID, user.ID, body.Category, body.CandidateID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- Ações administrativas sobre partidas ---

func (s *Server) handleCreateMatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MatchDate            string    `json:"match_date"`
		StartTime            string    `json:"start_time"`
		EndTime              string    `json:"end_time"`
		Venue                string    `json:"venue"`
		Address              string    `json:"address"`
		ConfirmationDeadline time.Time `json:"confirmation_deadline"`
		Notes                string    `json:"notes"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if body.MatchDate == "" || body.StartTime == "" || body.Venue == "" || body.ConfirmationDeadline.IsZero() {
		writeError(w, http.StatusBadRequest, "data, horário, local e prazo de confirmação são obrigatórios")
		return
	}
	if body.EndTime == "" {
		body.EndTime = body.StartTime
	}
	user := currentUser(r)
	match, err := s.store.CreateMatch(r.Context(), store.MatchInput{
		MatchDate: body.MatchDate, StartTime: body.StartTime, EndTime: body.EndTime,
		Venue: body.Venue, Address: body.Address,
		ConfirmationDeadline: body.ConfirmationDeadline, Notes: body.Notes,
	}, user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.store.LogActivity(r.Context(), &user.ID, "match_created",
		fmt.Sprintf("%s abriu as confirmações da partida de %s", user.Name, formatDateBR(match.MatchDate)))
	writeJSON(w, http.StatusCreated, match)
}

func (s *Server) handleUpdateMatch(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	var body struct {
		MatchDate            *string    `json:"match_date"`
		StartTime            *string    `json:"start_time"`
		EndTime              *string    `json:"end_time"`
		Venue                *string    `json:"venue"`
		Address              *string    `json:"address"`
		ConfirmationDeadline *time.Time `json:"confirmation_deadline"`
		Notes                *string    `json:"notes"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	err := s.store.UpdateMatch(r.Context(), matchID, store.MatchUpdate{
		MatchDate: body.MatchDate, StartTime: body.StartTime, EndTime: body.EndTime,
		Venue: body.Venue, Address: body.Address,
		ConfirmationDeadline: body.ConfirmationDeadline, Notes: body.Notes,
	})
	if err != nil {
		writeStoreError(w, err)
		return
	}
	match, err := s.store.MatchByID(r.Context(), matchID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, match)
}

func (s *Server) handleCancelMatch(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}
	if err := s.store.CancelMatch(r.Context(), r.PathValue("id"), body.Reason); err != nil {
		writeStoreError(w, err)
		return
	}
	user := currentUser(r)
	s.store.LogActivity(r.Context(), &user.ID, "match_cancelled",
		fmt.Sprintf("%s cancelou a partida (%s)", user.Name, body.Reason))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleCloseConfirmations(w http.ResponseWriter, r *http.Request) {
	if err := s.store.SetMatchStatus(r.Context(), r.PathValue("id"), "closed", "open"); err != nil {
		writeStoreError(w, err)
		return
	}
	user := currentUser(r)
	s.store.LogActivity(r.Context(), &user.ID, "confirmations_closed",
		fmt.Sprintf("%s encerrou as confirmações da partida", user.Name))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleReopenConfirmations(w http.ResponseWriter, r *http.Request) {
	if err := s.store.SetMatchStatus(r.Context(), r.PathValue("id"), "open", "closed", "teams_drawn"); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDrawTeams(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	var body struct {
		TeamCount int `json:"team_count"`
	}
	if !decodeJSON(w, r, &body) {
		return
	}

	match, err := s.store.MatchByID(r.Context(), matchID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if match.Status != "closed" && match.Status != "teams_drawn" {
		writeError(w, http.StatusConflict, "o sorteio só fica disponível após encerrar as confirmações")
		return
	}

	entries, err := s.store.Confirmations(r.Context(), matchID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	var players []store.TeamMember
	for _, e := range entries {
		if e.Response == "going" {
			players = append(players, store.TeamMember{UserID: e.UserID, Name: e.Name, AvatarColor: e.AvatarColor})
		}
	}
	if len(players) < 2 {
		writeError(w, http.StatusConflict, "é preciso ao menos 2 confirmados para sortear")
		return
	}

	teams := draw.Teams(players, body.TeamCount)
	if err := s.store.ReplaceTeams(r.Context(), matchID, teams); err != nil {
		writeStoreError(w, err)
		return
	}
	user := currentUser(r)
	s.store.LogActivity(r.Context(), &user.ID, "teams_drawn",
		fmt.Sprintf("%s sorteou os times da partida de %s", user.Name, formatDateBR(match.MatchDate)))

	saved, err := s.store.Teams(r.Context(), matchID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleFinishMatch(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	user := currentUser(r)

	votingDays := s.store.SettingInt(r.Context(), "voting_close_days", 2)
	closes := time.Now().AddDate(0, 0, votingDays)

	if err := s.store.FinishMatch(r.Context(), matchID, user.ID, closes); err != nil {
		writeStoreError(w, err)
		return
	}
	s.store.LogActivity(r.Context(), &user.ID, "match_finished",
		fmt.Sprintf("%s encerrou a partida; votação aberta até %s", user.Name, closes.Format("02/01")))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleCloseVoting(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	if err := s.store.SetMatchStatus(r.Context(), matchID, "finished", "voting"); err != nil {
		writeStoreError(w, err)
		return
	}
	s.logVoteWinners(r, matchID)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) logVoteWinners(r *http.Request, matchID string) {
	results, err := s.store.VoteResults(r.Context(), matchID)
	if err != nil {
		return
	}
	for _, res := range results {
		for _, winner := range res.Winners {
			label := "artilheiro"
			if res.Category == "worst_player" {
				label = "perna de pau"
			}
			s.store.LogActivity(r.Context(), nil, "vote_result",
				fmt.Sprintf("%s foi eleito %s da partida", winner.Name, label))
		}
	}
}

// formatDateBR converte YYYY-MM-DD para DD/MM.
func formatDateBR(isoDate string) string {
	t, err := time.Parse("2006-01-02", isoDate)
	if err != nil {
		return isoDate
	}
	return t.Format("02/01")
}
