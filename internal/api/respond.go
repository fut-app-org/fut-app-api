package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/jackc/pgx/v5/pgconn"

	"futdarapaziada/api/internal/store"
)

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if payload != nil {
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			log.Printf("escrevendo resposta: %v", err)
		}
	}
}

type apiError struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, apiError{Error: message})
}

func writeErrorCode(w http.ResponseWriter, status int, message, code string) {
	writeJSON(w, status, apiError{Error: message, Code: code})
}

// writeStoreError converte erros comuns do store em respostas HTTP.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound), isMalformedID(err):
		writeError(w, http.StatusNotFound, "registro não encontrado")
	case errors.Is(err, store.ErrConfirmationClosed):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrBatchExists):
		writeError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("erro interno: %v", err)
		writeError(w, http.StatusInternalServerError, "erro interno")
	}
}

// isMalformedID identifica um id de URL que não é um uuid válido. O Postgres
// rejeita o texto na conversão; como o registro não poderia existir, a resposta
// é 404 em vez de 500.
func isMalformedID(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "22P02" // invalid_text_representation
}

// orEmpty troca um slice nil por um vazio. Sem isso o encoding/json escreve
// "null", e o front trata toda lista como array.
func orEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "corpo da requisição inválido")
		return false
	}
	return true
}
