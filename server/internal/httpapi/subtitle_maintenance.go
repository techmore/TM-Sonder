package httpapi

import (
	"net/http"
	"tm-sonder/server/internal/subtitles"
)

// SetSubtitleManager is called before either HTTP listener starts serving.
func (s *Server) SetSubtitleManager(manager *subtitles.Manager) { s.subtitleManager = manager }

func (s *Server) handleSubtitleMaintenance(w http.ResponseWriter, r *http.Request) {
	if !s.requireSubtitleOwner(w, r) {
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

// Runtime control is restricted to the owner account or direct loopback access.
func (s *Server) requireSubtitleOwner(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("X-Forwarded-Host") == "" && isLoopback(peerHost(r)) && hostIsLoopback(r.Host) {
		return true
	}
	username, ok := s.sessionUsername(r)
	if !ok || username != s.accounts.Username() {
		writeError(w, http.StatusForbidden, "Owner account required")
		return false
	}
	return true
}
