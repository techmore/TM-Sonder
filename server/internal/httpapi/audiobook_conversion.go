package httpapi

import (
	"encoding/json"
	"mime"
	"net/http"
	"os"
	"strings"
	"tm-sonder/server/internal/api"
	"tm-sonder/server/internal/audiobookopt"
)

func (s *Server) SetMP3Converter(m *audiobookopt.MP3Manager) { s.mp3Converter = m }
func (s *Server) conversionManager(w http.ResponseWriter) bool {
	if s.mp3Converter == nil {
		writeError(w, http.StatusServiceUnavailable, "MP3 conversion requires FFmpeg and FFprobe")
		return false
	}
	return true
}
func (s *Server) handleAudiobookConversion(w http.ResponseWriter, r *http.Request) {
	if !s.conversionManager(w) {
		return
	}
	writeJSON(w, http.StatusOK, s.mp3Converter.Status(r.PathValue("id")))
}
func (s *Server) handleAudiobookConversionStart(w http.ResponseWriter, r *http.Request) {
	if !s.conversionManager(w) {
		return
	}
	owner := false
	if name, ok := s.sessionUsername(r); ok && s.accounts != nil {
		owner = name == s.accounts.Username()
	}
	if !owner && !s.tokenMatches(r) && !(isLoopback(peerHost(r)) && hostIsLoopback(r.Host)) {
		writeError(w, http.StatusForbidden, "Only the server owner can create conversions")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var body struct {
		ApproveCatalogCover bool `json:"approveCatalogCover"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid conversion request")
		return
	}
	var reviewed []api.AudiobookChapter
	var review audiobookopt.MP3ChapterReview
	parts, err := s.chapterBookParts(r.PathValue("id"))
	if err == nil {
		imported, loadErr := s.loadImportedChapterMap(parts)
		if loadErr != nil && !os.IsNotExist(loadErr) {
			writeError(w, http.StatusUnprocessableEntity, "Reviewed chapter map is stale or invalid; review it before conversion")
			return
		}
		if loadErr == nil {
			review.SourceURL = imported.SourceURL
			review.ImportedAt = imported.ImportedAt
			for i, c := range imported.Chapters {
				reviewed = append(reviewed, api.AudiobookChapter{Index: i + 1, Title: c.Title, StartSeconds: c.StartSeconds, EndSeconds: &c.EndSeconds})
			}
		}
	}
	review.Chapters = reviewed
	job, err := s.mp3Converter.Enqueue(r.PathValue("id"), body.ApproveCatalogCover, review)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"job": job})
}
func (s *Server) handleAudiobookConversionMedia(w http.ResponseWriter, r *http.Request) {
	if !s.conversionManager(w) {
		return
	}
	if r.PathValue("conversionMedia") != "download" && r.PathValue("conversionMedia") != "stream" {
		http.NotFound(w, r)
		return
	}
	f, err := s.mp3Converter.Open(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "No verified converted audiobook available")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Converted audiobook unavailable")
		return
	}
	name := "audiobook.m4b"
	if item, ok := s.store.Get(r.PathValue("id")); ok && strings.TrimSpace(item.Title) != "" {
		title := strings.NewReplacer("/", " - ", "\\", " - ", "\n", " ", "\r", " ").Replace(strings.TrimSpace(item.Title))
		name = title + ".m4b"
	}
	w.Header().Set("Content-Type", "audio/mp4")
	w.Header().Set("Cache-Control", "private, no-store")
	if r.PathValue("conversionMedia") == "download" {
		w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": name}))
	}
	http.ServeContent(w, r, name, st.ModTime(), f)
}
