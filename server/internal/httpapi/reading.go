package httpapi

import (
	"math"
	"net/http"
	"strings"
	"time"

	"tm-sonder/server/internal/api"
)

type readingQueueOrderRequest struct {
	ItemIDs []string `json:"itemIDs"`
}

func (s *Server) handleReading(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, s.store.ReadingState())
}

func (s *Server) handleReadingBook(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	item, ok := s.store.Get(id)
	if !ok || (item.Kind != api.KindAudiobook && item.Kind != api.KindEbook) {
		writeError(w, http.StatusNotFound, "Book not found")
		return
	}
	switch r.Method {
	case http.MethodPatch, http.MethodPut:
		var update api.LibraryReadingUpdate
		if err := jsonDecode(w, r, &update); err != nil || (update.Queued == nil && update.Liked == nil) {
			writeError(w, http.StatusBadRequest, "Provide queued and/or liked state")
			return
		}
		record, ok := s.store.SetBookReadingFlags(id, update.Queued, update.Liked, time.Now().UTC())
		if !ok {
			writeError(w, http.StatusNotFound, "Book not found")
			return
		}
		s.mutationSaved()
		writeJSON(w, http.StatusOK, record)
	default:
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
	}
}

func (s *Server) handleReadingQueueReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	var request readingQueueOrderRequest
	if err := jsonDecode(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid queue order")
		return
	}
	queue := s.store.ReorderReadingQueue(request.ItemIDs)
	s.mutationSaved()
	writeJSON(w, http.StatusOK, map[string][]string{"queue": queue})
}

func (s *Server) handleReadingSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	var update api.ReadingSessionUpdate
	if err := jsonDecode(w, r, &update); err != nil || strings.TrimSpace(update.SessionID) == "" ||
		!finiteNonNegative(update.ActiveSeconds) || !finiteNonNegative(update.MediaSeconds) ||
		update.ActiveSeconds > 24*60*60 || update.MediaSeconds > 7*24*60*60 {
		writeError(w, http.StatusBadRequest, "Invalid reading session")
		return
	}
	record, ok := s.store.RecordReadSession(id, update, time.Now().UTC())
	if !ok {
		writeError(w, http.StatusNotFound, "Audiobook not found")
		return
	}
	// This compact sidecar is synced independently from the large catalog so
	// force-close and power-loss recovery retain both position and read history.
	s.progressChanged()
	writeJSON(w, http.StatusOK, record)
}

func finiteNonNegative(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
