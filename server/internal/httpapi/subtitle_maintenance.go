package httpapi

import (
	"net/http"
	"tm-sonder/server/internal/subtitles"
)

// SetSubtitleManager is called before either HTTP listener starts serving.
func (s *Server) SetSubtitleManager(manager *subtitles.Manager) { s.subtitleManager = manager }

func (s *Server) handleSubtitleMaintenance(w http.ResponseWriter, r *http.Request) {
	if !s.requireOwner(w, r) {
		return
	}
	if s.subtitleManager == nil {
		writeError(w, http.StatusServiceUnavailable, "Subtitle maintenance has not started")
		return
	}
	if r.Method == http.MethodPost {
		s.subtitleManager.Trigger()
		writeJSON(w, http.StatusAccepted, map[string]bool{"scheduled": true})
		return
	}
	writeJSON(w, http.StatusOK, s.subtitleManager.Status())
}
