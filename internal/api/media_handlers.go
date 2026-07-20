package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

const (
	maxPhotoBytes = 10 << 20  // 10 MB
	maxVideoBytes = 100 << 20 // 100 MB
)

var allowedExtensions = map[string]string{
	".jpg": "photo", ".jpeg": "photo", ".png": "photo", ".webp": "photo", ".gif": "photo",
	".mp4": "video", ".mov": "video", ".webm": "video",
}

func (s *Server) handleListMedia(w http.ResponseWriter, r *http.Request) {
	media, err := s.store.MediaByMatch(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, orEmpty(media))
}

// handleUploadMedia recebe multipart (campo "file", opcional "caption") e grava o
// arquivo em disco; a URL servida fica atrás de autenticação.
func (s *Server) handleUploadMedia(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("id")
	user := currentUser(r)

	participant, err := s.store.IsParticipant(r.Context(), matchID, user.ID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !participant {
		writeError(w, http.StatusForbidden, "somente quem participou da partida pode enviar mídias")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxVideoBytes)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "upload inválido ou arquivo grande demais")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "campo 'file' ausente")
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	mediaType, ok := allowedExtensions[ext]
	if !ok {
		writeError(w, http.StatusBadRequest, "formato não suportado (fotos: jpg/png/webp/gif; vídeos: mp4/mov/webm)")
		return
	}
	if mediaType == "photo" && header.Size > maxPhotoBytes {
		writeError(w, http.StatusBadRequest, "foto acima de 10 MB")
		return
	}

	dir := filepath.Join(s.cfg.MediaDir, matchID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeStoreError(w, err)
		return
	}
	filename := uuid.NewString() + ext
	dst, err := os.Create(filepath.Join(dir, filename))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	defer dst.Close()
	if _, err := io.Copy(dst, file); err != nil {
		writeStoreError(w, err)
		return
	}

	url := fmt.Sprintf("/api/media/files/%s/%s", matchID, filename)
	media, err := s.store.AddMedia(r.Context(), matchID, user.ID, mediaType, url, r.FormValue("caption"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	s.store.LogActivity(r.Context(), &user.ID, "media_added",
		fmt.Sprintf("%s adicionou uma %s à partida", user.Name, mediaLabel(mediaType)))
	writeJSON(w, http.StatusCreated, media)
}

func mediaLabel(mediaType string) string {
	if mediaType == "video" {
		return "vídeo"
	}
	return "foto"
}

// handleDeleteMedia permite remoção pelo autor ou moderação por admin (soft delete).
func (s *Server) handleDeleteMedia(w http.ResponseWriter, r *http.Request) {
	media, err := s.store.MediaByID(r.Context(), r.PathValue("id"))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	user := currentUser(r)
	if media.UploadedBy != user.ID && user.Role != "admin" {
		writeError(w, http.StatusForbidden, "somente o autor ou um administrador pode remover")
		return
	}
	if err := s.store.RemoveMedia(r.Context(), media.ID); err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleServeMedia entrega o arquivo físico; a rota exige sessão válida.
func (s *Server) handleServeMedia(w http.ResponseWriter, r *http.Request) {
	matchID := r.PathValue("matchId")
	filename := r.PathValue("filename")
	// PathValue não contém separadores, então o join abaixo não escapa do MediaDir.
	http.ServeFile(w, r, filepath.Join(s.cfg.MediaDir, matchID, filename))
}
